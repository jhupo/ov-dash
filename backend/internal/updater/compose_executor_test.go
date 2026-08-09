package updater

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestComposeExecutorReleaseEnvironmentsIncludeVersionAndArtifacts(t *testing.T) {
	root := t.TempDir()
	executor, err := newTestComposeExecutor(root, &snapshotRunner{})
	if err != nil {
		t.Fatal(err)
	}

	manifest := ReleaseManifest{
		ReleaseID: "ov-dash-2.0.0",
		Version:   "2.0.0",
		Images: map[string]string{
			"backend": "registry.example/ov-dash/backend@sha256:current",
		},
		Health: HealthPolicy{TimeoutSeconds: 30},
	}
	stagedPath, err := executor.writeStagedEnvironment(manifest)
	if err != nil {
		t.Fatal(err)
	}
	assertEnvironmentFile(t, stagedPath, "APP_VERSION=2.0.0\nBACKEND_IMAGE=registry.example/ov-dash/backend@sha256:current\n")

	if err := executor.Switch(context.Background(), Operation{Manifest: &manifest}); err != nil {
		t.Fatal(err)
	}
	assertEnvironmentFile(t, executor.currentEnvironmentPath(), "APP_VERSION=2.0.0\nBACKEND_IMAGE=registry.example/ov-dash/backend@sha256:current\n")

	rollbackContext, cancelRollback := context.WithCancel(context.Background())
	cancelRollback()
	rollbackErr := executor.Rollback(rollbackContext, Operation{Previous: &InstalledRelease{
		ReleaseID: "ov-dash-1.9.0",
		Version:   "1.9.0",
		Images: map[string]string{
			"backend": "registry.example/ov-dash/backend@sha256:previous",
		},
	}})
	if !errors.Is(rollbackErr, context.Canceled) {
		t.Fatalf("rollback error = %v, want context canceled after environment activation", rollbackErr)
	}
	assertEnvironmentFile(t, executor.currentEnvironmentPath(), "APP_VERSION=1.9.0\nBACKEND_IMAGE=registry.example/ov-dash/backend@sha256:previous\n")
}

func TestComposeExecutorCreatesAndRestoresDatabaseSnapshotWithoutShell(t *testing.T) {
	root := t.TempDir()
	runner := &snapshotRunner{}
	executor, err := newTestComposeExecutor(root, runner)
	if err != nil {
		t.Fatal(err)
	}

	backup, err := executor.Backup(context.Background(), Operation{ID: "3123456789abcdef0123456789abcdef"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(backup.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "database-snapshot" {
		t.Fatalf("snapshot = %q", data)
	}
	if err := executor.restoreDatabase(context.Background(), backup); err != nil {
		t.Fatal(err)
	}

	commands := runner.commands()
	if len(commands) != 2 || commands[0].Name != "docker" || commands[1].Name != "docker" {
		t.Fatalf("commands = %#v", commands)
	}
	if !slices.Contains(commands[0].Args, "pg_dump") || !slices.Contains(commands[0].Args, "--format=custom") {
		t.Fatalf("backup args = %v", commands[0].Args)
	}
	if !slices.Contains(commands[1].Args, "pg_restore") || !slices.Contains(commands[1].Args, "--clean") || !slices.Contains(commands[1].Args, "--create") {
		t.Fatalf("restore args = %v", commands[1].Args)
	}
	if runner.restored != "database-snapshot" {
		t.Fatalf("restored input = %q", runner.restored)
	}
}

func TestComposeExecutorCommitFailureContainsResumedServices(t *testing.T) {
	runner := &scriptedRunner{failAt: 1}
	executor, err := newTestComposeExecutor(t.TempDir(), runner)
	if err != nil {
		t.Fatal(err)
	}
	manifest := ReleaseManifest{
		ReleaseID: "ov-dash-2.0.0",
		Version:   "2.0.0",
		Sequence:  20,
		Images: map[string]string{
			"backend": "registry.example/ov-dash/backend@sha256:current",
		},
		Database: DatabaseRelease{ToSchema: 11},
		Health:   HealthPolicy{TimeoutSeconds: 30},
	}

	err = executor.Commit(context.Background(), Operation{Manifest: &manifest})
	if err == nil || !strings.Contains(err.Error(), "resume application services") {
		t.Fatalf("Commit() error = %v", err)
	}
	commands := runner.commands()
	if len(commands) != 2 || !slices.Contains(commands[0].Args, "up") || !slices.Contains(commands[1].Args, "stop") || !slices.Contains(commands[1].Args, "45") {
		t.Fatalf("commands = %#v", commands)
	}
	if _, statErr := os.Stat(executor.deploymentPath()); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("deployment state written before successful commit: %v", statErr)
	}
}

func newTestComposeExecutor(root string, runner CommandRunner) (*ComposeExecutor, error) {
	return NewComposeExecutor(ComposeExecutorConfig{
		StateDir:            root,
		WorkDir:             root,
		ComposeFile:         filepath.Join(root, "compose.yml"),
		BaseEnvFile:         filepath.Join(root, ".env"),
		ProjectName:         "ov-dash",
		DockerBinary:        "docker",
		ReleaseEnvFile:      filepath.Join(root, "current.env"),
		ArtifactEnvironment: map[string]string{"backend": "BACKEND_IMAGE"},
		ServiceArtifacts:    map[string]string{"api": "backend", "worker": "backend", "migrate": "backend"},
		ApplicationServices: []string{"api", "worker"},
		ValidationServices:  []string{"api"},
		ResumeServices:      []string{"worker"},
		MigrationService:    "migrate",
		DatabaseService:     "postgres",
		DatabaseName:        "ov_dash",
		DatabaseUser:        "ov_dash",
		ValidationHealthURL: "http://127.0.0.1:8080/readyz?scope=api",
		HealthURL:           "http://127.0.0.1:8080/readyz",
	}, staticReleaseSource{}, runner, &http.Client{})
}

func assertEnvironmentFile(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != expected {
		t.Fatalf("environment %s = %q, want %q", path, data, expected)
	}
}

type staticReleaseSource struct{}

func (staticReleaseSource) Latest(context.Context) (string, error) {
	return "ov-dash-2.0.0", nil
}

func (staticReleaseSource) Download(context.Context, string) (ReleaseBundle, error) {
	return ReleaseBundle{}, nil
}

type snapshotRunner struct {
	mu       sync.Mutex
	seen     []Command
	restored string
}

type scriptedRunner struct {
	mu     sync.Mutex
	seen   []Command
	failAt int
}

func (r *scriptedRunner) Run(_ context.Context, command Command) (CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, command)
	if len(r.seen) == r.failAt {
		return CommandResult{}, fmt.Errorf("scripted command failure")
	}
	return CommandResult{}, nil
}

func (r *scriptedRunner) RunStream(context.Context, Command, io.Reader, io.Writer) (CommandResult, error) {
	return CommandResult{}, errors.New("unexpected stream command")
}

func (r *scriptedRunner) commands() []Command {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Command(nil), r.seen...)
}

func (r *snapshotRunner) Run(context.Context, Command) (CommandResult, error) {
	return CommandResult{}, nil
}

func (r *snapshotRunner) RunStream(_ context.Context, command Command, stdin io.Reader, stdout io.Writer) (CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, command)
	if stdin == nil {
		_, _ = io.WriteString(stdout, "database-snapshot")
		return CommandResult{}, nil
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return CommandResult{}, err
	}
	r.restored = string(data)
	return CommandResult{}, nil
}

func (r *snapshotRunner) commands() []Command {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Command(nil), r.seen...)
}
