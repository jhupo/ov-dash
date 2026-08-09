package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var environmentNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

type ComposeExecutorConfig struct {
	StateDir            string
	WorkDir             string
	ComposeFile         string
	BaseEnvFile         string
	ProjectName         string
	DockerBinary        string
	ReleaseEnvFile      string
	ArtifactEnvironment map[string]string
	ServiceArtifacts    map[string]string
	ApplicationServices []string
	ValidationServices  []string
	ResumeServices      []string
	MigrationService    string
	DatabaseService     string
	DatabaseName        string
	DatabaseUser        string
	ValidationHealthURL string
	HealthURL           string
	ReleaseHeader       string
	CommandTimeout      time.Duration
}

type ComposeExecutor struct {
	config ComposeExecutorConfig
	source ReleaseSource
	runner CommandRunner
	client *http.Client
}

func NewComposeExecutor(config ComposeExecutorConfig, source ReleaseSource, runner CommandRunner, client *http.Client) (*ComposeExecutor, error) {
	if source == nil || runner == nil {
		return nil, errors.New("release source and command runner are required")
	}
	if err := validateComposeExecutorConfig(config); err != nil {
		return nil, err
	}
	if config.DockerBinary == "" {
		config.DockerBinary = "docker"
	}
	if config.ReleaseHeader == "" {
		config.ReleaseHeader = "X-OV-Dash-Release"
	}
	if config.CommandTimeout <= 0 {
		config.CommandTimeout = 15 * time.Minute
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &ComposeExecutor{config: config, source: source, runner: runner, client: client}, nil
}

func (e *ComposeExecutor) Discover(ctx context.Context) (string, error) {
	return e.source.Latest(ctx)
}

func (e *ComposeExecutor) Download(ctx context.Context, releaseID string) (ReleaseBundle, error) {
	return e.source.Download(ctx, releaseID)
}

func (e *ComposeExecutor) Current(context.Context) (InstalledRelease, error) {
	data, err := os.ReadFile(e.deploymentPath())
	if errors.Is(err, os.ErrNotExist) {
		return InstalledRelease{}, nil
	}
	if err != nil {
		return InstalledRelease{}, fmt.Errorf("read installed release: %w", err)
	}
	var release InstalledRelease
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&release); err != nil {
		return InstalledRelease{}, fmt.Errorf("decode installed release: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return InstalledRelease{}, err
	}
	if release.Empty() {
		return InstalledRelease{}, errors.New("installed release state is incomplete")
	}
	if err := ValidateReleaseID(release.ReleaseID); err != nil {
		return InstalledRelease{}, errors.New("installed release state is invalid")
	}
	return release, nil
}

func (e *ComposeExecutor) Preflight(ctx context.Context, manifest ReleaseManifest) error {
	stagedEnv, err := e.writeStagedEnvironment(manifest)
	if err != nil {
		return err
	}
	if _, err := e.run(ctx, Command{Name: e.config.DockerBinary, Args: []string{"version"}, Dir: e.config.WorkDir}); err != nil {
		return fmt.Errorf("docker preflight: %w", err)
	}
	if _, err := e.run(ctx, Command{Name: e.config.DockerBinary, Args: []string{"compose", "version"}, Dir: e.config.WorkDir}); err != nil {
		return fmt.Errorf("compose preflight: %w", err)
	}
	services := sortedKeys(e.config.ServiceArtifacts)
	args := append(e.composeArgs(stagedEnv), "pull")
	args = append(args, services...)
	if _, err := e.run(ctx, Command{Name: e.config.DockerBinary, Args: args, Dir: e.config.WorkDir}); err != nil {
		return fmt.Errorf("stage release images: %w", err)
	}
	return nil
}

func (e *ComposeExecutor) Quiesce(ctx context.Context, _ ReleaseManifest) error {
	args := append(e.composeArgs(e.currentEnvironmentPath()), "stop", "--timeout", "45")
	args = append(args, e.config.ApplicationServices...)
	if _, err := e.run(ctx, Command{Name: e.config.DockerBinary, Args: args, Dir: e.config.WorkDir}); err != nil {
		return fmt.Errorf("quiesce application services: %w", err)
	}
	return nil
}

func (e *ComposeExecutor) Backup(ctx context.Context, operation Operation) (Backup, error) {
	dir := filepath.Join(e.config.StateDir, "backups", operation.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Backup{}, fmt.Errorf("create backup directory: %w", err)
	}
	path := filepath.Join(dir, "database.dump")
	temp, err := os.CreateTemp(dir, ".database-*.dump")
	if err != nil {
		return Backup{}, err
	}
	tempName := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempName)
	}
	args := append(e.composeArgs(e.currentEnvironmentPath()), "exec", "-T", e.config.DatabaseService,
		"pg_dump", "--username", e.config.DatabaseUser, "--format=custom", "--no-owner", "--no-privileges", e.config.DatabaseName)
	commandCtx, cancel := context.WithTimeout(ctx, e.config.CommandTimeout)
	_, runErr := e.runner.RunStream(commandCtx, Command{Name: e.config.DockerBinary, Args: args, Dir: e.config.WorkDir}, nil, temp)
	cancel()
	if runErr != nil {
		cleanup()
		return Backup{}, fmt.Errorf("create database snapshot: %w", runErr)
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return Backup{}, err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempName)
		return Backup{}, err
	}
	if err := os.Rename(tempName, path); err != nil {
		_ = os.Remove(tempName)
		return Backup{}, err
	}
	if err := syncDirectory(dir); err != nil {
		return Backup{}, err
	}
	return Backup{ID: operation.ID, Path: path, CreatedAt: time.Now().UTC()}, nil
}

