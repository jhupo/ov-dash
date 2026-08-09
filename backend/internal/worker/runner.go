package worker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"ov-dash/backend/internal/events"
	"ov-dash/backend/internal/platform"
	platformmodule "ov-dash/backend/internal/platform/module"
	"ov-dash/backend/internal/queue"

	"go.uber.org/zap"
)

type RunnerDeps struct {
	Runtime *platform.Runtime
	Jobs    *platformmodule.JobRegistry
}

type lifecycleQueue interface {
	MarkRunning(context.Context, string, queue.Job) error
	MarkCompleted(context.Context, string, queue.Job) error
	MarkFailed(context.Context, string, queue.Job, string, *time.Time) error
	MarkDead(context.Context, string, queue.Job, string) error
	MarkCanceled(context.Context, string, queue.Job, string) error
	IsJobCancelRequested(context.Context, string) (bool, error)
	AppendJobLog(context.Context, string, string, string, map[string]any) error
}

type Runner struct {
	runtime    *platform.Runtime
	lifecycle  lifecycleQueue
	jobs       *platformmodule.JobRegistry
	instanceID string
	releaseID  string
	runWorker  func(context.Context, queue.WorkerOptions, queue.Dispatcher) error
	heartbeat  func(context.Context, string, string, string, string, int) error
}

func NewRunner(deps RunnerDeps) (*Runner, error) {
	if deps.Runtime == nil {
		return nil, errors.New("worker runtime is required")
	}
	if deps.Runtime.Queue == nil {
		return nil, errors.New("worker queue is required")
	}
	if deps.Jobs == nil {
		return nil, errors.New("worker job registry is required")
	}
	releaseID := queue.NormalizeReleaseID(deps.Runtime.Config.App.Version)
	if releaseID == "" {
		return nil, errors.New("worker release ID is required")
	}
	return &Runner{
		runtime:    deps.Runtime,
		lifecycle:  deps.Runtime.Queue,
		jobs:       deps.Jobs,
		instanceID: newInstanceID(),
		releaseID:  releaseID,
		runWorker:  deps.Runtime.Queue.RunWorker,
		heartbeat:  deps.Runtime.Queue.RecordWorkerHeartbeat,
	}, nil
}

func (r *Runner) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	outboxDone := make(chan struct{})
	go func() {
		defer close(outboxDone)
		r.loopOutbox(runCtx)
	}()

	workerStarted := make(chan struct{})
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		select {
		case <-runCtx.Done():
			return
		case <-workerStarted:
			r.loopWorkerHeartbeat(runCtx)
		}
	}()

	err := r.runWorker(runCtx, queue.WorkerOptions{
		QueueName:       r.runtime.Config.Worker.QueueName,
		Concurrency:     r.runtime.Config.Worker.Concurrency,
		JobTimeout:      r.runtime.Config.Worker.JobTimeout,
		RescueAfter:     r.runtime.Config.Worker.RescueAfter,
		ShutdownTimeout: r.runtime.Config.App.ShutdownTimeout,
		InstanceID:      r.instanceID,
		Started: func() {
			close(workerStarted)
		},
	}, r)
	cancel()
	<-heartbeatDone
	<-outboxDone
	return err
}

