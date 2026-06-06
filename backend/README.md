# ov-dash backend

Go API and worker foundation for ov-dash.

## Services

- `cmd/api`: HTTP API with health probes and job enqueue endpoint.
- `cmd/worker`: Redis-backed worker runner with pluggable handlers.
- `internal/platform`: shared backend runtime that wires core infrastructure for all modules.
- `internal/cache`: Redis-backed cache facade with namespaced keys.
- `internal/config`: environment-driven configuration.
- `internal/database`: database management helpers, including SQL migration execution.
- `internal/db`: PostgreSQL pool.
- `internal/events`: in-process event flow center for publishing and subscribing to module events.
- `internal/queue`: Redis queue client.
- `internal/modules/proxy`: SOCKS5 proxy settings and reusable proxied HTTP client factory.
- `internal/worker`: Go worker handlers, including Python script invocation.

## Foundation modules

Backend services are initialized through `platform.Open(ctx, cfg)`. New modules should receive the shared runtime, or the specific dependency they need from it:

- `Runtime.DB`: PostgreSQL pool for repositories and migrations.
- `Runtime.Migrations`: SQL migration runner for controlled database upgrades.
- `Runtime.Cache`: Redis cache facade for short-lived shared state.
- `Runtime.Queue`: Redis job queue for background work.
- `Runtime.Events`: event flow center for module lifecycle events.
- `Runtime.Logger`: structured Zap logger.
- `Runtime.Proxy`: SOCKS5 proxy settings service and HTTP client factory.

The event center currently runs in-process and logs every event. It is ready for later subscribers such as audit logs, notifications, WebSocket streams, or external queues.

## Local run

```powershell
go mod tidy
go run ./cmd/api
go run ./cmd/worker
```

Create a sample job:

```powershell
Invoke-RestMethod -Method Post http://localhost:8080/api/v1/jobs -ContentType application/json -Body '{"type":"noop","payload":{}}'
```
