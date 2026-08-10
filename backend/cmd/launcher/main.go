package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"ov-dash/backend/internal/config"
	"ov-dash/backend/internal/updater"
)

var version = "local"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	launch, err := updater.PrepareRuntime(ctx, updater.LauncherConfig{
		RuntimeDir: cfg.Update.RuntimeDir,
		SeedDir:    "/opt/ov-dash/seed",
		Version:    version,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare application runtime: %v\n", err)
		os.Exit(1)
	}
	environment := replaceEnvironment(os.Environ(), map[string]string{
		"APP_VERSION":    launch.Version,
		"FRONTEND_DIR":   launch.FrontendDir,
		"MIGRATIONS_DIR": launch.MigrationsDir,
	})
	if err := syscall.Exec(launch.AppPath, []string{launch.AppPath}, environment); err != nil {
		fmt.Fprintf(os.Stderr, "start application: %v\n", err)
		os.Exit(1)
	}
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
