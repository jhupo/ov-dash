package worker

import (
	"context"
	"errors"
	"sync"
	"time"

	"ov-dash/backend/internal/config"
	"ov-dash/backend/internal/db"
	"ov-dash/backend/internal/queue"

	"go.uber.org/zap"
)

type RunnerDeps struct {
	Config config.Config
	DB     *db.Pool
	Queue  *queue.Client
	Logger *zap.Logger
}

type Runner struct {
	cfg      config.Config
	db       *db.Pool
	queue    *queue.Client
	logger   *zap.Logger
	handlers map[string]Handler
}

type Handler interface {
	Handle(ctx context.Context, job queue.Job) error
}

func NewRunner(deps RunnerDeps) *Runner {
	r := &Runner{
		cfg:    deps.Config,
		db:     deps.DB,
		queue:  deps.Queue,
		logger: deps.Logger,
	}
	r.handlers = map[string]Handler{
		"python.script": NewPythonScriptHandler(deps.Config.Python, deps.Logger),
		"noop":          NoopHandler{},
	}
	return r
}

func (r *Runner) Run(ctx context.Context) error {
	if r.cfg.Worker.Concurrency < 1 {
		return errors.New("worker concurrency must be greater than zero")
	}

	var wg sync.WaitGroup
	for i := 0; i < r.cfg.Worker.Concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			r.loop(ctx, workerID)
		}(i + 1)
	}

	<-ctx.Done()
	r.logger.Info("worker shutdown requested")
	wg.Wait()
	return nil
}

func (r *Runner) loop(ctx context.Context, workerID int) {
	logger := r.logger.With(zap.Int("worker_id", workerID))
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		job, err := r.queue.Dequeue(ctx, r.cfg.Worker.QueueName, r.cfg.Worker.PollInterval)
		if err != nil {
			logger.Error("dequeue job", zap.Error(err))
			time.Sleep(r.cfg.Worker.PollInterval)
			continue
		}
		if job == nil {
			continue
		}

		handler, ok := r.handlers[job.Type]
		if !ok {
			logger.Warn("unknown job type", zap.String("job_id", job.ID), zap.String("job_type", job.Type))
			continue
		}

		jobCtx, cancel := context.WithTimeout(ctx, r.cfg.Worker.JobTimeout)
		err = handler.Handle(jobCtx, *job)
		cancel()

		if err != nil {
			logger.Error("job failed", zap.String("job_id", job.ID), zap.String("job_type", job.Type), zap.Error(err))
			continue
		}

		logger.Info("job completed", zap.String("job_id", job.ID), zap.String("job_type", job.Type))
	}
}
