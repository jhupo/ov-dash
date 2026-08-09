package worker

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ov-dash/backend/internal/config"
	"ov-dash/backend/internal/events"
	"ov-dash/backend/internal/platform"
	platformmodule "ov-dash/backend/internal/platform/module"
	"ov-dash/backend/internal/queue"

	"go.uber.org/zap"
)

func TestNewRunnerRequiresExternalJobRegistry(t *testing.T) {
	_, err := NewRunner(RunnerDeps{Runtime: &platform.Runtime{Queue: &queue.Client{}}})
	if err == nil || err.Error() != "worker job registry is required" {
		t.Fatalf("error = %v", err)
	}
}

func TestRunnerStartsHeartbeatOnlyAfterRiverStarts(t *testing.T) {
	registry := platformmodule.NewJobRegistry()
	heartbeatRecorded := make(chan string, 1)
	workerStopped := errors.New("worker stopped")
	var heartbeatCalls atomic.Int32
	runner := &Runner{
		runtime: &platform.Runtime{
			Config: config.Config{
				App: config.AppConfig{ShutdownTimeout: time.Second},
				Worker: config.WorkerConfig{
					QueueName: "jobs:test", Concurrency: 1, JobTimeout: time.Second, RescueAfter: 2 * time.Second,
				},
			},
			Logger: zap.NewNop(),
		},
		jobs:       registry,
		instanceID: "worker-test",
		releaseID:  "ov-dash-1.2.3",
		runWorker: func(_ context.Context, opts queue.WorkerOptions, _ queue.Dispatcher) error {
			if got := heartbeatCalls.Load(); got != 0 {
				t.Fatalf("heartbeat calls before River start = %d", got)
			}
			opts.Started()
			select {
			case <-heartbeatRecorded:
				return workerStopped
			case <-time.After(time.Second):
				t.Fatal("heartbeat was not recorded after River start")
				return nil
			}
		},
		heartbeat: func(_ context.Context, _ string, _ string, releaseID string, _ string, _ int) error {
			heartbeatCalls.Add(1)
			heartbeatRecorded <- releaseID
			return nil
		},
	}

	if err := runner.Run(context.Background()); !errors.Is(err, workerStopped) {
		t.Fatalf("Run error = %v, want worker stopped", err)
	}
	if got := heartbeatCalls.Load(); got != 1 {
		t.Fatalf("heartbeat calls = %d, want 1", got)
	}
}

