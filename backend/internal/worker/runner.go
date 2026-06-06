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
		"noop":          NoopHandler{},
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
		r.loopServerCollection(ctx)
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

		handler, ok := r.handlers[job.Type]
		if !ok {
			logger.Warn("unknown job type", zap.String("job_id", job.ID), zap.String("job_type", job.Type))
			r.publishJobEvent(ctx, "job.unknown", *job, map[string]any{"worker_id": workerID})
			continue
		}

		r.publishJobEvent(ctx, "job.started", *job, map[string]any{"worker_id": workerID})

		jobCtx, cancel := context.WithTimeout(ctx, r.runtime.Config.Worker.JobTimeout)
		err = handler.Handle(jobCtx, *job)
		cancel()

		if err != nil {
			logger.Error("job failed", zap.String("job_id", job.ID), zap.String("job_type", job.Type), zap.Error(err))
			r.publishJobEvent(ctx, "job.failed", *job, map[string]any{
				"worker_id": workerID,
				"error":     err.Error(),
			})
			continue
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

func (r *Runner) loopServerCollection(ctx context.Context) {
	collector := servers.NewCollector(servers.NewRepository(r.runtime.DB))
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	r.runtime.Logger.Info("server collector started")
	collector.CollectAll(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collector.CollectAll(ctx)
		}
	}
}
