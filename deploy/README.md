# Deployment

ov-dash runs as three containers:

- `app`: Go HTTP server, River worker, frontend static files, and online update client.
- `postgres`: application data and River queue.
- `redis`: cache.

The application container uses the named `app_runtime` volume:

```text
/opt/ov-dash/runtime/
|-- releases/<version>/
|-- current -> releases/<version>
|-- previous -> releases/<version>
|-- pending.json
|-- operation.json
`-- update.lock
```

PostgreSQL, Redis, and their volumes are not replaced during an application update.

## First deployment

```sh
cp .env.example .env
docker compose --env-file .env pull
docker compose --env-file .env up -d
```

The runtime image seeds its bundled release into `app_runtime`. The stable launcher runs versioned migrations and then executes `current/bin/app`. The application serves both the API and frontend on host port `APP_PORT`, default `50003`.

Create the first administrator after the app becomes healthy:

```sh
docker compose --env-file .env --profile tools run --rm \
  -e BOOTSTRAP_ADMIN_EMAIL=admin@example.com \
  -e BOOTSTRAP_ADMIN_PASSWORD='replace-me' \
  -e BOOTSTRAP_ADMIN_USERNAME=admin \
  bootstrap-admin
```

## Release trust

GitHub Actions publishes:

```text
release.json
release.json.sig
ov-dash_<version>_linux_amd64.tar.gz
ov-dash_<version>_linux_amd64.tar.gz.sha256
ov-dash_<version>_linux_amd64.tar.gz.sha256.sig
ov-dash_<version>_linux_arm64.tar.gz
ov-dash_<version>_linux_arm64.tar.gz.sha256
ov-dash_<version>_linux_arm64.tar.gz.sha256.sig
```

Configure `UPDATE_PUBLIC_KEY` with the Ed25519 public key matching the repository secret `RELEASE_ED25519_PRIVATE_KEY`. For an `.env` file, a single-line base64 PKIX DER value is the least error-prone form:

```sh
openssl pkey -pubin -in release.pub -outform DER | base64 -w0
```

The application verifies the signed release manifest, signed checksum, archive digest, package version, package architecture, and required files before staging an update.

`UPDATE_PROXY_URL` is an optional HTTPS download accelerator prefix. Administrators can override it at runtime through the registered `update.proxy_url` platform setting.

## Update lifecycle

1. The application downloads and verifies the signed release metadata.
2. It downloads the architecture-specific package and separate checksum/signature.
3. It extracts into a staging directory and atomically installs `releases/<version>`.
4. It writes `pending.json`, responds to the API request, and requests graceful shutdown.
5. Docker restarts the container because `restart: unless-stopped` is enabled.
6. The launcher runs the pending release's migrations.
7. On success it atomically changes `previous` and `current`, records the operation as committed, and starts the new application.
8. On migration failure it records the operation as failed, removes `pending.json`, and does not switch `current`. A failed migration intentionally blocks application startup until the migration fault is repaired.

Application directory switching is atomic. Database migrations must remain forward-compatible because this private deployment flow does not restore a database snapshot or automatically reverse migrations.

## Operations

```sh
make compose-config
make compose-pull
make compose-up
make compose-ps
make app-logs
make backup-db
```

PostgreSQL and Redis bind to localhost by default. The app binds to `0.0.0.0:50003` by default; place it behind HTTPS and set `COOKIE_SECURE=true` for internet-facing deployments.
