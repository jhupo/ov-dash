package updater

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Controller struct {
	ctx      context.Context
	store    *Store
	verifier *Verifier
	executor Executor
	lockPath string

	mu      sync.Mutex
	waiters map[string]chan struct{}
}

func NewController(ctx context.Context, store *Store, verifier *Verifier, executor Executor) (*Controller, error) {
	if ctx == nil || store == nil || verifier == nil || executor == nil {
		return nil, errors.New("controller context, store, verifier, and executor are required")
	}
	return &Controller{
		ctx:      ctx,
		store:    store,
		verifier: verifier,
		executor: executor,
		lockPath: filepath.Join(store.Root(), "active.lock"),
		waiters:  make(map[string]chan struct{}),
	}, nil
}

func (c *Controller) Start(releaseID string) (Operation, error) {
	if err := ValidateReleaseID(releaseID); err != nil {
		return Operation{}, err
	}
	id, err := newOperationID()
	if err != nil {
		return Operation{}, err
	}
	lock, err := acquireOperationLock(c.lockPath, id)
	if err != nil {
		return Operation{}, err
	}
	active, err := c.store.Active()
	if err != nil {
		_ = lock.Release()
		return Operation{}, err
	}
	if len(active) != 0 {
		_ = lock.Release()
		for _, operation := range active {
			if operation.State.RequiresOperatorIntervention() {
				return Operation{}, fmt.Errorf("%w: operation %s is %s", ErrOperatorInterventionRequired, operation.ID, operation.State)
			}
		}
		return Operation{}, ErrOperationActive
	}
	operation, err := c.store.Create(Operation{ID: id, ReleaseID: releaseID, State: StateRequested})
	if err != nil {
		_ = lock.Release()
		return Operation{}, err
	}
	c.launch(operation, lock, false)
	return operation, nil
}

func (c *Controller) Check(ctx context.Context) (CheckResult, error) {
	releaseID, err := c.executor.Discover(ctx)
	if err != nil {
		return CheckResult{}, fmt.Errorf("discover latest release: %w", err)
	}
	current, err := c.executor.Current(ctx)
	if err != nil {
		return CheckResult{}, fmt.Errorf("read installed release: %w", err)
	}
	if current.Empty() {
		return CheckResult{}, errors.New("online update requires an installed release state")
	}
	bundle, err := c.executor.Download(ctx, releaseID)
	if err != nil {
		return CheckResult{}, fmt.Errorf("download latest release: %w", err)
	}
	candidate, err := c.verifier.Inspect(bundle, releaseID)
	if err != nil {
		return CheckResult{}, err
	}
	currentVersion, err := parseSemanticVersion(current.Version)
	if err != nil {
		return CheckResult{}, errors.New("installed release has an invalid version")
	}
	candidateVersion, err := parseSemanticVersion(candidate.Version)
	if err != nil {
		return CheckResult{}, errors.New("candidate release has an invalid version")
	}
	sequenceOrder := 0
	if candidate.Sequence < current.Sequence {
		sequenceOrder = -1
	} else if candidate.Sequence > current.Sequence {
		sequenceOrder = 1
	}
	versionOrder := compareSemanticVersions(candidateVersion, currentVersion)
	if (sequenceOrder > 0) != (versionOrder > 0) && sequenceOrder != 0 && versionOrder != 0 {
		return CheckResult{}, errors.New("release sequence and version ordering disagree")
	}
	hasUpdate := sequenceOrder > 0 && versionOrder > 0
	if hasUpdate {
		if err := c.verifier.validateUpgrade(candidate, current); err != nil {
			return CheckResult{}, err
		}
	}
	return CheckResult{Current: current, Candidate: candidate, HasUpdate: hasUpdate}, nil
}

func (c *Controller) Current(ctx context.Context) (InstalledRelease, error) {
	return c.executor.Current(ctx)
}

func (c *Controller) Recover() (*Operation, error) {
	lock, err := acquireOperationLock(c.lockPath, "recovery")
	if err != nil {
		return nil, err
	}
	active, err := c.store.Active()
	if err != nil {
		_ = lock.Release()
		return nil, err
	}
	if len(active) == 0 {
		_ = lock.Release()
		return nil, nil
	}
	if len(active) > 1 {
		_ = lock.Release()
		return nil, errors.New("multiple active update operations require manual repair")
	}
	operation := active[0]
	if operation.State.RequiresOperatorIntervention() {
		_ = lock.Release()
		return &operation, nil
	}
	c.launch(operation, lock, true)
	return &operation, nil
}

