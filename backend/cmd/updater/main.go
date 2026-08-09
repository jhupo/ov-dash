package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"ov-dash/backend/internal/updater"
)

type options struct {
	stateDir        string
	socketPath      string
	releaseURL      string
	publicKeyPath   string
	allowHTTP       bool
	workDir         string
	composeFile     string
	baseEnvFile     string
	releaseEnvFile  string
	projectName     string
	dockerBinary    string
	artifacts       string
	services        string
	appServices     string
	validationSvcs  string
	resumeSvcs      string
	migrateService  string
	database        string
	databaseUser    string
	databaseSvc     string
	healthURL       string
	validationURL   string
	assertStartSafe bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		log.Printf("updater stopped: %v", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	options, err := parseFlags(args)
	if err != nil {
		return err
	}

	if options.assertStartSafe {
		return assertStartSafe(options.stateDir)
	}

	publicKey, err := updater.LoadPublicKey(options.publicKeyPath)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	artifactEnvironment, err := parseMapping(options.artifacts)
	if err != nil {
		return fmt.Errorf("parse artifacts: %w", err)
	}
	serviceArtifacts, err := parseMapping(options.services)
	if err != nil {
		return fmt.Errorf("parse services: %w", err)
	}
	applicationServices, err := parseList(options.appServices)
	if err != nil {
		return fmt.Errorf("parse application services: %w", err)
	}
	validationServices, err := parseList(options.validationSvcs)
	if err != nil {
		return fmt.Errorf("parse validation services: %w", err)
	}
	resumeServices, err := parseList(options.resumeSvcs)
	if err != nil {
		return fmt.Errorf("parse resume services: %w", err)
	}
	allowedImages := make([]string, 0, len(artifactEnvironment))
	for artifact := range artifactEnvironment {
		allowedImages = append(allowedImages, artifact)
	}
	sort.Strings(allowedImages)

	store, err := updater.NewStore(options.stateDir)
	if err != nil {
		return err
	}
	verifier, err := updater.NewVerifier(publicKey, updater.VerifyOptions{AllowedImages: allowedImages})
	if err != nil {
		return err
	}
	releaseSource, err := updater.NewHTTPReleaseSource(options.releaseURL, nil, options.allowHTTP)
	if err != nil {
		return err
	}
	executor, err := updater.NewComposeExecutor(updater.ComposeExecutorConfig{
		StateDir:            options.stateDir,
		WorkDir:             options.workDir,
		ComposeFile:         options.composeFile,
		BaseEnvFile:         options.baseEnvFile,
		ProjectName:         options.projectName,
		DockerBinary:        options.dockerBinary,
		ReleaseEnvFile:      options.releaseEnvFile,
		ArtifactEnvironment: artifactEnvironment,
		ServiceArtifacts:    serviceArtifacts,
		ApplicationServices: applicationServices,
		ValidationServices:  validationServices,
		ResumeServices:      resumeServices,
		MigrationService:    options.migrateService,
		DatabaseService:     options.databaseSvc,
		DatabaseName:        options.database,
		DatabaseUser:        options.databaseUser,
		ValidationHealthURL: options.validationURL,
		HealthURL:           options.healthURL,
		CommandTimeout:      20 * time.Minute,
	}, releaseSource, updater.OSCommandRunner{}, &http.Client{Timeout: 10 * time.Second})
	if err != nil {
		return err
	}
	controller, err := updater.NewController(ctx, store, verifier, executor)
	if err != nil {
		return err
	}
	service := updater.NewService(controller)
	if operation, err := service.Recover(); err != nil {
		return fmt.Errorf("recover interrupted operation: %w", err)
	} else if operation != nil {
		log.Printf("recovering update operation id=%s state=%s", operation.ID, operation.State)
	}
	handler, err := updater.NewHTTPHandler(service)
	if err != nil {
		return err
	}
	server := updater.UnixServer{SocketPath: options.socketPath, SocketMode: 0o660, Handler: handler, Ready: notifySystemdReady}
	log.Printf("updater listening on unix://%s", options.socketPath)
	return server.ListenAndServe(ctx)
}

func notifySystemdReady() error {
	address := os.Getenv("NOTIFY_SOCKET")
	if address == "" {
		return nil
	}
	if strings.HasPrefix(address, "@") {
		address = "\x00" + strings.TrimPrefix(address, "@")
	}
	connection, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: address, Net: "unixgram"})
	if err != nil {
		return err
	}
	defer connection.Close()
	_, err = connection.Write([]byte("READY=1"))
	return err
}

