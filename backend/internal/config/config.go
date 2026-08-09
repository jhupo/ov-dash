package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"sort"
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
	Host                string
	Port                int
	AllowedOrigins      []string
	TrustedProxyCIDRs   []string
	SessionCookieSecure bool
}

func (c HTTPConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func (c HTTPConfig) Validate() error {
	for _, value := range c.TrustedProxyCIDRs {
		if _, err := netip.ParsePrefix(value); err != nil {
			return fmt.Errorf("invalid HTTP_TRUSTED_PROXY_CIDRS entry %q: %w", value, err)
		}
	}
	return nil
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
	QueueName   string
	JobTimeout  time.Duration
	Concurrency int
	RescueAfter time.Duration
}

type UpdateConfig struct {
	SocketPath string
}

type MigrationsConfig struct {
	Dir string
}

type UploadsConfig struct {
	WikiDir string
}

type SecurityConfig struct {
	ActiveSecretKeyID string
	SecretKeys        map[string]string
	validationError   error
}

const DefaultSecretKey = "local-development-secret-change-me"

func (c SecurityConfig) DefaultSecretKeyIDs() []string {
	ids := make([]string, 0)
	for id, key := range c.SecretKeys {
		if strings.TrimSpace(key) == DefaultSecretKey {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func (c SecurityConfig) Validate() error {
	if c.validationError != nil {
		return c.validationError
	}
	if strings.TrimSpace(c.ActiveSecretKeyID) == "" {
		return errors.New("APP_SECRET_ACTIVE_KEY_ID is required")
	}
	if len(c.SecretKeys) == 0 {
		return errors.New("APP_SECRET_KEYS_JSON must contain at least one key")
	}
	if strings.TrimSpace(c.SecretKeys[c.ActiveSecretKeyID]) == "" {
		return fmt.Errorf("active secret key %q is missing from APP_SECRET_KEYS_JSON", c.ActiveSecretKeyID)
	}
	return nil
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
			Host:                env("HTTP_HOST", "0.0.0.0"),
			Port:                intEnv("HTTP_PORT", 8080),
			AllowedOrigins:      listEnv("HTTP_ALLOWED_ORIGINS", []string{"http://localhost:5173"}),
			TrustedProxyCIDRs:   listEnv("HTTP_TRUSTED_PROXY_CIDRS", nil),
			SessionCookieSecure: boolEnv("COOKIE_SECURE", false),
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
			QueueName:   env("WORKER_QUEUE_NAME", "jobs-default"),
			JobTimeout:  durationEnv("WORKER_JOB_TIMEOUT", 5*time.Minute),
			Concurrency: intEnv("WORKER_CONCURRENCY", 4),
			RescueAfter: durationEnv("WORKER_RESCUE_AFTER", 15*time.Minute),
		},
		Update: UpdateConfig{
			SocketPath: env("UPDATE_SOCKET_PATH", "/run/ov-dash/updater.sock"),
		},
		Migrations: MigrationsConfig{
			Dir: env("MIGRATIONS_DIR", "/migrations"),
		},
		Uploads: UploadsConfig{
			WikiDir: env("WIKI_UPLOADS_DIR", "./uploads/wiki"),
		},
		Security: loadSecurityConfig(),
	}
}

func loadSecurityConfig() SecurityConfig {
	config := SecurityConfig{
		ActiveSecretKeyID: env("APP_SECRET_ACTIVE_KEY_ID", "local"),
		SecretKeys:        map[string]string{},
	}
	raw := env("APP_SECRET_KEYS_JSON", `{"local":"`+DefaultSecretKey+`"}`)
	if err := json.Unmarshal([]byte(raw), &config.SecretKeys); err != nil {
		config.validationError = fmt.Errorf("decode APP_SECRET_KEYS_JSON: %w", err)
		return config
	}
	rawIDs := make([]string, 0, len(config.SecretKeys))
	for id := range config.SecretKeys {
		rawIDs = append(rawIDs, id)
	}
	sort.Strings(rawIDs)

	normalized := make(map[string]string, len(config.SecretKeys))
	for _, rawID := range rawIDs {
		id := strings.TrimSpace(rawID)
		key := strings.TrimSpace(config.SecretKeys[rawID])
		if id == "" || key == "" {
			config.validationError = errors.New("APP_SECRET_KEYS_JSON contains an empty key id or value")
			return config
		}
		if _, exists := normalized[id]; exists {
			config.validationError = fmt.Errorf("APP_SECRET_KEYS_JSON contains duplicate key id %q after trimming", id)
			return config
		}
		normalized[id] = key
	}
	config.SecretKeys = normalized
	return config
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

func boolEnv(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
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