func (c *Controller) Get(id string) (Operation, error) {
	return c.store.Get(id)
}

func (c *Controller) Events(id string) ([]Event, error) {
	return c.store.Events(id)
}

func (c *Controller) Active() (*Operation, error) {
	active, err := c.store.Active()
	if err != nil {
		return nil, err
	}
	if len(active) == 0 {
		return nil, nil
	}
	if len(active) > 1 {
		return nil, errors.New("multiple active update operations found")
	}
	return &active[0], nil
}

func (c *Controller) Wait(ctx context.Context, id string) (Operation, error) {
	for {
		operation, err := c.store.Get(id)
		if err != nil {
			return Operation{}, err
		}
		if operation.State.Terminal() {
			return operation, nil
		}
		c.mu.Lock()
		waiter := c.waiters[id]
		c.mu.Unlock()
		if waiter == nil {
			select {
			case <-ctx.Done():
				return Operation{}, ctx.Err()
			case <-time.After(10 * time.Millisecond):
			}
			continue
		}
		select {
		case <-ctx.Done():
			return Operation{}, ctx.Err()
		case <-waiter:
		}
	}
}

func (c *Controller) launch(operation Operation, lock operationLock, recovering bool) {
	c.mu.Lock()
	waiter := make(chan struct{})
	c.waiters[operation.ID] = waiter
	c.mu.Unlock()
	go func() {
		defer func() {
			_ = lock.Release()
			c.mu.Lock()
			close(waiter)
			delete(c.waiters, operation.ID)
			c.mu.Unlock()
		}()
		_ = c.run(c.ctx, operation, recovering)
	}()
}

