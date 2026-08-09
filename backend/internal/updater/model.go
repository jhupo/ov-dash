package updater

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type State string

const (
	StateRequested          State = "requested"
	StateDownloaded         State = "downloaded"
	StateVerified           State = "verified"
	StatePreflight          State = "preflight"
	StateQuiescing          State = "quiescing"
	StateBackup             State = "backup"
	StateMigrating          State = "migrating"
	StateSwitching          State = "switching"
	StateHealthChecking     State = "health_checking"
	StateCommitting         State = "committing"
	StateCommitted          State = "committed"
	StateRollingBack        State = "rolling_back"
	StateRolledBack         State = "rolled_back"
	StateFailed             State = "failed"
	StateRollbackFailed     State = "rollback_failed"
	StateManualIntervention State = "manual_intervention"
)

var releaseIDPattern = regexp.MustCompile(`^ov-dash-[0-9A-Za-z][0-9A-Za-z.-]{0,79}$`)

var transitions = map[State]map[State]struct{}{
	StateRequested:      stateSet(StateDownloaded, StateFailed),
	StateDownloaded:     stateSet(StateVerified, StateFailed),
	StateVerified:       stateSet(StatePreflight, StateFailed),
	StatePreflight:      stateSet(StateQuiescing, StateFailed),
	StateQuiescing:      stateSet(StateBackup, StateRollingBack, StateManualIntervention),
	StateBackup:         stateSet(StateMigrating, StateRollingBack, StateManualIntervention),
	StateMigrating:      stateSet(StateSwitching, StateRollingBack, StateManualIntervention),
	StateSwitching:      stateSet(StateHealthChecking, StateRollingBack, StateManualIntervention),
	StateHealthChecking: stateSet(StateCommitting, StateRollingBack, StateManualIntervention),
	StateCommitting:     stateSet(StateCommitted, StateManualIntervention),
	StateRollingBack:    stateSet(StateRolledBack, StateRollbackFailed, StateManualIntervention),
}

func stateSet(states ...State) map[State]struct{} {
	result := make(map[State]struct{}, len(states))
	for _, state := range states {
		result[state] = struct{}{}
	}
	return result
}

func (s State) Terminal() bool {
	switch s {
	case StateCommitted, StateRolledBack, StateFailed, StateRollbackFailed, StateManualIntervention:
		return true
	default:
		return false
	}
}

func (s State) BlocksStartup() bool {
	return !s.Terminal() || s == StateRollbackFailed || s == StateManualIntervention
}

func (s State) RequiresOperatorIntervention() bool {
	return s == StateRollbackFailed || s == StateManualIntervention
}

func ValidateTransition(from, to State) error {
	if _, ok := transitions[from][to]; !ok {
		return fmt.Errorf("invalid updater transition %q -> %q", from, to)
	}
	return nil
}

func ValidateReleaseID(id string) error {
	if !releaseIDPattern.MatchString(id) || strings.Contains(id, "..") {
		return errors.New("release_id must be an ov-dash release identifier")
	}
	return nil
}

type ReleaseManifest struct {
	SchemaVersion  int               `json:"schema_version"`
	ReleaseID      string            `json:"release_id"`
	Version        string            `json:"version"`
	Sequence       uint64            `json:"sequence"`
	PublishedAt    time.Time         `json:"published_at"`
	ExpiresAt      time.Time         `json:"expires_at"`
	MinimumVersion string            `json:"minimum_version"`
	Images         map[string]string `json:"images"`
	Database       DatabaseRelease   `json:"database"`
	Health         HealthPolicy      `json:"health"`
}

type DatabaseRelease struct {
	FromSchema     uint64 `json:"from_schema"`
	ToSchema       uint64 `json:"to_schema"`
	Strategy       string `json:"strategy"`
	Transactional  bool   `json:"transactional"`
	BackupRequired bool   `json:"backup_required"`
}

type HealthPolicy struct {
	TimeoutSeconds   int `json:"timeout_seconds"`
	StabilitySeconds int `json:"stability_seconds"`
}

type ReleaseBundle struct {
	Manifest  []byte `json:"manifest"`
	Signature []byte `json:"signature"`
}

type CheckResult struct {
	Current   InstalledRelease `json:"current"`
	Candidate ReleaseManifest  `json:"candidate"`
	HasUpdate bool             `json:"has_update"`
}

type InstalledRelease struct {
	ReleaseID   string            `json:"release_id"`
	Version     string            `json:"version"`
	Sequence    uint64            `json:"sequence"`
	Schema      uint64            `json:"schema"`
	Images      map[string]string `json:"images"`
	ReleaseEnv  string            `json:"release_env,omitempty"`
	CommittedAt time.Time         `json:"committed_at"`
}

func (r InstalledRelease) Empty() bool {
	return r.ReleaseID == "" && r.Version == "" && r.Sequence == 0 && r.Schema == 0
}

type Backup struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
}

type Operation struct {
	ID             string            `json:"id"`
	ReleaseID      string            `json:"release_id"`
	State          State             `json:"state"`
	Revision       uint64            `json:"revision"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	Bundle         *ReleaseBundle    `json:"bundle,omitempty"`
	Manifest       *ReleaseManifest  `json:"manifest,omitempty"`
	Previous       *InstalledRelease `json:"previous,omitempty"`
	Backup         *Backup           `json:"backup,omitempty"`
	LastError      string            `json:"last_error,omitempty"`
	RecoveryReason string            `json:"recovery_reason,omitempty"`
}

type Event struct {
	OperationID string    `json:"operation_id"`
	Revision    uint64    `json:"revision"`
	Previous    State     `json:"previous,omitempty"`
	State       State     `json:"state"`
	RecordedAt  time.Time `json:"recorded_at"`
	Operation   Operation `json:"operation"`
}

type RecoveryAction string

const (
	RecoveryResume   RecoveryAction = "resume"
	RecoveryRollback RecoveryAction = "rollback"
	RecoveryCommit   RecoveryAction = "commit"
	RecoveryManual   RecoveryAction = "manual_intervention"
)

type RecoveryDecision struct {
	Action RecoveryAction `json:"action"`
	Reason string         `json:"reason"`
}

func (d RecoveryDecision) Validate(state State) error {
	switch d.Action {
	case RecoveryResume, RecoveryRollback, RecoveryManual:
		return nil
	case RecoveryCommit:
		if state != StateCommitting {
			return errors.New("commit recovery is only valid during committing")
		}
		return nil
	default:
		return fmt.Errorf("unknown recovery action %q", d.Action)
	}
}