func (e *ComposeExecutor) Migrate(ctx context.Context, operation Operation) error {
	if operation.Manifest == nil {
		return errors.New("migration manifest is missing")
	}
	args := append(e.composeArgs(e.stagedEnvironmentPath(operation.Manifest.ReleaseID)), "run", "--rm", "--no-deps", e.config.MigrationService)
	if _, err := e.run(ctx, Command{Name: e.config.DockerBinary, Args: args, Dir: e.config.WorkDir}); err != nil {
		return fmt.Errorf("run database migration: %w", err)
	}
	return nil
}

func (e *ComposeExecutor) Switch(ctx context.Context, operation Operation) error {
	if operation.Manifest == nil {
		return errors.New("switch manifest is missing")
	}
	staged, err := os.ReadFile(e.stagedEnvironmentPath(operation.Manifest.ReleaseID))
	if err != nil {
		return fmt.Errorf("read staged release environment: %w", err)
	}
	if err := atomicWrite(e.currentEnvironmentPath(), staged, 0o600); err != nil {
		return fmt.Errorf("activate release environment: %w", err)
	}
	args := append(e.composeArgs(e.currentEnvironmentPath()), "up", "-d", "--no-build")
	args = append(args, e.config.ValidationServices...)
	if _, err := e.run(ctx, Command{Name: e.config.DockerBinary, Args: args, Dir: e.config.WorkDir}); err != nil {
		return fmt.Errorf("switch application services: %w", err)
	}
	return nil
}

func (e *ComposeExecutor) HealthCheck(ctx context.Context, operation Operation) error {
	if operation.Manifest == nil {
		return errors.New("health manifest is missing")
	}
	return e.waitForHealth(ctx, e.config.ValidationHealthURL, operation.Manifest.ReleaseID, operation.Manifest.Health)
}