func parseFlags(args []string) (options, error) {
	var result options
	flags := flag.NewFlagSet("updater", flag.ContinueOnError)
	flags.StringVar(&result.stateDir, "state-dir", "/var/lib/ov-dash/updater", "persistent updater state directory")
	flags.StringVar(&result.socketPath, "socket", "/run/ov-dash/updater.sock", "HTTP Unix socket path")
	flags.StringVar(&result.releaseURL, "release-url", "", "trusted HTTPS release repository")
	flags.StringVar(&result.publicKeyPath, "release-public-key", "/etc/ov-dash/release.pub", "Ed25519 release public key")
	flags.BoolVar(&result.allowHTTP, "allow-http-release-source", false, "allow HTTP release source for isolated development only")
	flags.StringVar(&result.workDir, "work-dir", "/opt/ov-dash", "Compose working directory")
	flags.StringVar(&result.composeFile, "compose-file", "/opt/ov-dash/docker-compose.yml", "controlled Compose file")
	flags.StringVar(&result.baseEnvFile, "base-env-file", "/opt/ov-dash/.env", "base Compose environment file")
	flags.StringVar(&result.releaseEnvFile, "release-env-file", "/var/lib/ov-dash/updater/current.env", "active immutable image environment file")
	flags.StringVar(&result.projectName, "project", "ov-dash", "fixed Compose project name")
	flags.StringVar(&result.dockerBinary, "docker-binary", "docker", "Docker CLI executable")
	flags.StringVar(&result.artifacts, "artifacts", "backend=BACKEND_IMAGE,frontend=FRONTEND_IMAGE", "signed artifact to environment mapping")
	flags.StringVar(&result.services, "services", "api=backend,worker=backend,frontend=frontend,migrate=backend", "Compose service to signed artifact mapping")
	flags.StringVar(&result.appServices, "application-services", "api,worker,frontend", "services stopped and switched in the maintenance window")
	flags.StringVar(&result.validationSvcs, "validation-services", "api", "read-only services started before release validation")
	flags.StringVar(&result.resumeSvcs, "resume-services", "worker,frontend", "write-capable services started during commit")
	flags.StringVar(&result.migrateService, "migration-service", "migrate", "one-shot migration service")
	flags.StringVar(&result.databaseSvc, "database-service", "postgres", "Compose database service")
	flags.StringVar(&result.database, "database-name", "ov_dash", "PostgreSQL database name")
	flags.StringVar(&result.databaseUser, "database-user", "ov_dash", "PostgreSQL backup user")
	flags.StringVar(&result.validationURL, "validation-health-url", "http://127.0.0.1:8080/readyz?scope=api", "release-aware API-only validation URL")
	flags.StringVar(&result.healthURL, "health-url", "http://127.0.0.1:8080/readyz", "release-aware complete-stack readiness URL")
	flags.BoolVar(&result.assertStartSafe, "assert-start-safe", false, "fail when updater state forbids starting the application stack")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if flags.NArg() != 0 {
		return options{}, errors.New("positional arguments are not accepted")
	}
	result.stateDir = absolute(result.stateDir)
	result.socketPath = absolute(result.socketPath)
	result.workDir = absolute(result.workDir)
	result.composeFile = absolute(result.composeFile)
	result.baseEnvFile = absolute(result.baseEnvFile)
	result.releaseEnvFile = absolute(result.releaseEnvFile)
	result.publicKeyPath = absolute(result.publicKeyPath)
	return result, nil
}

func assertStartSafe(stateDir string) error {
	store, err := updater.OpenStore(stateDir)
	if err != nil {
		return err
	}
	blocking, err := store.Active()
	if err != nil {
		return err
	}
	if len(blocking) == 0 {
		return nil
	}
	for _, operation := range blocking {
		if operation.State.RequiresOperatorIntervention() {
			return fmt.Errorf("%w: operation %s is %s", updater.ErrOperatorInterventionRequired, operation.ID, operation.State)
		}
	}
	operation := blocking[0]
	return fmt.Errorf("%w: operation %s is %s", updater.ErrOperationActive, operation.ID, operation.State)
}

func parseMapping(value string) (map[string]string, error) {
	result := make(map[string]string)
	for _, item := range strings.Split(value, ",") {
		parts := strings.Split(item, "=")
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, errors.New("mapping entries must use key=value")
		}
		key := strings.TrimSpace(parts[0])
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("duplicate mapping key %q", key)
		}
		result[key] = strings.TrimSpace(parts[1])
	}
	return result, nil
}

func parseList(value string) ([]string, error) {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, errors.New("list contains an empty value")
		}
		if _, exists := seen[item]; exists {
			return nil, fmt.Errorf("duplicate list value %q", item)
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result, nil
}

func absolute(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return absolutePath
}
