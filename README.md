# ov-dash

ov-dash is an operations dashboard foundation with:

- frontend: `satnaing/shadcn-admin` under `frontend/`
- backend: Go API under `backend/cmd/api`
- worker: Go River worker under `backend/cmd/worker`
- data and queue: PostgreSQL with River
- cache: Redis
- deployment: Docker Compose under `docker-compose.yml`

## Layout

```text
backend/              Go API, worker, config, db, queue and handlers
frontend/             shadcn-admin React/Vite application
deploy/docker/        Dockerfiles for backend and frontend
deploy/nginx/         nginx reverse proxy config
docker-compose.yml    local/test deployment stack
```

## Quick Start

```bash
cp .env.example .env
docker login ghcr.io
docker compose --env-file .env pull
docker compose --env-file .env up -d --no-build
```

Local development:

```bash
make compose-up-build
make backend-api
make backend-worker
make frontend-dev
```

Health endpoints:

- `GET /healthz`
- `GET /readyz`
- `GET /api/v1/health`

Queue a sample job:

```bash
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{"type":"noop","payload":{}}'
```

Telegram notifications:

1. Sign in to the dashboard and open `Settings -> Telegram 通知`.
2. Configure the Telegram Bot Token, an inbound token, and each user's Telegram Chat ID.
3. Send an inbound message to a user:

```bash
curl -X POST http://localhost:8080/api/v1/incoming-messages \
  -H "Content-Type: application/json" \
  -H "X-OV-Dash-Token: <inbound-token>" \
  -d '{"username":"classicriver","title":"Alert","message":"Server CPU is high","source":"monitor"}'
```

The endpoint also accepts `Authorization: Bearer <inbound-token>` and can target users by `user_id` instead of `username`.
If the request omits both `username` and `user_id`, the message is delivered to the configured Telegram group.
Set `"deliver_to_group": true` to send a user-targeted message to both the user and the group.

## Test deployment

Target host: `192.168.2.17`

Use the release image flow from `deploy/README.md`. Build on the server only for local test environments.

Use host credentials through your normal secure channel and keep them outside the repository.

## Deployment

Docker deployment scaffolding lives under `deploy/` and is wired by the root `docker-compose.yml`, `.env.example`, and `Makefile`.

Start the full stack:

```sh
make env
make compose-pull
make compose-up
```

Start only PostgreSQL, Redis, API, and worker:

```sh
make compose-up-backend
```

Common operations:

```sh
make env
make compose-config
make compose-build
make compose-ps
make compose-logs
make backup-db
```

See `deploy/README.md` for host setup, nginx behavior, systemd usage, and deployment notes.

## Backend

The backend is a Go service skeleton under `backend/` with separate API and worker entrypoints.

### Layout

- `backend/cmd/api`: HTTP API process.
- `backend/cmd/worker`: River/PostgreSQL job worker process.
- `backend/internal/platform`: Shared backend runtime for database, cache, queue, events, logs, and proxy.
- `backend/internal/cache`: Redis-backed cache facade.
- `backend/internal/config`: Environment-driven application configuration.
- `backend/internal/database`: Database management helpers, including SQL migrations.
- `backend/internal/db`: PostgreSQL connection pool setup via `pgx`.
- `backend/internal/events`: In-process event bus and durable PostgreSQL outbox.
- `backend/internal/http`: Router, middleware, health checks, and job enqueue endpoint.
- `backend/internal/modules/proxy`: SOCKS5 proxy settings and reusable proxied HTTP client factory.
- `backend/internal/queue`: River/PostgreSQL queue, job audit, logs, cancellation, and requeue.
- `backend/internal/worker`: River worker runner and durable event dispatcher.
- `backend/pkg/logging`: Shared Zap logger factory.

### Local run

```bash
cd backend
go mod tidy
go run ./cmd/api
go run ./cmd/worker
```

Default dependencies are expected at:

- PostgreSQL: `postgres://ov_dash:ov_dash@localhost:5432/ov_dash?sslmode=disable`
- Redis: `localhost:6379`

Common environment variables:

- `HTTP_HOST`: API listen host, default `0.0.0.0`.
- `HTTP_PORT`: API listen port, default `8080`.
- `HTTP_ALLOWED_ORIGINS`: comma-separated browser origins.
- `HTTP_TRUSTED_PROXY_CIDRS`: comma-separated proxy networks allowed to provide client IP headers.
- `COOKIE_SECURE`: require HTTPS-only session cookies.
- `POSTGRES_DSN`: PostgreSQL DSN.
- `REDIS_ADDR`: Redis host and port.
- `REDIS_PASSWORD`: Redis password.
- `REDIS_PREFIX`: Redis key prefix, default `ov-dash`.
- `WORKER_QUEUE_NAME`: River queue name, default `jobs:default`.
- `WORKER_CONCURRENCY`: Worker goroutine count, default `4`.
- `APP_SECRET_ACTIVE_KEY_ID`: active secret encryption key ID.
- `APP_SECRET_KEYS_JSON`: JSON keyring used to decrypt stored secrets and encrypt with the active key.

### Health and queue endpoints

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
```
