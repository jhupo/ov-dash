package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type LauncherConfig struct {
	RuntimeDir string
	SeedDir    string
	Version    string
}

type Launch struct {
	AppPath       string
	FrontendDir   string
	MigrationsDir string
	Version       string
}

type launchManifest struct {
	Version string `json:"version"`
}

func PrepareRuntime(ctx context.Context, config LauncherConfig) (Launch, error) {
	config.RuntimeDir = filepath.Clean(strings.TrimSpace(config.RuntimeDir))
	config.SeedDir = filepath.Clean(strings.TrimSpace(config.SeedDir))
	config.Version = strings.TrimSpace(config.Version)
	if config.RuntimeDir == "." || config.RuntimeDir == "" {
		return Launch{}, errors.New("update runtime directory is required")
	}
	if config.SeedDir == "." || config.SeedDir == "" {
		return Launch{}, errors.New("release seed directory is required")
	}
	if err := os.MkdirAll(filepath.Join(config.RuntimeDir, "releases"), 0o755); err != nil {
		return Launch{}, fmt.Errorf("create releases directory: %w", err)
	}
	lock, err := acquireRuntimeLock(filepath.Join(config.RuntimeDir, "update.lock"))
	if err != nil {
		return Launch{}, err
	}
	defer lock.Close()

	if err := initializeFromSeed(config); err != nil {
		return Launch{}, err
	}
	if err := activatePending(ctx, config.RuntimeDir); err != nil {
		fmt.Fprintf(os.Stderr, "pending update failed, keeping current release: %v\n", err)
	}
	if err := failInterruptedOperation(config.RuntimeDir); err != nil {
		return Launch{}, err
	}
	currentDir := filepath.Join(config.RuntimeDir, "current")
	manifest, err := readLaunchManifest(filepath.Join(currentDir, "manifest.json"))
	if err != nil {
		return Launch{}, err
	}
	if err := runMigration(ctx, currentDir, manifest.Version); err != nil {
		return Launch{}, err
	}
	return Launch{
		AppPath:       filepath.Join(currentDir, "bin", "app"),
		FrontendDir:   filepath.Join(currentDir, "web"),
		MigrationsDir: filepath.Join(currentDir, "migrations"),
		Version:       manifest.Version,
	}, nil
}

func failInterruptedOperation(runtimeDir string) error {
	if _, err := os.Stat(filepath.Join(runtimeDir, "pending.json")); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	operation, err := readOperation(runtimeDir)
	if err != nil || operation == nil || operation.State == StateCommitted || operation.State == StateFailed {
		return err
	}
	operation.State = StateFailed
	operation.Revision++
	operation.UpdatedAt = time.Now().UTC()
	operation.LastError = "update was interrupted before the release was staged"
	return writeJSONAtomic(filepath.Join(runtimeDir, "operation.json"), operation, 0o600)
}

func initializeFromSeed(config LauncherConfig) error {
	currentPath := filepath.Join(config.RuntimeDir, "current")
	if _, err := os.Lstat(currentPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect current release: %w", err)
	}
	seedPath := filepath.Join(config.SeedDir, config.Version)
	if _, err := os.Stat(seedPath); err != nil {
		return fmt.Errorf("find release seed %q: %w", config.Version, err)
	}
	releasePath := filepath.Join(config.RuntimeDir, "releases", config.Version)
	if _, err := os.Stat(releasePath); errors.Is(err, os.ErrNotExist) {
		stagingPath, err := os.MkdirTemp(filepath.Join(config.RuntimeDir, "releases"), ".seed-"+config.Version+"-")
		if err != nil {
			return fmt.Errorf("create seed staging directory: %w", err)
		}
		if err := os.Remove(stagingPath); err != nil {
			return fmt.Errorf("prepare seed staging directory: %w", err)
		}
		defer os.RemoveAll(stagingPath)
		if err := copyDirectory(seedPath, stagingPath); err != nil {
			return fmt.Errorf("install release seed: %w", err)
		}
		if err := os.Rename(stagingPath, releasePath); err != nil {
			return fmt.Errorf("commit release seed: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect seeded release: %w", err)
	}
	if err := validateSeedRelease(releasePath); err != nil {
		return err
	}
	return replaceSymlink(currentPath, filepath.Join("releases", config.Version))
}

func validateSeedRelease(releasePath string) error {
	for _, name := range []string{
		"manifest.json",
		filepath.Join("bin", "app"),
		filepath.Join("bin", "migrate"),
		filepath.Join("bin", "bootstrap-admin"),
		filepath.Join("web", "index.html"),
	} {
		info, err := os.Stat(filepath.Join(releasePath, name))
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("seeded release is missing %s", filepath.ToSlash(name))
		}
	}
	return nil
}

