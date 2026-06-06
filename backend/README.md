# ov-dash backend

Go API and worker foundation for ov-dash.

## Services

- `cmd/api`: HTTP API with health probes and job enqueue endpoint.
- `cmd/worker`: Redis-backed worker runner with pluggable handlers.
- `internal/config`: environment-driven configuration.
- `internal/db`: PostgreSQL pool.
- `internal/queue`: Redis queue client.
- `internal/worker`: Go worker handlers, including Python script invocation.

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
