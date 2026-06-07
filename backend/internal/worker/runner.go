package worker

import (
	"context"
	"errors"
	"sync"
	"time"

	"ov-dash/backend/internal/events"
	"ov-dash/backend/internal/modules/servers"
	"ov-dash/backend/internal/platform"
	"ov-dash/backend/internal/queue"

	"go.uber.org/zap"
)

type RunnerDeps struct {
	Runtime *platform.Runtime
}

type Runner struct {
	runtime  *platform.Runtime
	handlers map[string]Handler
}

type Handler interface {
	Handle(ctx context.Context, job queue.Job) error
}

func NewRunner(deps RunnerDeps) *Runner {
	r := &Runner{
		runtime: deps.Runtime,
	}
	r.handlers = map[string]Handler{
		"python.script": NewPythonScriptHandler(deps.Runtime.Config.Python, deps.Runtime.Logger),
		"server.collect": NewServerCollectHandler(
			servers.NewCollector(servers.NewRepository(deps.Runtime.DB)),
			deps.Runtime.Logger,
		),
		"noop": NoopHandler{},
	}
	return r
}

func (r *Runner) Run(ctx context.Context) error {
	if r.runtime.Config.Worker.Concurrency < 1 {
		return errors.New("worker concurrency must be greater than zero")
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.loopServerCollectionScheduler(ctx)
	}()

	for i := 0; i < r.runtime.Config.Worker.Concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			r.loop(ctx, workerID)
		}(i + 1)
	}

	<-ctx.Done()
	r.runtime.Logger.Info("worker shutdown requested")
	wg.Wait()
	return nil
}

func (r *Runner) loop(ctx context.Context, workerID int) {
	logger := r.runtime.Logger.With(zap.Int("worker_id", workerID))
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		job, err := r.runtime.Queue.Dequeue(ctx, r.runtime.Config.Worker.QueueName, r.runtime.Config.Worker.PollInterval)
		if err != nil {
			logger.Error("dequeue job", zap.Error(err))
			time.Sleep(r.runtime.Config.Worker.PollInterval)
			continue
		}
		if job == nil {
			continue
		}
		job.Attempts++

		handler, ok := r.handlers[job.Type]
		if !ok {
			logger.Warn("unknown job type", zap.String("job_id", job.ID), zap.String("job_type", job.Type))
			if err := r.runtime.Queue.MarkDead(ctx, r.runtime.Config.Worker.QueueName, *job, "unknown job type"); err != nil {
				logger.Error("mark unknown job dead", zap.String("job_id", job.ID), zap.Error(err))
			}
			r.publishJobEvent(ctx, "job.unknown", *job, map[string]any{"worker_id": workerID})
			continue
		}

		if err := r.runtime.Queue.MarkRunning(ctx, r.runtime.Config.Worker.QueueName, *job); err != nil {
			logger.Error("mark job running", zap.String("job_id", job.ID), zap.Error(err))
		}
		r.publishJobEvent(ctx, "job.started", *job, map[string]any{"worker_id": workerID})

		jobCtx, cancel := r.jobContext(ctx, job.Type)
		err = handler.Handle(jobCtx, *job)
		cancel()

		if err != nil {
			if job.Attempts < job.MaxAttempts {
				retryAt := time.Now().UTC().Add(retryBackoff(job.Attempts))
				logger.Warn(
					"job failed, scheduling retry",
					zap.String("job_id", job.ID),
					zap.String("job_type", job.Type),
					zap.Int("attempts", job.Attempts),
					zap.Int("max_attempts", job.MaxAttempts),
					zap.Time("retry_at", retryAt),
					zap.Error(err),
				)
				if retryErr := r.runtime.Queue.ScheduleRetry(ctx, r.runtime.Config.Worker.QueueName, *job, retryAt, err.Error()); retryErr != nil {
					logger.Error("schedule job retry", zap.String("job_id", job.ID), zap.Error(retryErr))
					if markErr := r.runtime.Queue.MarkDead(ctx, r.runtime.Config.Worker.QueueName, *job, retryErr.Error()); markErr != nil {
						logger.Error("mark retry scheduling failure dead", zap.String("job_id", job.ID), zap.Error(markErr))
					}
				}
				r.publishJobEvent(ctx, "job.failed", *job, map[string]any{
					"worker_id": workerID,
					"error":     err.Error(),
					"retry_at":  retryAt,
				})
				continue
			}

			logger.Error(
				"job failed permanently",
				zap.String("job_id", job.ID),
				zap.String("job_type", job.Type),
				zap.Int("attempts", job.Attempts),
				zap.Int("max_attempts", job.MaxAttempts),
				zap.Error(err),
			)
			if markErr := r.runtime.Queue.MarkDead(ctx, r.runtime.Config.Worker.QueueName, *job, err.Error()); markErr != nil {
				logger.Error("mark job dead", zap.String("job_id", job.ID), zap.Error(markErr))
			}
			r.publishJobEvent(ctx, "job.failed", *job, map[string]any{
				"worker_id": workerID,
				"error":     err.Error(),
			})
			r.publishJobEvent(ctx, "job.dead", *job, map[string]any{
				"worker_id": workerID,
				"error":     err.Error(),
			})
			continue
		}

		if err := r.runtime.Queue.MarkCompleted(ctx, r.runtime.Config.Worker.QueueName, *job); err != nil {
			logger.Error("mark job completed", zap.String("job_id", job.ID), zap.Error(err))
		}
		logger.Info("job completed", zap.String("job_id", job.ID), zap.String("job_type", job.Type))
		r.publishJobEvent(ctx, "job.completed", *job, map[string]any{"worker_id": workerID})
	}
}