func (c *Controller) run(ctx context.Context, operation Operation, recovering bool) error {
	if recovering {
		var err error
		operation, err = c.applyRecoveryDecision(ctx, operation)
		if err != nil || operation.State.Terminal() {
			return err
		}
	}
	for !operation.State.Terminal() {
		var err error
		switch operation.State {
		case StateRequested:
			var bundle ReleaseBundle
			bundle, err = c.executor.Download(ctx, operation.ReleaseID)
			if err == nil {
				operation, err = c.update(operation, StateDownloaded, func(next *Operation) { next.Bundle = &bundle })
			}
		case StateDownloaded:
			if operation.Bundle == nil {
				err = errors.New("downloaded operation is missing its release bundle")
				break
			}
			var current InstalledRelease
			current, err = c.executor.Current(ctx)
			if err != nil {
				break
			}
			var manifest ReleaseManifest
			manifest, err = c.verifier.Verify(*operation.Bundle, operation.ReleaseID, current)
			if err == nil {
				operation, err = c.update(operation, StateVerified, func(next *Operation) {
					next.Manifest = &manifest
					next.Previous = &current
				})
			}
		case StateVerified:
			operation, err = c.transition(operation, StatePreflight)
		case StatePreflight:
			err = c.requireManifest(operation)
			if err == nil {
				err = c.executor.Preflight(ctx, *operation.Manifest)
			}
			if err == nil {
				operation, err = c.transition(operation, StateQuiescing)
			}
		case StateQuiescing:
			err = c.executor.Quiesce(ctx, *operation.Manifest)
			if err == nil {
				operation, err = c.transition(operation, StateBackup)
			}
		case StateBackup:
			var backup Backup
			backup, err = c.executor.Backup(ctx, operation)
			if err == nil {
				operation, err = c.update(operation, StateMigrating, func(next *Operation) { next.Backup = &backup })
			}
		case StateMigrating:
			err = c.executor.Migrate(ctx, operation)
			if err == nil {
				operation, err = c.transition(operation, StateSwitching)
			}
		case StateSwitching:
			err = c.executor.Switch(ctx, operation)
			if err == nil {
				operation, err = c.transition(operation, StateHealthChecking)
			}
		case StateHealthChecking:
			err = c.executor.HealthCheck(ctx, operation)
			if err == nil {
				operation, err = c.transition(operation, StateCommitting)
			}
		case StateCommitting:
			err = c.executor.Commit(ctx, operation)
			if err == nil {
				operation, err = c.transition(operation, StateCommitted)
			}
		case StateRollingBack:
			err = c.executor.Rollback(ctx, operation)
			if err == nil {
				operation, err = c.transition(operation, StateRolledBack)
			} else {
				operation, _ = c.failTransition(operation, StateRollbackFailed, err)
				return err
			}
		default:
			return fmt.Errorf("cannot execute operation in state %q", operation.State)
		}
		if err != nil {
			if operation.State == StateCommitting {
				_, transitionErr := c.failTransition(operation, StateManualIntervention, fmt.Errorf("commit may have reopened writes; automatic rollback is forbidden: %w", err))
				return errors.Join(err, transitionErr)
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return c.handlePhaseFailure(ctx, operation, err)
		}
	}
	return nil
}

func (c *Controller) applyRecoveryDecision(ctx context.Context, operation Operation) (Operation, error) {
	switch operation.State {
	case StateRequested, StateDownloaded, StateVerified, StatePreflight:
		return operation, nil
	case StateRollingBack:
		return operation, nil
	}
	decision, err := c.executor.Recover(ctx, operation)
	if err != nil {
		return c.failTransition(operation, StateManualIntervention, fmt.Errorf("inspect interrupted update: %w", err))
	}
	if err := decision.Validate(operation.State); err != nil {
		return c.failTransition(operation, StateManualIntervention, err)
	}
	switch decision.Action {
	case RecoveryResume:
		return c.annotateRecovery(operation, decision.Reason)
	case RecoveryRollback:
		return c.update(operation, StateRollingBack, func(next *Operation) { next.RecoveryReason = cleanError(decision.Reason) })
	case RecoveryCommit:
		if err := c.executor.Commit(ctx, operation); err != nil {
			return c.failTransition(operation, StateManualIntervention, fmt.Errorf("commit may have reopened writes; automatic rollback is forbidden: %w", err))
		}
		return c.update(operation, StateCommitted, func(next *Operation) { next.RecoveryReason = cleanError(decision.Reason) })
	case RecoveryManual:
		return c.update(operation, StateManualIntervention, func(next *Operation) { next.RecoveryReason = cleanError(decision.Reason) })
	default:
		return Operation{}, errors.New("invalid recovery decision")
	}
}

func (c *Controller) handlePhaseFailure(ctx context.Context, operation Operation, phaseErr error) error {
	if operation.State == StateQuiescing || operation.State == StateBackup || operation.State == StateMigrating || operation.State == StateSwitching || operation.State == StateHealthChecking {
		rollingBack, err := c.failTransition(operation, StateRollingBack, phaseErr)
		if err != nil {
			return errors.Join(phaseErr, err)
		}
		if err := c.executor.Rollback(ctx, rollingBack); err != nil {
			_, transitionErr := c.failTransition(rollingBack, StateRollbackFailed, err)
			return errors.Join(phaseErr, err, transitionErr)
		}
		_, err = c.transition(rollingBack, StateRolledBack)
		return errors.Join(phaseErr, err)
	}
	_, err := c.failTransition(operation, StateFailed, phaseErr)
	return errors.Join(phaseErr, err)
}

func (c *Controller) transition(operation Operation, state State) (Operation, error) {
	return c.update(operation, state, nil)
}

func (c *Controller) failTransition(operation Operation, state State, cause error) (Operation, error) {
	return c.update(operation, state, func(next *Operation) { next.LastError = cleanError(cause.Error()) })
}

func (c *Controller) annotateRecovery(operation Operation, reason string) (Operation, error) {
	return c.store.Update(operation, func(next *Operation) error {
		next.RecoveryReason = cleanError(reason)
		return nil
	})
}

func (c *Controller) update(operation Operation, state State, mutate func(*Operation)) (Operation, error) {
	return c.store.Update(operation, func(next *Operation) error {
		next.State = state
		if mutate != nil {
			mutate(next)
		}
		return nil
	})
}

func (c *Controller) requireManifest(operation Operation) error {
	if operation.Manifest == nil {
		return errors.New("verified operation is missing its manifest")
	}
	return nil
}

func cleanError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 2048 {
		return value[:2048] + " [truncated]"
	}
	return value
}

func newOperationID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate operation id: %w", err)
	}
	return hex.EncodeToString(random[:]), nil
}