func activatePending(ctx context.Context, runtimeDir string) error {
	pendingPath := filepath.Join(runtimeDir, "pending.json")
	data, err := os.ReadFile(pendingPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read pending update: %w", err)
	}
	var pending Pending
	if err := decodeStrictJSON(data, &pending); err != nil {
		return failPending(runtimeDir, pending, fmt.Errorf("decode pending update: %w", err))
	}
	releaseDir := filepath.Join(runtimeDir, "releases", pending.Version)
	manifest, err := readLaunchManifest(filepath.Join(releaseDir, "manifest.json"))
	if err != nil {
		return failPending(runtimeDir, pending, err)
	}
	if manifest.Version != pending.Version {
		return failPending(runtimeDir, pending, errors.New("pending release manifest version mismatch"))
	}
	if err := runMigration(ctx, releaseDir, pending.Version); err != nil {
		return failPending(runtimeDir, pending, err)
	}

	currentPath := filepath.Join(runtimeDir, "current")
	if target, err := os.Readlink(currentPath); err == nil {
		if err := replaceSymlink(filepath.Join(runtimeDir, "previous"), target); err != nil {
			return failPending(runtimeDir, pending, fmt.Errorf("record previous release: %w", err))
		}
	}
	if err := replaceSymlink(currentPath, filepath.Join("releases", pending.Version)); err != nil {
		return failPending(runtimeDir, pending, fmt.Errorf("activate pending release: %w", err))
	}
	pending.Operation.State = StateCommitted
	pending.Operation.Revision++
	pending.Operation.UpdatedAt = time.Now().UTC()
	pending.Operation.LastError = ""
	if err := writeJSONAtomic(filepath.Join(runtimeDir, "operation.json"), pending.Operation, 0o600); err != nil {
		return fmt.Errorf("commit update operation: %w", err)
	}
	if err := os.Remove(pendingPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove pending update: %w", err)
	}
	return nil
}

func failPending(runtimeDir string, pending Pending, cause error) error {
	if pending.Operation.ID != "" {
		pending.Operation.State = StateFailed
		pending.Operation.Revision++
		pending.Operation.UpdatedAt = time.Now().UTC()
		pending.Operation.LastError = cause.Error()
		_ = writeJSONAtomic(filepath.Join(runtimeDir, "operation.json"), pending.Operation, 0o600)
	}
	_ = os.Remove(filepath.Join(runtimeDir, "pending.json"))
	return cause
}

func runMigration(ctx context.Context, releaseDir, version string) error {
	command := exec.CommandContext(ctx, filepath.Join(releaseDir, "bin", "migrate"))
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = replaceEnvironment(os.Environ(), map[string]string{
		"APP_VERSION":    version,
		"MIGRATIONS_DIR": filepath.Join(releaseDir, "migrations"),
	})
	if err := command.Run(); err != nil {
		return fmt.Errorf("run migrations for %s: %w", version, err)
	}
	return nil
}

func replaceSymlink(path, target string) error {
	temp := path + ".next"
	_ = os.Remove(temp)
	if err := os.Symlink(target, temp); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return nil
}

func copyDirectory(source, destination string) error {
	if err := os.Mkdir(destination, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(source, entry.Name())
		destinationPath := filepath.Join(destination, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case info.IsDir():
			if err := copyDirectory(sourcePath, destinationPath); err != nil {
				return err
			}
		case info.Mode().IsRegular():
			if err := copyFile(sourcePath, destinationPath, info.Mode().Perm()); err != nil {
				return err
			}
		default:
			return fmt.Errorf("seed contains unsupported entry %s", sourcePath)
		}
	}
	return nil
}

func copyFile(source, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}

func readLaunchManifest(path string) (launchManifest, error) {
	var result launchManifest
	data, err := os.ReadFile(path)
	if err != nil {
		return result, fmt.Errorf("read release manifest: %w", err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("decode release manifest: %w", err)
	}
	if strings.TrimSpace(result.Version) == "" {
		return result, errors.New("release manifest version is required")
	}
	return result, nil
}

func replaceEnvironment(current []string, replacements map[string]string) []string {
	result := make([]string, 0, len(current)+len(replacements))
	for _, item := range current {
		key, _, _ := strings.Cut(item, "=")
		if _, replace := replacements[key]; !replace {
			result = append(result, item)
		}
	}
	for key, value := range replacements {
		result = append(result, key+"="+value)
	}
	return result
}