func (r *Runner) loopOutbox(ctx context.Context) {
	if r.runtime.Outbox == nil || r.runtime.Events == nil {
		return
	}
	dispatcher, err := events.NewDispatcher(r.runtime.Outbox, r.runtime.Events, r.instanceID)
	if err != nil {
		r.runtime.Logger.Error("create outbox dispatcher", zap.Error(err))
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if _, err := dispatcher.DispatchBatch(ctx); err != nil && ctx.Err() == nil {
			r.runtime.Logger.Error("dispatch outbox", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Runner) Dispatch(ctx context.Context, job queue.Job) error {
	definition, ok := r.jobs.Definition(job.Type)
	if !ok {
		err := fmt.Errorf("job type is not registered: %s", job.Type)
		return errors.Join(err, r.recordFailure(ctx, job, err))
	}

	if err := r.lifecycle.MarkRunning(ctx, r.runtime.Config.Worker.QueueName, job); err != nil {
		return fmt.Errorf("mark job running: %w", err)
	}
	r.appendJobLog(ctx, job, "system", "job started", nil)

	err := definition.Handler.Handle(ctx, job)
	if err != nil {
		cancelRequested, cancelErr := r.lifecycle.IsJobCancelRequested(context.WithoutCancel(ctx), job.ID)
		if cancelErr != nil {
			r.runtime.Logger.Error("check job cancel request", zap.String("job_id", job.ID), zap.Error(cancelErr))
		}
		if cancelRequested {
			message := "job canceled"
			if markErr := r.lifecycle.MarkCanceled(context.WithoutCancel(ctx), r.runtime.Config.Worker.QueueName, job, message); markErr != nil {
				return errors.Join(err, fmt.Errorf("mark job canceled: %w", markErr))
			}
			r.appendJobLog(context.WithoutCancel(ctx), job, "system", message, nil)
			return err
		}
		return errors.Join(err, r.recordFailure(context.WithoutCancel(ctx), job, err))
	}

	cancelRequested, err := r.lifecycle.IsJobCancelRequested(context.WithoutCancel(ctx), job.ID)
	if err != nil {
		return fmt.Errorf("check job cancel request: %w", err)
	}
	if cancelRequested {
		message := "job canceled"
		if err := r.lifecycle.MarkCanceled(context.WithoutCancel(ctx), r.runtime.Config.Worker.QueueName, job, message); err != nil {
			return fmt.Errorf("mark job canceled: %w", err)
		}
		r.appendJobLog(context.WithoutCancel(ctx), job, "system", message, nil)
		return nil
	}

	completionCtx := context.WithoutCancel(ctx)
	if err := r.lifecycle.MarkCompleted(completionCtx, r.runtime.Config.Worker.QueueName, job); err != nil {
		return fmt.Errorf("mark job completed: %w", err)
	}
	r.appendJobLog(completionCtx, job, "system", "job completed", nil)
	r.runtime.Logger.Info("job completed", zap.String("job_id", job.ID), zap.String("job_type", job.Type))
	return nil
}

func (r *Runner) Timeout(job queue.Job) time.Duration {
	if definition, ok := r.jobs.Definition(job.Type); ok && definition.Timeout != 0 {
		return definition.Timeout
	}
	return r.runtime.Config.Worker.JobTimeout
}

func (r *Runner) recordFailure(ctx context.Context, job queue.Job, jobErr error) error {
	metadata := map[string]any{
		"attempts":     job.Attempts,
		"max_attempts": job.MaxAttempts,
	}
	if job.Attempts < job.MaxAttempts {
		retryAt := time.Now().UTC().Add(queue.RetryBackoff(job.Attempts))
		if err := r.lifecycle.MarkFailed(ctx, r.runtime.Config.Worker.QueueName, job, jobErr.Error(), &retryAt); err != nil {
			return fmt.Errorf("mark job retrying: %w", err)
		}
		metadata["retry_at"] = retryAt
		r.appendJobLog(ctx, job, "stderr", jobErr.Error(), metadata)
		return nil
	}

	if err := r.lifecycle.MarkDead(ctx, r.runtime.Config.Worker.QueueName, job, jobErr.Error()); err != nil {
		return fmt.Errorf("mark job dead: %w", err)
	}
	r.appendJobLog(ctx, job, "stderr", jobErr.Error(), metadata)
	return nil
}

func (r *Runner) appendJobLog(ctx context.Context, job queue.Job, stream string, message string, metadata map[string]any) {
	if err := r.lifecycle.AppendJobLog(ctx, job.ID, stream, message, metadata); err != nil {
		r.runtime.Logger.Error("append job log", zap.String("job_id", job.ID), zap.Error(err))
	}
}

func (r *Runner) loopWorkerHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	r.recordWorkerHeartbeat(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.recordWorkerHeartbeat(ctx)
		}
	}
}

func (r *Runner) recordWorkerHeartbeat(ctx context.Context) {
	hostname, _ := os.Hostname()
	if err := r.heartbeat(
		ctx,
		r.instanceID,
		r.runtime.Config.Worker.QueueName,
		r.releaseID,
		hostname,
		os.Getpid(),
	); err != nil {
		r.runtime.Logger.Error("record worker heartbeat", zap.Error(err))
	}
}

func newInstanceID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("worker-%d", time.Now().UnixNano())
	}
	return "worker-" + hex.EncodeToString(b[:])
}