func TestRunnerStartFailureDoesNotRecordHeartbeatOrLeakWaiter(t *testing.T) {
	startErr := errors.New("river unavailable")
	var heartbeatCalls atomic.Int32
	runner := &Runner{
		runtime: &platform.Runtime{
			Config: config.Config{
				App: config.AppConfig{ShutdownTimeout: time.Second},
				Worker: config.WorkerConfig{
					QueueName: "jobs:test", Concurrency: 1, JobTimeout: time.Second, RescueAfter: 2 * time.Second,
				},
			},
			Logger: zap.NewNop(),
		},
		jobs:       platformmodule.NewJobRegistry(),
		instanceID: "worker-test",
		releaseID:  "ov-dash-1.2.3",
		runWorker: func(context.Context, queue.WorkerOptions, queue.Dispatcher) error {
			return startErr
		},
		heartbeat: func(context.Context, string, string, string, string, int) error {
			heartbeatCalls.Add(1)
			return nil
		},
	}

	done := make(chan error, 1)
	go func() { done <- runner.Run(context.Background()) }()
	select {
	case err := <-done:
		if !errors.Is(err, startErr) {
			t.Fatalf("Run error = %v, want start failure", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Runner did not release heartbeat waiter after start failure")
	}
	if got := heartbeatCalls.Load(); got != 0 {
		t.Fatalf("heartbeat calls = %d, want 0", got)
	}
}

func TestRunnerCanceledBeforeStartDoesNotRecordHeartbeat(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var heartbeatCalls atomic.Int32
	runner := &Runner{
		runtime: &platform.Runtime{
			Config: config.Config{
				App: config.AppConfig{ShutdownTimeout: time.Second},
				Worker: config.WorkerConfig{
					QueueName: "jobs:test", Concurrency: 1, JobTimeout: time.Second, RescueAfter: 2 * time.Second,
				},
			},
			Logger: zap.NewNop(),
		},
		jobs:       platformmodule.NewJobRegistry(),
		instanceID: "worker-test",
		releaseID:  "ov-dash-1.2.3",
		runWorker: func(ctx context.Context, _ queue.WorkerOptions, _ queue.Dispatcher) error {
			return ctx.Err()
		},
		heartbeat: func(context.Context, string, string, string, string, int) error {
			heartbeatCalls.Add(1)
			return nil
		},
	}

	if err := runner.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context canceled", err)
	}
	if got := heartbeatCalls.Load(); got != 0 {
		t.Fatalf("heartbeat calls = %d, want 0", got)
	}
}

func TestRunnerTimeoutUsesDefinitionOverride(t *testing.T) {
	registry := platformmodule.NewJobRegistry()
	if err := registry.Register(platformmodule.JobDefinition{
		Type:    "noop",
		Timeout: 7 * time.Second,
		Handler: fakeJobHandler{},
	}); err != nil {
		t.Fatal(err)
	}
	runner := &Runner{
		runtime: &platform.Runtime{Config: config.Config{Worker: config.WorkerConfig{JobTimeout: time.Minute}}},
		jobs:    registry,
	}
	if got := runner.Timeout(queue.Job{Type: "noop"}); got != 7*time.Second {
		t.Fatalf("timeout = %s", got)
	}
	if got := runner.Timeout(queue.Job{Type: "missing"}); got != time.Minute {
		t.Fatalf("fallback timeout = %s", got)
	}
}

func TestDispatchPersistsLifecycleWithoutSynchronousPublish(t *testing.T) {
	registry := platformmodule.NewJobRegistry()
	if err := registry.Register(platformmodule.JobDefinition{Type: "noop", Handler: fakeJobHandler{}}); err != nil {
		t.Fatal(err)
	}
	bus := events.NewBus(zap.NewNop())
	published := []events.Event{}
	bus.SubscribeAll(func(_ context.Context, event events.Event) error {
		published = append(published, event)
		return nil
	})
	lifecycle := &fakeLifecycleQueue{}
	runner := &Runner{
		runtime: &platform.Runtime{
			Config: config.Config{Worker: config.WorkerConfig{QueueName: "jobs:test"}},
			Events: bus,
			Logger: zap.NewNop(),
		},
		lifecycle: lifecycle,
		jobs:      registry,
	}
	job, err := queue.NewJob("noop", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	job.Attempts = 1

	if err := runner.Dispatch(context.Background(), job); err != nil {
		t.Fatalf("dispatch job: %v", err)
	}
	if got := strings.Join(lifecycle.transitions, ","); got != "started,completed" {
		t.Fatalf("transitions = %q, want started,completed", got)
	}
	if len(lifecycle.logs) != 2 || lifecycle.logs[0] != "job started" || lifecycle.logs[1] != "job completed" {
		t.Fatalf("logs = %#v", lifecycle.logs)
	}
	if len(published) != 0 {
		t.Fatalf("synchronous events = %#v, want none", published)
	}
}

func TestDispatchReturnsCompletedTransitionFailure(t *testing.T) {
	registry := platformmodule.NewJobRegistry()
	if err := registry.Register(platformmodule.JobDefinition{Type: "noop", Handler: fakeJobHandler{}}); err != nil {
		t.Fatal(err)
	}
	lifecycle := &fakeLifecycleQueue{completedErr: errors.New("outbox unavailable")}
	runner := &Runner{
		runtime: &platform.Runtime{
			Config: config.Config{Worker: config.WorkerConfig{QueueName: "jobs:test"}},
			Logger: zap.NewNop(),
		},
		lifecycle: lifecycle,
		jobs:      registry,
	}
	job, err := queue.NewJob("noop", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}

	err = runner.Dispatch(context.Background(), job)
	if err == nil || !strings.Contains(err.Error(), "mark job completed") {
		t.Fatalf("error = %v, want completed transition failure", err)
	}
	if len(lifecycle.logs) != 1 || lifecycle.logs[0] != "job started" {
		t.Fatalf("logs = %#v, completed log must not be written", lifecycle.logs)
	}
}

func TestDispatchHonorsCancelRequestedAfterSuccessfulHandler(t *testing.T) {
	registry := platformmodule.NewJobRegistry()
	if err := registry.Register(platformmodule.JobDefinition{Type: "noop", Handler: fakeJobHandler{}}); err != nil {
		t.Fatal(err)
	}
	lifecycle := &fakeLifecycleQueue{cancelRequested: true}
	runner := &Runner{
		runtime: &platform.Runtime{
			Config: config.Config{Worker: config.WorkerConfig{QueueName: "jobs:test"}},
			Logger: zap.NewNop(),
		},
		lifecycle: lifecycle,
		jobs:      registry,
	}
	job, err := queue.NewJob("noop", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}

	if err := runner.Dispatch(context.Background(), job); err != nil {
		t.Fatalf("dispatch canceled job: %v", err)
	}
	if got := strings.Join(lifecycle.transitions, ","); got != "started,canceled" {
		t.Fatalf("transitions = %q, want started,canceled", got)
	}
	if len(lifecycle.logs) != 2 || lifecycle.logs[1] != "job canceled" {
		t.Fatalf("logs = %#v, want terminal canceled log", lifecycle.logs)
	}
}

func TestDispatchCommitsCompletionWithoutCanceledHandlerContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	registry := platformmodule.NewJobRegistry()
	if err := registry.Register(platformmodule.JobDefinition{
		Type: "noop",
		Handler: fakeJobHandler{handle: func(context.Context, queue.Job) error {
			cancel()
			return nil
		}},
	}); err != nil {
		t.Fatal(err)
	}
	lifecycle := &fakeLifecycleQueue{}
	runner := &Runner{
		runtime: &platform.Runtime{
			Config: config.Config{Worker: config.WorkerConfig{QueueName: "jobs:test"}},
			Logger: zap.NewNop(),
		},
		lifecycle: lifecycle,
		jobs:      registry,
	}
	job, err := queue.NewJob("noop", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}

	if err := runner.Dispatch(ctx, job); err != nil {
		t.Fatalf("dispatch job: %v", err)
	}
	if lifecycle.completedContextErr != nil {
		t.Fatalf("completed context error = %v, want nil", lifecycle.completedContextErr)
	}
	if lifecycle.completedLogContextErr != nil {
		t.Fatalf("completed log context error = %v, want nil", lifecycle.completedLogContextErr)
	}
}

type fakeJobHandler struct {
	handle func(context.Context, queue.Job) error
}

func (h fakeJobHandler) Handle(ctx context.Context, job queue.Job) error {
	if h.handle != nil {
		return h.handle(ctx, job)
	}
	return nil
}

type fakeLifecycleQueue struct {
	transitions            []string
	logs                   []string
	completedErr           error
	cancelRequested        bool
	completedContextErr    error
	completedLogContextErr error
}

func (q *fakeLifecycleQueue) MarkRunning(context.Context, string, queue.Job) error {
	q.transitions = append(q.transitions, "started")
	return nil
}

func (q *fakeLifecycleQueue) MarkCompleted(ctx context.Context, _ string, _ queue.Job) error {
	q.transitions = append(q.transitions, "completed")
	q.completedContextErr = ctx.Err()
	return q.completedErr
}

func (q *fakeLifecycleQueue) MarkFailed(context.Context, string, queue.Job, string, *time.Time) error {
	q.transitions = append(q.transitions, "failed")
	return nil
}

func (q *fakeLifecycleQueue) MarkDead(context.Context, string, queue.Job, string) error {
	q.transitions = append(q.transitions, "dead")
	return nil
}

func (q *fakeLifecycleQueue) MarkCanceled(context.Context, string, queue.Job, string) error {
	q.transitions = append(q.transitions, "canceled")
	return nil
}

func (q *fakeLifecycleQueue) IsJobCancelRequested(context.Context, string) (bool, error) {
	return q.cancelRequested, nil
}

func (q *fakeLifecycleQueue) AppendJobLog(ctx context.Context, _ string, _ string, message string, _ map[string]any) error {
	q.logs = append(q.logs, message)
	if message == "job completed" {
		q.completedLogContextErr = ctx.Err()
	}
	return nil
}
