package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	App        AppConfig
	HTTP       HTTPConfig
	Postgres   PostgresConfig
	Redis      RedisConfig
	Worker     WorkerConfig
	Python     PythonConfig
	Update     UpdateConfig
	Migrations MigrationsConfig
	Uploads    UploadsConfig
	Security   SecurityConfig
}

type AppConfig struct {
	Name            string
	Env             string
	Version         string
	ShutdownTimeout time.Duration
}

type HTTPConfig struct {
	Host           string
	Port           int
	AllowedOrigins []string
}

func (c HTTPConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

type PostgresConfig struct {
	DSN             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
	Prefix   string
}

type WorkerConfig struct {
	QueueName       string
	PollInterval    time.Duration
	JobTimeout      time.Duration
	Concurrency     int
	VisibilityLease time.Duration
}

type PythonConfig struct {
	Bin        string
	ScriptsDir string
}

type UpdateConfig struct {
	WorkDir                 string
	Remote                  string
	Image                   string
	BackendImageRepository  string
	FrontendImageRepository string
	FallbackBuild           bool
	Project                 string
	Enabled                 bool
}

type MigrationsConfig struct {
	Dir string
}

type UploadsConfig struct {
	WikiDir string
}

type SecurityConfig struct {
	SecretKey string
}

const DefaultSecretKey = "local-development-secret-change-me"

func (c SecurityConfig) UsesDefaultSecretKey() bool {
	return strings.TrimSpace(c.SecretKey) == DefaultSecretKey
}

func Load() Config {
	return Config{
		App: AppConfig{
			Name:            env("APP_NAME", "ov-dash"),
			Env:             env("APP_ENV", "local"),
			Version:         env("APP_VERSION", "local"),
			ShutdownTimeout: durationEnv("APP_SHUTDOWN_TIMEOUT", 15*time.Second),
		},
		HTTP: HTTPConfig{
			Host:           env("HTTP_HOST", "0.0.0.0"),
			Port:           intEnv("HTTP_PORT", 8080),
			AllowedOrigins: listEnv("HTTP_ALLOWED_ORIGINS", []string{"http://localhost:5173"}),
		},
		Postgres: PostgresConfig{
			DSN:             env("POSTGRES_DSN", "postgres://ov_dash:ov_dash@localhost:5432/ov_dash?sslmode=disable"),
			MaxConns:        int32(intEnv("POSTGRES_MAX_CONNS", 20)),
			MinConns:        int32(intEnv("POSTGRES_MIN_CONNS", 2)),
			MaxConnLifetime: durationEnv("POSTGRES_MAX_CONN_LIFETIME", time.Hour),
			MaxConnIdleTime: durationEnv("POSTGRES_MAX_CONN_IDLE_TIME", 30*time.Minute),
		},
		Redis: RedisConfig{
			Addr:     env("REDIS_ADDR", "localhost:6379"),
			Password: env("REDIS_PASSWORD", ""),
			DB:       intEnv("REDIS_DB", 0),
			Prefix:   env("REDIS_PREFIX", "ov-dash"),
		},
		Worker: WorkerConfig{
			QueueName:       env("WORKER_QUEUE_NAME", "jobs:default"),
			PollInterval:    durationEnv("WORKER_POLL_INTERVAL", time.Second),
			JobTimeout:      durationEnv("WORKER_JOB_TIMEOUT", 5*time.Minute),
			Concurrency:     intEnv("WORKER_CONCURRENCY", 4),
			VisibilityLease: durationEnv("WORKER_VISIBILITY_LEASE", 10*time.Minute),
		},
		Python: PythonConfig{
			Bin:        env("PYTHON_BIN", "python3"),
			ScriptsDir: env("PYTHON_SCRIPTS_DIR", "./scripts"),
		},
		Update: UpdateConfig{
			WorkDir:                 env("UPDATE_WORKDIR", "/opt/ov-dash"),
			Remote:                  env("UPDATE_REMOTE", "origin"),
			Image:                   env("UPDATE_IMAGE", "ov-dash-backend:local"),
			BackendImageRepository:  env("UPDATE_BACKEND_IMAGE_REPOSITORY", "ghcr.io/jhupo/ov-dash-backend"),
			FrontendImageRepository: env("UPDATE_FRONTEND_IMAGE_REPOSITORY", "ghcr.io/jhupo/ov-dash-frontend"),
			FallbackBuild:           boolEnv("UPDATE_FALLBACK_BUILD", false),
			Project:                 env("UPDATE_PROJECT", "ov-dash"),
			Enabled:                 boolEnv("UPDATE_ENABLED", true),
		},
		Migrations: MigrationsConfig{
			Dir: env("MIGRATIONS_DIR", "/migrations"),
		},
		Uploads: UploadsConfig{
			WikiDir: env("WIKI_UPLOADS_DIR", "./uploads/wiki"),
		},
		Security: SecurityConfig{
			SecretKey: env("APP_SECRET_KEY", DefaultSecretKey),
		},
	}
}

func boolEnv(key string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func intEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return value
}

func listEnv(key string, fallback []string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	items := strings.Split(raw, ",")
	values := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			values = append(values, item)
		}
	}
	if len(values) == 0 {
		return fallback
	}
	return values
}