func (e *ComposeExecutor) Commit(ctx context.Context, operation Operation) error {
	if operation.Manifest == nil {
		return errors.New("commit manifest is missing")
	}
	args := append(e.composeArgs(e.currentEnvironmentPath()), "up", "-d", "--no-build", "--wait", "--wait-timeout", fmt.Sprint(operation.Manifest.Health.TimeoutSeconds))
	args = append(args, e.config.ResumeServices...)
	if _, err := e.run(ctx, Command{Name: e.config.DockerBinary, Args: args, Dir: e.config.WorkDir}); err != nil {
		return e.containCommitFailure(fmt.Errorf("resume application services: %w", err))
	}
	if err := e.waitForHealth(ctx, e.config.HealthURL, operation.Manifest.ReleaseID, operation.Manifest.Health); err != nil {
		return e.containCommitFailure(fmt.Errorf("validate complete application stack: %w", err))
	}
	current := InstalledRelease{
		ReleaseID:   operation.Manifest.ReleaseID,
		Version:     operation.Manifest.Version,
		Sequence:    operation.Manifest.Sequence,
		Schema:      operation.Manifest.Database.ToSchema,
		Images:      cloneStrings(operation.Manifest.Images),
		ReleaseEnv:  e.currentEnvironmentPath(),
		CommittedAt: time.Now().UTC(),
	}
	data, err := json.Marshal(current)
	if err != nil {
		return err
	}
	if err := atomicWrite(e.deploymentPath(), append(data, '\n'), 0o600); err != nil {
		return e.containCommitFailure(fmt.Errorf("persist installed release: %w", err))
	}
	return nil
}

func (e *ComposeExecutor) containCommitFailure(cause error) error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	args := append(e.composeArgs(e.currentEnvironmentPath()), "stop", "--timeout", "45")
	args = append(args, e.config.ResumeServices...)
	_, cleanupErr := e.run(cleanupCtx, Command{Name: e.config.DockerBinary, Args: args, Dir: e.config.WorkDir})
	if cleanupErr != nil {
		return errors.Join(cause, fmt.Errorf("containment failed while stopping resumed services: %w", cleanupErr))
	}
	return cause
}

func (e *ComposeExecutor) Rollback(ctx context.Context, operation Operation) error {
	if operation.Previous == nil || operation.Previous.Empty() {
		return errors.New("cannot roll back without a previous installed release")
	}
	stopArgs := append(e.composeArgs(e.currentEnvironmentPath()), "stop", "--timeout", "45")
	stopArgs = append(stopArgs, e.config.ApplicationServices...)
	if _, err := e.run(ctx, Command{Name: e.config.DockerBinary, Args: stopArgs, Dir: e.config.WorkDir}); err != nil {
		return fmt.Errorf("stop failed release: %w", err)
	}
	if operation.Backup != nil {
		if err := e.restoreDatabase(ctx, *operation.Backup); err != nil {
			return err
		}
	}
	previousEnv, err := e.environmentData(operation.Previous.Version, operation.Previous.Images)
	if err != nil {
		return fmt.Errorf("build rollback environment: %w", err)
	}
	if err := atomicWrite(e.currentEnvironmentPath(), previousEnv, 0o600); err != nil {
		return fmt.Errorf("restore release environment: %w", err)
	}
	args := append(e.composeArgs(e.currentEnvironmentPath()), "up", "-d", "--no-build", "--wait")
	args = append(args, e.config.ApplicationServices...)
	if _, err := e.run(ctx, Command{Name: e.config.DockerBinary, Args: args, Dir: e.config.WorkDir}); err != nil {
		return fmt.Errorf("restore previous application services: %w", err)
	}
	policy := HealthPolicy{TimeoutSeconds: 180, StabilitySeconds: 15}
	if err := e.waitForHealth(ctx, e.config.HealthURL, operation.Previous.ReleaseID, policy); err != nil {
		return fmt.Errorf("verify rolled back release: %w", err)
	}
	return e.writeInstalledRelease(*operation.Previous)
}

func (e *ComposeExecutor) Recover(_ context.Context, operation Operation) (RecoveryDecision, error) {
	switch operation.State {
	case StateQuiescing, StateBackup:
		return RecoveryDecision{Action: RecoveryResume, Reason: "safe phase will be retried idempotently"}, nil
	case StateMigrating, StateSwitching:
		return RecoveryDecision{Action: RecoveryRollback, Reason: "database or service switch outcome is ambiguous"}, nil
	case StateHealthChecking:
		return RecoveryDecision{Action: RecoveryResume, Reason: "target release health will be verified again"}, nil
	case StateCommitting:
		return RecoveryDecision{Action: RecoveryCommit, Reason: "resume may have reopened writes; complete commit without database rollback"}, nil
	case StateRollingBack:
		return RecoveryDecision{Action: RecoveryResume, Reason: "rollback will be retried"}, nil
	default:
		return RecoveryDecision{Action: RecoveryManual, Reason: "state has no automatic recovery rule"}, nil
	}
}

