# ov-dash deployment

This deployment scaffold runs:

- Go API from `backend/cmd/api`
- Go worker from `backend/cmd/worker`
- PostgreSQL 16 with persistent storage
- Redis 7 with AOF persistence and password auth
- shadcn-admin frontend served by nginx

## Files

- `docker/backend.Dockerfile`: shared multi-stage Go image for API and worker, with Python available for worker scripts.
- `docker/frontend.Dockerfile`: pnpm build for shadcn-admin plus nginx runtime.
- `nginx/default.conf`: SPA fallback, static cache headers, `/api/` reverse proxy, and nginx health check.
- `postgres/init/`: SQL or shell files executed only when the PostgreSQL volume is first created.
- `scripts/install-docker-ubuntu.sh`: Docker Engine and Compose plugin bootstrap for Ubuntu hosts.
- `systemd/ov-dash-compose.service`: optional systemd wrapper for `/opt/ov-dash`.

## First deploy

Use the test host address as the deployment target and enter credentials manually through your normal secure channel. Do not commit host credentials or production secrets.

```sh
cd /opt/ov-dash
cp .env.example .env
vi .env
docker login ghcr.io
docker compose --env-file .env pull
docker compose --env-file .env up -d --no-build
```

Open `http://192.168.2.17` after the stack is healthy.

For a clean Ubuntu host without Docker:

```sh
sudo sh deploy/scripts/install-docker-ubuntu.sh
```

## Operations

```sh
make env
make compose-config
make compose-pull
make compose-up
make compose-up-backend
make compose-logs
make compose-ps
make backup-db
```

`compose-up` starts the full stack from release images. `compose-up-backend` starts PostgreSQL, Redis, API, and worker only. Use `make compose-build` and `make compose-up-build` only for local builds with `docker-compose.build.yml`.

## Security defaults

- PostgreSQL and Redis bind to `127.0.0.1` on the host by default.
- API is exposed on `HTTP_PORT`, default `8080`.
- Frontend nginx is exposed on `FRONTEND_PORT`, default `80`.
- Change every password in `.env` before deployment.
- Keep database and Redis passwords URL-safe unless you also customize the DSN format consumed by the backend.

## Optional systemd

```sh
sudo cp deploy/systemd/ov-dash-compose.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now ov-dash-compose
```

The unit assumes the repository lives at `/opt/ov-dash`.
