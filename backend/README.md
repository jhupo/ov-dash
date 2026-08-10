# ov-dash backend

The backend is one application process with a stable release launcher.

## Entrypoints

- `cmd/app`: HTTP API, River worker, outbox dispatcher, and frontend static serving.
- `cmd/launcher`: initializes `app_runtime`, runs migrations, activates a pending release, and executes the current app.
- `cmd/migrate`: advisory-locked versioned migrations, secret backfill, and key rotation.
- `cmd/bootstrap-admin`: explicit first-administrator bootstrap.

## Platform

Modules implement the platform `Module` contract and register HTTP routes, jobs, events, capabilities, settings, and health checks through one catalog.

- `Runtime.DB`: PostgreSQL repositories and migration state.
- `Runtime.Cache`: Redis cache.
- `Runtime.Queue`: River/PostgreSQL job platform.
- `Runtime.Events`: synchronous process-local events.
- `Runtime.Outbox`: durable transactional events.
- `Runtime.Secrets`: encrypted keyring-backed secret storage.
- `Runtime.Lifecycle`: coordinated application shutdown.

`internal/updater` handles signed package discovery, download, verification, staging, and launcher activation. It does not control Docker or use a host Unix socket.

## Local run

Start PostgreSQL and Redis, then:

```powershell
go run ./cmd/migrate
go run ./cmd/app
```

Set `FRONTEND_DIR` to a built frontend directory when the Go app should serve the UI locally.
