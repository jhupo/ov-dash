package updater

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestControllerCommitsCompleteStateMachine(t *testing.T) {
	controller, executor := newTestController(t)
	operation, err := controller.Start("ov-dash-2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	finished, err := controller.Wait(context.Background(), operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.State != StateCommitted {
		t.Fatalf("state = %s, error = %s", finished.State, finished.LastError)
	}
	events, err := controller.Events(operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	states := make([]State, 0, len(events))
	for _, event := range events {
		states = append(states, event.State)
	}
	want := []State{StateRequested, StateDownloaded, StateVerified, StatePreflight, StateQuiescing, StateBackup, StateMigrating, StateSwitching, StateHealthChecking, StateCommitting, StateCommitted}
	if !reflect.DeepEqual(states, want) {
		t.Fatalf("states = %v, want %v", states, want)
	}
	if !reflect.DeepEqual(executor.actions(), []string{"download", "current", "preflight", "quiesce", "backup", "migrate", "switch", "health", "commit"}) {
		t.Fatalf("executor actions = %v", executor.actions())
	}
}

func TestControllerDoesNotRollbackAfterCommitStarts(t *testing.T) {
	controller, executor := newTestController(t)
	executor.failAt = "commit"
	operation, err := controller.Start("ov-dash-2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	finished, err := controller.Wait(context.Background(), operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.State != StateManualIntervention || !strings.Contains(finished.LastError, "automatic rollback is forbidden") {
		t.Fatalf("finished = state %s error %q", finished.State, finished.LastError)
	}
	if slices := executor.actions(); slices[len(slices)-1] == "rollback" {
		t.Fatalf("actions = %v, rollback must not run after commit starts", slices)
	}
}

func TestControllerFaultStatesLockFutureUpdates(t *testing.T) {
	for _, faultState := range []State{StateRollbackFailed, StateManualIntervention} {
		t.Run(string(faultState), func(t *testing.T) {
			controller, _ := newTestController(t)
			operation, err := controller.store.Create(Operation{ID: "4123456789abcdef0123456789abcdef", ReleaseID: "ov-dash-2.0.0", State: StateRequested})
			if err != nil {
				t.Fatal(err)
			}
			path := []State{StateDownloaded, StateVerified, StatePreflight, StateQuiescing, StateRollingBack, faultState}
			for _, state := range path {
				operation, err = controller.store.Update(operation, func(next *Operation) error {
					next.State = state
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := controller.Start("ov-dash-2.0.0"); !errors.Is(err, ErrOperatorInterventionRequired) {
				t.Fatalf("Start() error = %v, want operator intervention lock", err)
			}
		})
	}
}

func TestControllerRestoresSnapshotWhenMigrationFails(t *testing.T) {
	controller, executor := newTestController(t)
	executor.failAt = "migrate"
	operation, err := controller.Start("ov-dash-2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	finished, err := controller.Wait(context.Background(), operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.State != StateRolledBack || !strings.Contains(finished.LastError, "migrate failed") {
		t.Fatalf("finished = state %s error %q", finished.State, finished.LastError)
	}
	actions := executor.actions()
	if actions[len(actions)-1] != "rollback" {
		t.Fatalf("actions = %v, want rollback last", actions)
	}
}

func TestControllerRecoveryUsesExecutorDecision(t *testing.T) {
	controller, executor := newTestController(t)
	store := controller.store
	operation, err := store.Create(Operation{ID: "2123456789abcdef0123456789abcdef", ReleaseID: "ov-dash-2.0.0", State: StateRequested})
	if err != nil {
		t.Fatal(err)
	}
	manifest := testManifest(executor.now)
	previous := testInstalledRelease()
	states := []State{StateDownloaded, StateVerified, StatePreflight, StateQuiescing, StateBackup, StateMigrating}
	for _, state := range states {
		operation, err = store.Update(operation, func(next *Operation) error {
			next.State = state
			next.Manifest = &manifest
			next.Previous = &previous
			next.Backup = &Backup{ID: next.ID, Path: "snapshot"}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	executor.recovery = RecoveryDecision{Action: RecoveryRollback, Reason: "ambiguous migration"}
	recovered, err := controller.Recover()
	if err != nil {
		t.Fatal(err)
	}
	finished, err := controller.Wait(context.Background(), recovered.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.State != StateRolledBack || finished.RecoveryReason != "ambiguous migration" {
		t.Fatalf("recovered = %#v", finished)
	}
}

func TestControllerRejectsConcurrentOperation(t *testing.T) {
	controller, executor := newTestController(t)
	executor.blockPreflight = make(chan struct{})
	operation, err := controller.Start("ov-dash-2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-executor.preflightStarted:
	case <-time.After(time.Second):
		t.Fatal("preflight did not start")
	}
	if _, err := controller.Start("ov-dash-2.0.0"); !errors.Is(err, ErrOperationActive) {
		t.Fatalf("second Start() error = %v", err)
	}
	close(executor.blockPreflight)
	if _, err := controller.Wait(context.Background(), operation.ID); err != nil {
		t.Fatal(err)
	}
}

type fakeExecutor struct {
	mu               sync.Mutex
	actionLog        []string
	bundle           ReleaseBundle
	current          InstalledRelease
	now              time.Time
	failAt           string
	recovery         RecoveryDecision
	blockPreflight   chan struct{}
	preflightStarted chan struct{}
	preflightOnce    sync.Once
}

func (e *fakeExecutor) action(name string) error {
	e.mu.Lock()
	e.actionLog = append(e.actionLog, name)
	e.mu.Unlock()
	if e.failAt == name {
		return errors.New(name + " failed")
	}
	return nil
}

func (e *fakeExecutor) actions() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.actionLog...)
}

func (e *fakeExecutor) Discover(context.Context) (string, error) {
	return "ov-dash-2.0.0", nil
}

func (e *fakeExecutor) Download(context.Context, string) (ReleaseBundle, error) {
	return e.bundle, e.action("download")
}
func (e *fakeExecutor) Current(context.Context) (InstalledRelease, error) {
	return e.current, e.action("current")
}
func (e *fakeExecutor) Preflight(context.Context, ReleaseManifest) error {
	if err := e.action("preflight"); err != nil {
		return err
	}
	e.preflightOnce.Do(func() { close(e.preflightStarted) })
	if e.blockPreflight != nil {
		<-e.blockPreflight
	}
	return nil
}
func (e *fakeExecutor) Quiesce(context.Context, ReleaseManifest) error {
	return e.action("quiesce")
}
func (e *fakeExecutor) Backup(context.Context, Operation) (Backup, error) {
	return Backup{ID: "backup", Path: "snapshot", CreatedAt: e.now}, e.action("backup")
}
func (e *fakeExecutor) Migrate(context.Context, Operation) error { return e.action("migrate") }
func (e *fakeExecutor) Switch(context.Context, Operation) error  { return e.action("switch") }
func (e *fakeExecutor) HealthCheck(context.Context, Operation) error {
	return e.action("health")
}
func (e *fakeExecutor) Commit(context.Context, Operation) error   { return e.action("commit") }
func (e *fakeExecutor) Rollback(context.Context, Operation) error { return e.action("rollback") }
func (e *fakeExecutor) Recover(context.Context, Operation) (RecoveryDecision, error) {
	if err := e.action("recover"); err != nil {
		return RecoveryDecision{}, err
	}
	return e.recovery, nil
}

func newTestController(t *testing.T) (*Controller, *fakeExecutor) {
	t.Helper()
	verifier, privateKey, now := newTestVerifier(t)
	executor := &fakeExecutor{
		bundle:           signedTestBundle(t, privateKey, testManifest(now)),
		current:          testInstalledRelease(),
		now:              now,
		recovery:         RecoveryDecision{Action: RecoveryResume, Reason: "resume"},
		preflightStarted: make(chan struct{}),
	}
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	controller, err := NewController(context.Background(), store, verifier, executor)
	if err != nil {
		t.Fatal(err)
	}
	return controller, executor
}
