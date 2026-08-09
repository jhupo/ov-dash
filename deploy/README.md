# ov-dash deployment

This deployment scaffold runs:

- Go API from `backend/cmd/api`
- Go worker from `backend/cmd/worker`
- one-shot database migration and legacy secret backfill from `backend/cmd/migrate`
- explicit first-administrator bootstrap from `backend/cmd/bootstrap-admin`
- PostgreSQL 16 with persistent storage
- Redis 7 with AOF persistence and password auth
- shadcn-admin frontend served by nginx

## Files

- `docker/backend.Dockerfile`: non-root runtime image containing API, worker, migrate, and bootstrap-admin binaries.
- `docker/frontend.Dockerfile`: pnpm build for shadcn-admin plus nginx runtime.
- `nginx/default.conf`: SPA fallback, static cache headers, `/api/` reverse proxy, and nginx health check.
- `postgres/init/`: PostgreSQL bootstrap SQL only. Application migrations never run from this directory.
- `scripts/install-docker-ubuntu.sh`: Docker Engine and Compose plugin bootstrap for Ubuntu hosts.
- `scripts/install-updater.sh`: verifies the installed release and installs the privileged updater.
- `systemd/ov-dash-compose.service`: optional systemd wrapper for `/opt/ov-dash`.
- `systemd/ov-dash-updater.service`: hardened host service that exclusively owns Docker update access.

## First deploy

Use the test host address as the deployment target and enter credentials manually through your normal secure channel. Do not commit host credentials or production secrets.

```sh
cd /opt/ov-dash
cp .env.example .env
vi .env
docker login ghcr.io
docker compose --env-file .env pull
docker compose --env-file .env run --rm migrate
```

Create the first administrator explicitly. The command refuses to create or promote an account when an administrator already exists.

```sh
read -r -p "Admin email: " BOOTSTRAP_ADMIN_EMAIL
read -r -p "Admin username: " BOOTSTRAP_ADMIN_USERNAME
read -r -s -p "Admin password: " BOOTSTRAP_ADMIN_PASSWORD
printf '\n'
export BOOTSTRAP_ADMIN_EMAIL BOOTSTRAP_ADMIN_USERNAME BOOTSTRAP_ADMIN_PASSWORD
docker compose --env-file .env --profile tools run --rm \
  -e BOOTSTRAP_ADMIN_EMAIL \
  -e BOOTSTRAP_ADMIN_USERNAME \
  -e BOOTSTRAP_ADMIN_PASSWORD \
  bootstrap-admin
unset BOOTSTRAP_ADMIN_EMAIL BOOTSTRAP_ADMIN_USERNAME BOOTSTRAP_ADMIN_PASSWORD
docker compose --env-file .env up -d --no-build
```

Open `http://192.168.2.17` after the stack is healthy.

For a private repository, the GHCR login token must be able to read the package images. Use a GitHub PAT classic with `read:packages` and repository read access, then log in with:

```sh
echo "$GITHUB_TOKEN" | docker login ghcr.io -u <github-user> --password-stdin
```

Production deployments must use immutable image digests from the signed release manifest. Version tags are published for discovery, but `latest` is not published.

```sh
APP_VERSION=0.1.5
BACKEND_IMAGE=ghcr.io/jhupo/ov-dash-backend@sha256:<backend-digest>
FRONTEND_IMAGE=ghcr.io/jhupo/ov-dash-frontend@sha256:<frontend-digest>
docker compose --env-file .env pull
docker compose --env-file .env stop frontend api worker
docker compose --env-file .env run --rm migrate
docker compose --env-file .env up -d --no-build
```

API and worker containers never execute migrations. The one-shot `migrate` service is the only migration and legacy secret backfill entry point.

## Signed releases

The release workflow accepts tags such as `v0.1.11`, but the trusted manifest uses strict SemVer without the prefix: `version=0.1.11` and `release_id=ov-dash-0.1.11`. Numeric prerelease identifiers with leading zeroes are rejected. It publishes `latest.json`, `release.json`, detached `release.json.sig`, and release archives. The manifest contains exactly the `backend` and `frontend` image keys, both pinned by digest. The one-shot `migrate` service always runs the signed `backend` digest; it has no independently configurable image.

Create the release key once on an offline administrative machine:

```sh
openssl genpkey -algorithm Ed25519 -out release-private.pem
openssl pkey -in release-private.pem -pubout -out release.pub
gh secret set RELEASE_ED25519_PRIVATE_KEY < release-private.pem
```

Keep `release-private.pem` offline and backed up. Install only `release.pub` on production hosts. By default, `minimum_version=0.0.0` and `database.from_schema=0` are stable compatibility baselines, so a host may skip releases. Raise them only through the repository variables `UPDATE_MINIMUM_VERSION` and `UPDATE_MINIMUM_SCHEMA` after intentionally dropping old upgrade paths. The verifier accepts an installed schema inside the inclusive `[from_schema,to_schema]` range. The workflow derives `database.to_schema` from the current migration set, uses a monotonic workflow run number, and expires manifests after 90 days.

The updater release source is a static HTTPS origin with this layout:

```text
https://updates.example.internal/ov-dash/
  latest.json
  ov-dash-0.1.11/
    release.json
    release.json.sig
```

GitHub Release assets are the source artifacts. Mirror the signed release archive to the private HTTPS origin without rewriting its files. Configure the base URL as `https://updates.example.internal/ov-dash`; the updater reads `latest.json`, then fetches `<release_id>/release.json` and `<release_id>/release.json.sig`. The pointer is untrusted discovery metadata; the selected manifest is always verified with Ed25519 before use.