func (r *Runner) publishJobEvent(ctx context.Context, eventType string, job queue.Job, payload map[string]any) {
	payload["job_id"] = job.ID
	payload["job_type"] = job.Type
	r.runtime.Events.Publish(ctx, events.New(eventType, "worker.runner", payload))
}

func (r *Runner) jobContext(ctx context.Context, jobType string) (context.Context, context.CancelFunc) {
	if jobType == "server.collect" {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, r.runtime.Config.Worker.JobTimeout)
}

func retryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}

	delay := 5 * time.Second
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= time.Minute {
			return time.Minute
		}
	}
	return delay
}

func (r *Runner) loopServerCollectionScheduler(ctx context.Context) {
	repository := servers.NewRepository(r.runtime.DB)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	r.runtime.Logger.Info("server collector scheduler started")
	r.scheduleServerCollections(ctx, repository)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.scheduleServerCollections(ctx, repository)
		}
	}
}

func (r *Runner) scheduleServerCollections(ctx context.Context, repository *servers.Repository) {
	items, err := repository.DueForCollection(ctx, 50)
	if err != nil {
		r.runtime.Logger.Error("list due server collections", zap.Error(err))
		return
	}
	for _, item := range items {
		job, err := queue.NewJob("server.collect", map[string]any{"server_id": item.ID})
		if err != nil {
			r.runtime.Logger.Error("create server collect job", zap.String("server_id", item.ID), zap.Error(err))
			continue
		}
		if err := r.runtime.Queue.Enqueue(ctx, r.runtime.Config.Worker.QueueName, job); err != nil {
			r.runtime.Logger.Error("enqueue server collect job", zap.String("server_id", item.ID), zap.Error(err))
			continue
		}
		if err := repository.MarkCollectQueued(ctx, item.ID); err != nil {
			r.runtime.Logger.Error("mark server collect queued", zap.String("server_id", item.ID), zap.Error(err))
		}
		r.runtime.Events.Publish(ctx, events.New("server.collect.enqueued", "worker.scheduler", map[string]any{
			"job_id":    job.ID,
			"server_id": item.ID,
		}))
	}
}