func (e *ComposeExecutor) restoreDatabase(ctx context.Context, backup Backup) error {
	file, err := os.Open(backup.Path)
	if err != nil {
		return fmt.Errorf("open database snapshot: %w", err)
	}
	defer file.Close()
	args := append(e.composeArgs(e.currentEnvironmentPath()), "exec", "-T", e.config.DatabaseService,
		"pg_restore", "--username", e.config.DatabaseUser, "--clean", "--if-exists", "--create", "--no-owner", "--no-privileges", "--dbname", "postgres")
	commandCtx, cancel := context.WithTimeout(ctx, e.config.CommandTimeout)
	defer cancel()
	if _, err := e.runner.RunStream(commandCtx, Command{Name: e.config.DockerBinary, Args: args, Dir: e.config.WorkDir}, file, io.Discard); err != nil {
		return fmt.Errorf("restore database snapshot: %w", err)
	}
	return nil
}

func (e *ComposeExecutor) waitForHealth(ctx context.Context, healthURL, releaseID string, policy HealthPolicy) error {
	deadline := time.Now().Add(time.Duration(policy.TimeoutSeconds) * time.Second)
	var stableSince time.Time
	for time.Now().Before(deadline) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			return err
		}
		response, err := e.client.Do(request)
		healthy := err == nil && response.StatusCode >= 200 && response.StatusCode < 300 && response.Header.Get(e.config.ReleaseHeader) == releaseID
		if response != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
			_ = response.Body.Close()
		}
		if healthy {
			if stableSince.IsZero() {
				stableSince = time.Now()
			}
			if time.Since(stableSince) >= time.Duration(policy.StabilitySeconds)*time.Second {
				return nil
			}
		} else {
			stableSince = time.Time{}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return errors.New("release did not pass the health stability window")
}

func (e *ComposeExecutor) run(ctx context.Context, command Command) (CommandResult, error) {
	commandCtx, cancel := context.WithTimeout(ctx, e.config.CommandTimeout)
	defer cancel()
	return e.runner.Run(commandCtx, command)
}

func (e *ComposeExecutor) composeArgs(environmentPath string) []string {
	args := []string{"compose", "--project-name", e.config.ProjectName, "--file", e.config.ComposeFile}
	args = append(args, "--env-file", e.config.BaseEnvFile)
	if environmentPath != "" {
		args = append(args, "--env-file", environmentPath)
	}
	return args
}

