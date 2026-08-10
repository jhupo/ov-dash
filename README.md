# ov-dash

ov-dash is a private operations dashboard built around a modular Go platform.

- `app`: Go API, River worker, outbox dispatcher, and frontend static files.
- `postgres`: business data, queue state, jobs, audit records, and durable events.
- `redis`: cache.
- `launcher`: versioned migrations and atomic application release activation.

## Layout

```text
backend/                 Go application, launcher, modules, and migrations
frontend/                React/Vite frontend
deploy/docker/           Unified runtime image
docker-compose.yml       Production/private deployment stack
docker-compose.build.yml Local image build override
```

## Quick start

```sh
cp .env.example .env
docker compose --env-file .env pull
docker compose --env-file .env up -d
```

The dashboard is exposed on `http://localhost:50003` by default. Health endpoints:

- `GET /healthz`
- `GET /readyz`
- `GET /api/v1/health`

Create the initial administrator with the `bootstrap-admin` profile described in [deploy/README.md](deploy/README.md).

## Development

```sh
make backend-app
make frontend-dev
```

The frontend development server calls `/api/v1`. Production packages include `frontend/dist`, and the Go application serves the SPA directly.

## Modules

Backend modules register their manifest, HTTP routes, jobs, event handlers, capabilities, settings, and health checks through `internal/platform/module`. Shared infrastructure lives under `internal/platform`, `internal/queue`, `internal/events`, and `internal/database`.

## Online updates

Tagged GitHub releases contain complete Linux application packages, separate SHA-256 checksums, and Ed25519 signatures. The app verifies and stages a package in the persistent `app_runtime` volume, requests graceful shutdown, and relies on Docker's `restart: unless-stopped`. The launcher runs the new migrations before atomically switching `current`.

See [deploy/README.md](deploy/README.md) for trust configuration, package layout, update lifecycle, and operational limits.