## Install the updater

The Linux release archive contains `ov-dash-updater`, both systemd units, `host-artifacts.json`, and its detached signature. First deploy the selected release normally, run its migrations, and verify `/readyz`. Then initialize updater state from that same signed release. The installer requires OpenSSL, `jq`, Docker, `flock`, and systemd; the public key must be PKIX PEM.

```sh
sudo apt-get install -y jq
sudo sh deploy/scripts/install-updater.sh \
  --binary ./ov-dash-updater \
  --public-key ./release.pub \
  --host-manifest ./host-artifacts.json \
  --host-signature ./host-artifacts.json.sig \
  --release-url https://updates.example.internal/ov-dash \
  --installed-manifest ./release.json \
  --installed-signature ./release.json.sig
```

Before installing any root executable or unit, the installer uses the system OpenSSL binary and the externally supplied public key to verify both detached signatures. It then checks the SHA-256 of the updater binary and both systemd units against the signed host artifact manifest. The binary being installed never verifies itself. The installer script is outside that self-contained chain: obtain the whole release archive from a trusted GitHub Release or verify the archive through an external trusted channel before executing the script. The installer then creates:

- `/usr/local/libexec/ov-dash-updater`
- `/etc/ov-dash/release.pub` and `/etc/ov-dash/updater.env`
- `/var/lib/ov-dash/updater/deployment.json` and `current.env`
- `/etc/systemd/system/ov-dash-updater.service`
- `/etc/systemd/system/ov-dash-compose.service`
- `/run/ov-dash/updater.sock`, owned by `root:ov-dash` with mode `0660`

Re-running the installer updates the signed binary, key, URL, and units but never overwrites existing deployment state. It reads the socket path, database name, database user, and host health port from `/opt/ov-dash/.env`, writes them to `/etc/ov-dash/updater.env`, and passes those values into the updater service. It also resolves the actual `ov-dash` host group GID, atomically updates `UPDATE_SOCKET_GID` in `/opt/ov-dash/.env`, and restarts the lock-gated Compose unit.

The signed installed manifest's `database.to_schema` must match the database already on the host. Do not initialize state from a future release or from a manifest that does not describe the currently running digest images.

The updater runs on the host and is the only application component with Docker authority. The API receives `/run/ov-dash` as a read-only directory bind mount and connects to the group-owned socket inside it. Mounting the directory keeps the connection valid across updater restarts that replace the socket inode. Never mount `/run/docker.sock` into API or worker containers.

```sh
sudo systemctl status ov-dash-updater
sudo journalctl -u ov-dash-updater -f
curl --unix-socket /run/ov-dash/updater.sock http://localhost/healthz
```

## Update rollback

Every online update enters a maintenance window, stops application writers with a 45-second grace period, creates a PostgreSQL custom-format snapshot, runs the one-shot migration from the exact signed backend digest, and starts only the API for release-aware `/readyz?scope=api` validation. Worker and frontend remain stopped. Failures through this API-only validation restore the database snapshot and previous images.

Commit starts worker and frontend, waits for Compose health plus release-aware `/readyz`, and only then writes installed release state. Because starting worker may create database writes or external side effects, any failure after commit begins is marked `manual_intervention`; automatic snapshot restoration is forbidden, and the updater attempts to stop worker and frontend while leaving the maintenance window closed.

Keep free disk space for at least one full database snapshot under `/var/lib/ov-dash/updater`. The updater snapshot covers PostgreSQL only; back up `/opt/ov-dash/.env`, uploaded files, and other host data separately before an update. Do not manually restart application services during `migrating`, `switching`, `health_checking`, `committing`, or `rolling_back`. A `rollback_failed` or `manual_intervention` operation is a durable global fault lock: new updates and automatic Compose startup remain blocked until an operator repairs the journaled state.

If pulling public base images is slow in China, configure Docker daemon registry mirrors or host-level proxy. Keep Compose image names canonical unless a specific mirror is known to preserve the exact upstream namespace and tags.

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
- API binds to `HTTP_BIND`, default `127.0.0.1`, on `HTTP_PORT` for updater health checks only. Frontend nginx proxies `/api/v1` and WebSocket traffic over the private Compose network.
- Frontend nginx is exposed on `FRONTEND_PORT`, default `80`.
- API and worker run as the image's unprivileged `app` user with all Linux capabilities dropped.
- API has no host repository, `.netrc`, migrations, or Docker socket mount.
- Change every password in `.env` before deployment.
- Configure `APP_SECRET_ACTIVE_KEY_ID` and `APP_SECRET_KEYS_JSON` with stable private values before the first migration. Keep old keys in the keyring until `migrate` has rotated every stored secret to the active key.
- Keep database and Redis passwords URL-safe unless you also customize the DSN format consumed by the backend.

## Systemd ownership

The installer manages both systemd units. The updater uses `Type=notify` and does not announce readiness until interrupted-operation recovery has acquired the exclusive update lock. The Compose unit starts afterward under a shared `flock`, runs the updater's read-only `--assert-start-safe` check, and only then executes `docker compose up`. The updater process does not hold the lock while idle. Once initialized, the Compose unit adds `/var/lib/ov-dash/updater/current.env` as the second environment file so immutable release images and `APP_VERSION` survive host restarts.