func (e *ComposeExecutor) writeStagedEnvironment(manifest ReleaseManifest) (string, error) {
	data, err := e.environmentData(manifest.Version, manifest.Images)
	if err != nil {
		return "", err
	}
	path := e.stagedEnvironmentPath(manifest.ReleaseID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := atomicWrite(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (e *ComposeExecutor) environmentData(version string, images map[string]string) ([]byte, error) {
	values := make(map[string]string, len(e.config.ArtifactEnvironment)+1)
	for artifact, variable := range e.config.ArtifactEnvironment {
		image, ok := images[artifact]
		if !ok {
			return nil, fmt.Errorf("release image %q is missing", artifact)
		}
		if previous, exists := values[variable]; exists && previous != image {
			return nil, fmt.Errorf("artifacts sharing %s have different image digests", variable)
		}
		values[variable] = image
	}
	if _, exists := values["APP_VERSION"]; exists {
		return nil, errors.New("APP_VERSION is reserved for the release version")
	}
	values["APP_VERSION"] = version
	keys := sortedKeys(values)
	var result strings.Builder
	for _, key := range keys {
		result.WriteString(key)
		result.WriteByte('=')
		result.WriteString(values[key])
		result.WriteByte('\n')
	}
	return []byte(result.String()), nil
}

func (e *ComposeExecutor) stagedEnvironmentPath(releaseID string) string {
	return filepath.Join(e.config.StateDir, "releases", releaseID, "release.env")
}

func (e *ComposeExecutor) currentEnvironmentPath() string {
	return e.config.ReleaseEnvFile
}

func (e *ComposeExecutor) deploymentPath() string {
	return filepath.Join(e.config.StateDir, "deployment.json")
}

func (e *ComposeExecutor) writeInstalledRelease(release InstalledRelease) error {
	data, err := json.Marshal(release)
	if err != nil {
		return err
	}
	return atomicWrite(e.deploymentPath(), append(data, '\n'), 0o600)
}

func validateComposeExecutorConfig(config ComposeExecutorConfig) error {
	for name, value := range map[string]string{
		"state directory": config.StateDir, "work directory": config.WorkDir, "compose file": config.ComposeFile,
		"base env file": config.BaseEnvFile, "release env file": config.ReleaseEnvFile,
	} {
		if value == "" || !filepath.IsAbs(value) {
			return fmt.Errorf("%s must be an absolute path", name)
		}
	}
	if config.ProjectName == "" || config.MigrationService == "" || config.DatabaseService == "" || config.DatabaseName == "" || config.DatabaseUser == "" {
		return errors.New("compose project, migration service, and database settings are required")
	}
	if len(config.ArtifactEnvironment) == 0 || len(config.ServiceArtifacts) == 0 || len(config.ApplicationServices) == 0 || len(config.ValidationServices) == 0 || len(config.ResumeServices) == 0 {
		return errors.New("artifact and service mappings are required")
	}
	for artifact, variable := range config.ArtifactEnvironment {
		if !artifactNamePattern.MatchString(artifact) || !environmentNamePattern.MatchString(variable) {
			return errors.New("artifact environment mapping is invalid")
		}
	}
	for service, artifact := range config.ServiceArtifacts {
		if !artifactNamePattern.MatchString(service) {
			return fmt.Errorf("compose service %q is invalid", service)
		}
		if _, ok := config.ArtifactEnvironment[artifact]; !ok {
			return fmt.Errorf("compose service %q references unknown artifact %q", service, artifact)
		}
	}
	for _, service := range config.ApplicationServices {
		if _, ok := config.ServiceArtifacts[service]; !ok {
			return fmt.Errorf("application service %q has no artifact mapping", service)
		}
	}
	applicationSet := make(map[string]struct{}, len(config.ApplicationServices))
	for _, service := range config.ApplicationServices {
		applicationSet[service] = struct{}{}
	}
	phaseSet := make(map[string]struct{}, len(config.ValidationServices)+len(config.ResumeServices))
	for phase, services := range map[string][]string{"validation": config.ValidationServices, "resume": config.ResumeServices} {
		for _, service := range services {
			if _, ok := applicationSet[service]; !ok {
				return fmt.Errorf("%s service %q is not an application service", phase, service)
			}
			if _, duplicate := phaseSet[service]; duplicate {
				return fmt.Errorf("service %q is assigned to multiple switch phases", service)
			}
			phaseSet[service] = struct{}{}
		}
	}
	if len(phaseSet) != len(applicationSet) {
		return errors.New("every application service must belong to validation or resume phase")
	}
	migrationArtifact, ok := config.ServiceArtifacts[config.MigrationService]
	if !ok {
		return errors.New("migration service has no artifact mapping")
	}
	if migrationArtifact != "backend" {
		return errors.New("migration service must use the backend artifact")
	}
	for name, value := range map[string]string{"validation health URL": config.ValidationHealthURL, "health URL": config.HealthURL} {
		healthURL, err := url.Parse(value)
		if err != nil || healthURL.Host == "" || (healthURL.Scheme != "http" && healthURL.Scheme != "https") {
			return fmt.Errorf("%s must be absolute HTTP or HTTPS", name)
		}
	}
	return nil
}

func cloneStrings(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
