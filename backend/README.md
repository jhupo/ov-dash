# ov-dash backend

Go API and worker foundation for ov-dash.

## Services

- `cmd/api`: HTTP API with health probes and job enqueue endpoint.
- `cmd/worker`: River/PostgreSQL worker runner with module-registered handlers.
- `cmd/migrate`: exclusive versioned migration, secret backfill, and key rotation entrypoint.
- `cmd/bootstrap-admin`: explicit first-administrator bootstrap.
- `cmd/updater`: host-side signed release controller.
- `internal/platform`: shared backend runtime that wires core infrastructure for all modules.
- `internal/cache`: Redis-backed cache facade with namespaced keys.
- `internal/config`: environment-driven configuration.
- `internal/database`: database management helpers, including SQL migration execution.
- `internal/db`: PostgreSQL pool.
- `internal/events`: in-process event bus and durable PostgreSQL outbox.
- `internal/queue`: River queue client plus durable job audit, logs, cancellation, and requeue.
- `internal/modules/proxy`: SOCKS5 proxy settings and reusable proxied HTTP client factory.
- `internal/worker`: module job dispatch, worker heartbeat, and outbox delivery.

## Foundation modules

API and Worker build the same immutable module catalog through `platform/app.Open(ctx, cfg)`. New services implement the platform `Module` contract and register HTTP routes, jobs, events, capabilities, settings, and health checks from one composition root.

- `Runtime.DB`: PostgreSQL pool for repositories and migrations.
- `Runtime.Cache`: Redis cache facade for short-lived shared state.
- `Runtime.Queue`: River/PostgreSQL job platform.
- `Runtime.Events`: synchronous in-process event delivery.
- `Runtime.Outbox`: durable transactional event delivery.
- `Runtime.Secrets`: encrypted keyring-backed secret store.
- `Runtime.Logger`: structured Zap logger.

Application migrations never run inside API or Worker. Run `cmd/migrate` before starting either process.

## Local run

```powershell
go mod tidy
go run ./cmd/api
go run ./cmd/worker
```
