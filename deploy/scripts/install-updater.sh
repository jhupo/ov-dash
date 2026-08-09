#!/bin/sh
set -eu

usage() {
  printf '%s\n' \
    'Usage: install-updater.sh \' \
    '  --binary PATH \' \
    '  --public-key PATH \' \
    '  --host-manifest PATH \' \
    '  --host-signature PATH \' \
    '  --release-url HTTPS_URL \' \
    '  [--installed-manifest PATH --installed-signature PATH]' \
    '' \
    'The signed installed release is required only when updater state has not been initialized.'
}

fail() {
  printf 'install-updater: %s\n' "$1" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

read_env_value() {
  key=$1
  default_value=$2
  value=$(
    awk -v key="$key" '
      index($0, key "=") == 1 {
        count++
        value = substr($0, length(key) + 2)
        sub(/\r$/, "", value)
      }
      END {
        if (count > 1) {
          exit 2
        }
        if (count == 1) {
          printf "%s", value
        }
      }
    ' "$BASE_ENV_FILE"
  ) || fail "$BASE_ENV_FILE contains duplicate $key entries"

  if [ -n "$value" ]; then
    printf '%s\n' "$value"
  else
    printf '%s\n' "$default_value"
  fi
}

validate_identifier() {
  name=$1
  value=$2
  case "$value" in
    ""|*[!A-Za-z0-9_.-]*) fail "$name contains unsupported characters" ;;
  esac
}

verify_ed25519_signature() {
  signed_file=$1
  signature_file=$2
  raw_signature=$3

  openssl base64 -d -A -in "$signature_file" -out "$raw_signature" \
    || fail "cannot decode signature: $signature_file"
  [ "$(wc -c < "$raw_signature" | tr -d ' ')" = "64" ] \
    || fail "invalid Ed25519 signature size: $signature_file"
  openssl pkeyutl -verify -pubin -inkey "$STAGED_PUBLIC_KEY" -rawin \
    -in "$signed_file" -sigfile "$raw_signature" >/dev/null 2>&1 \
    || fail "signature verification failed: $signed_file"
}

verify_sha256() {
  artifact_file=$1
  artifact_key=$2
  expected=$(jq -er --arg key "$artifact_key" '.artifacts[$key]' "$STAGED_HOST_MANIFEST") \
    || fail "host artifact hash is missing: $artifact_key"
  actual=$(openssl dgst -sha256 "$artifact_file" | awk '{ print $NF }')
  [ "$actual" = "$expected" ] || fail "host artifact hash mismatch: $artifact_key"
}

update_env_key() {
  env_file=$1
  key=$2
  value=$3
  temp_file=$(mktemp "$env_file.tmp.XXXXXX")
  BASE_ENV_TEMP=$temp_file

  awk -v key="$key" -v value="$value" '
    BEGIN {
      written = 0
    }
    index($0, key "=") == 1 {
      if (!written) {
        print key "=" value
        written = 1
      }
      next
    }
    {
      print
    }
    END {
      if (!written) {
        print key "=" value
      }
    }
  ' "$env_file" > "$temp_file"

  chown --reference="$env_file" "$temp_file"
  chmod --reference="$env_file" "$temp_file"
  mv -f "$temp_file" "$env_file"
  BASE_ENV_TEMP=
}

BINARY=
PUBLIC_KEY=
HOST_MANIFEST=
HOST_SIGNATURE=
RELEASE_URL=
INSTALLED_MANIFEST=
INSTALLED_SIGNATURE=

while [ "$#" -gt 0 ]; do
  case "$1" in
    --binary)
      [ "$#" -ge 2 ] || fail "--binary requires a value"
      BINARY=$2
      shift 2
      ;;
    --public-key)
      [ "$#" -ge 2 ] || fail "--public-key requires a value"
      PUBLIC_KEY=$2
      shift 2
      ;;
    --host-manifest)
      [ "$#" -ge 2 ] || fail "--host-manifest requires a value"
      HOST_MANIFEST=$2
      shift 2
      ;;
    --host-signature)
      [ "$#" -ge 2 ] || fail "--host-signature requires a value"
      HOST_SIGNATURE=$2
      shift 2
      ;;
    --release-url)
      [ "$#" -ge 2 ] || fail "--release-url requires a value"
      RELEASE_URL=$2
      shift 2
      ;;
    --installed-manifest)
      [ "$#" -ge 2 ] || fail "--installed-manifest requires a value"
      INSTALLED_MANIFEST=$2
      shift 2
      ;;
    --installed-signature)
      [ "$#" -ge 2 ] || fail "--installed-signature requires a value"
      INSTALLED_SIGNATURE=$2
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

[ "$(id -u)" -eq 0 ] || fail "run as root"
[ -n "$BINARY" ] || fail "--binary is required"
[ -x "$BINARY" ] || fail "updater binary is not executable: $BINARY"
[ -n "$PUBLIC_KEY" ] || fail "--public-key is required"
[ -f "$PUBLIC_KEY" ] || fail "release public key not found: $PUBLIC_KEY"
[ -n "$HOST_MANIFEST" ] || fail "--host-manifest is required"
[ -f "$HOST_MANIFEST" ] || fail "host artifact manifest not found: $HOST_MANIFEST"
[ -n "$HOST_SIGNATURE" ] || fail "--host-signature is required"
[ -f "$HOST_SIGNATURE" ] || fail "host artifact signature not found: $HOST_SIGNATURE"
[ -n "$RELEASE_URL" ] || fail "--release-url is required"
case "$RELEASE_URL" in
  https://*"?"*|https://*"#"*|*" "*|*"	"*) fail "release URL cannot contain query, fragment, or whitespace" ;;
  https://*) ;;
  *) fail "release URL must use HTTPS" ;;
esac

require_command awk
require_command chown
require_command chmod
require_command docker
require_command flock
require_command getent
require_command groupadd
require_command install
require_command jq
require_command mktemp
require_command mv
require_command openssl
require_command systemctl
require_command tr
require_command uname
require_command wc

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
UPDATER_UNIT_SOURCE="$SCRIPT_DIR/../systemd/ov-dash-updater.service"
COMPOSE_UNIT_SOURCE="$SCRIPT_DIR/../systemd/ov-dash-compose.service"
BASE_ENV_FILE=/opt/ov-dash/.env
STATE_DIR=/var/lib/ov-dash/updater
STATE_FILE="$STATE_DIR/deployment.json"
CURRENT_ENV_FILE="$STATE_DIR/current.env"
UPDATER_ENV_FILE=/etc/ov-dash/updater.env

[ -f "$UPDATER_UNIT_SOURCE" ] || fail "systemd unit not found: $UPDATER_UNIT_SOURCE"
[ -f "$COMPOSE_UNIT_SOURCE" ] || fail "systemd unit not found: $COMPOSE_UNIT_SOURCE"
[ -d /opt/ov-dash ] || fail "/opt/ov-dash does not exist"
[ -f /opt/ov-dash/docker-compose.yml ] || fail "/opt/ov-dash/docker-compose.yml does not exist"
[ -f "$BASE_ENV_FILE" ] || fail "$BASE_ENV_FILE does not exist"

SOCKET_PATH=$(read_env_value UPDATE_SOCKET_PATH /run/ov-dash/updater.sock)
DATABASE_NAME=$(read_env_value POSTGRES_DB ov_dash)
DATABASE_USER=$(read_env_value POSTGRES_USER ov_dash)
HEALTH_PORT=$(read_env_value HTTP_PORT 8080)

case "$SOCKET_PATH" in
  /run/ov-dash/*) ;;
  *) fail "UPDATE_SOCKET_PATH must be inside /run/ov-dash" ;;
esac
case "$SOCKET_PATH" in
  *[!A-Za-z0-9_./-]*) fail "UPDATE_SOCKET_PATH contains unsupported characters" ;;
esac
validate_identifier POSTGRES_DB "$DATABASE_NAME"
validate_identifier POSTGRES_USER "$DATABASE_USER"
case "$HEALTH_PORT" in
  ""|*[!0-9]*) fail "HTTP_PORT must be a TCP port number" ;;
esac
[ "$HEALTH_PORT" -ge 1 ] && [ "$HEALTH_PORT" -le 65535 ] || fail "HTTP_PORT must be between 1 and 65535"

TEMP_DIR=$(mktemp -d)
BASE_ENV_TEMP=
cleanup() {
  if [ -n "$BASE_ENV_TEMP" ]; then
    rm -f "$BASE_ENV_TEMP"
  fi
  rm -rf "$TEMP_DIR"
}
trap cleanup EXIT HUP INT TERM

STAGED_BINARY="$TEMP_DIR/ov-dash-updater"
STAGED_PUBLIC_KEY="$TEMP_DIR/release.pub"
STAGED_HOST_MANIFEST="$TEMP_DIR/host-artifacts.json"
STAGED_HOST_SIGNATURE="$TEMP_DIR/host-artifacts.json.sig"
STAGED_UPDATER_UNIT="$TEMP_DIR/ov-dash-updater.service"
STAGED_COMPOSE_UNIT="$TEMP_DIR/ov-dash-compose.service"
install -m 0755 "$BINARY" "$STAGED_BINARY"
install -m 0644 "$PUBLIC_KEY" "$STAGED_PUBLIC_KEY"
install -m 0600 "$HOST_MANIFEST" "$STAGED_HOST_MANIFEST"
install -m 0600 "$HOST_SIGNATURE" "$STAGED_HOST_SIGNATURE"
install -m 0644 "$UPDATER_UNIT_SOURCE" "$STAGED_UPDATER_UNIT"
install -m 0644 "$COMPOSE_UNIT_SOURCE" "$STAGED_COMPOSE_UNIT"

verify_ed25519_signature "$STAGED_HOST_MANIFEST" "$STAGED_HOST_SIGNATURE" "$TEMP_DIR/host-artifacts.sig"
jq -e '
  (keys | sort) == ["architecture", "artifacts", "schema_version", "version"] and
  .schema_version == 1 and
  (.version | type == "string" and length > 0) and
  (.architecture == "amd64" or .architecture == "arm64") and
  (.artifacts | keys | sort) == [
    "deploy/systemd/ov-dash-compose.service",
    "deploy/systemd/ov-dash-updater.service",
    "ov-dash-updater"
  ] and
  (.artifacts | all(.[]; type == "string" and test("^[a-f0-9]{64}$")))
' "$STAGED_HOST_MANIFEST" >/dev/null || fail "host artifact manifest is invalid"

case "$(uname -m)" in
  x86_64) HOST_ARCH=amd64 ;;
  aarch64|arm64) HOST_ARCH=arm64 ;;
  *) fail "unsupported host architecture: $(uname -m)" ;;
esac
MANIFEST_ARCH=$(jq -er '.architecture' "$STAGED_HOST_MANIFEST")
[ "$MANIFEST_ARCH" = "$HOST_ARCH" ] || fail "host artifact architecture mismatch"
HOST_VERSION=$(jq -er '.version' "$STAGED_HOST_MANIFEST")
verify_sha256 "$STAGED_BINARY" "ov-dash-updater"
verify_sha256 "$STAGED_UPDATER_UNIT" "deploy/systemd/ov-dash-updater.service"
verify_sha256 "$STAGED_COMPOSE_UNIT" "deploy/systemd/ov-dash-compose.service"

if [ ! -f "$STATE_FILE" ]; then
  [ -n "$INSTALLED_MANIFEST" ] || fail "--installed-manifest is required for first installation"
  [ -f "$INSTALLED_MANIFEST" ] || fail "installed manifest not found: $INSTALLED_MANIFEST"
  [ -n "$INSTALLED_SIGNATURE" ] || fail "--installed-signature is required for first installation"
  [ -f "$INSTALLED_SIGNATURE" ] || fail "installed signature not found: $INSTALLED_SIGNATURE"

  STAGED_MANIFEST="$TEMP_DIR/release.json"
  STAGED_SIGNATURE="$TEMP_DIR/release.json.sig"
  install -m 0600 "$INSTALLED_MANIFEST" "$STAGED_MANIFEST"
  install -m 0600 "$INSTALLED_SIGNATURE" "$STAGED_SIGNATURE"

  verify_ed25519_signature "$STAGED_MANIFEST" "$STAGED_SIGNATURE" "$TEMP_DIR/release.sig"
  jq -e '
    (keys | sort) == [
      "database", "expires_at", "health", "images", "minimum_version",
      "published_at", "release_id", "schema_version", "sequence", "version"
    ] and
    .schema_version == 1 and
    (.release_id | type == "string" and length > 0) and
    (.version | type == "string" and length > 0) and
    (.minimum_version | type == "string" and length > 0) and
    (.sequence | type == "number" and floor == . and . > 0) and
    (.images | keys | sort) == ["backend", "frontend"] and
    (.images | all(.[]; type == "string" and test("@sha256:[a-f0-9]{64}$"))) and
    (.database | keys | sort) == ["backup_required", "from_schema", "strategy", "to_schema", "transactional"] and
    (.database as $database |
      ($database.from_schema | type == "number" and floor == . and . >= 0) and
      ($database.to_schema | type == "number" and floor == . and . >= $database.from_schema)) and
    .database.strategy == "snapshot" and
    .database.backup_required == true
  ' "$STAGED_MANIFEST" >/dev/null || fail "installed release manifest is invalid"

  RELEASE_ID=$(jq -er '.release_id' "$STAGED_MANIFEST")
  VERSION=$(jq -er '.version' "$STAGED_MANIFEST")
  [ "$RELEASE_ID" = "ov-dash-$VERSION" ] || fail "installed release identity is invalid"
  [ "$VERSION" = "$HOST_VERSION" ] || fail "host artifacts and installed release versions differ"
  SEQUENCE=$(jq -er '.sequence' "$STAGED_MANIFEST")
  SCHEMA=$(jq -er '.database.to_schema' "$STAGED_MANIFEST")
  BACKEND_IMAGE=$(jq -er '.images.backend' "$STAGED_MANIFEST")
  FRONTEND_IMAGE=$(jq -er '.images.frontend' "$STAGED_MANIFEST")

  {
    printf 'APP_VERSION=%s\n' "$VERSION"
    printf 'BACKEND_IMAGE=%s\n' "$BACKEND_IMAGE"
    printf 'FRONTEND_IMAGE=%s\n' "$FRONTEND_IMAGE"
  } > "$TEMP_DIR/current.env"

  COMMITTED_AT=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
  jq -n \
    --arg release_id "$RELEASE_ID" \
    --arg version "$VERSION" \
    --argjson sequence "$SEQUENCE" \
    --argjson schema "$SCHEMA" \
    --arg backend "$BACKEND_IMAGE" \
    --arg frontend "$FRONTEND_IMAGE" \
    --arg release_env "$CURRENT_ENV_FILE" \
    --arg committed_at "$COMMITTED_AT" \
    '{
      release_id: $release_id,
      version: $version,
      sequence: $sequence,
      schema: $schema,
      images: {backend: $backend, frontend: $frontend},
      release_env: $release_env,
      committed_at: $committed_at
    }' > "$TEMP_DIR/deployment.json"
fi

if ! getent group ov-dash >/dev/null 2>&1; then
  groupadd --system ov-dash
fi
SOCKET_GID=$(getent group ov-dash | awk -F: 'NR == 1 { print $3 }')
case "$SOCKET_GID" in
  ""|*[!0-9]*) fail "cannot resolve the ov-dash group GID" ;;
esac
update_env_key "$BASE_ENV_FILE" UPDATE_SOCKET_GID "$SOCKET_GID"

install -d -m 0755 /usr/local/libexec
install -d -m 0750 -o root -g ov-dash /etc/ov-dash
install -d -m 0700 -o root -g root "$STATE_DIR" "$STATE_DIR/operations"
install -m 0755 -o root -g root "$STAGED_BINARY" /usr/local/libexec/ov-dash-updater
install -m 0640 -o root -g ov-dash "$STAGED_PUBLIC_KEY" /etc/ov-dash/release.pub

{
  printf 'OV_DASH_RELEASE_URL=%s\n' "$RELEASE_URL"
  printf 'OV_DASH_SOCKET_PATH=%s\n' "$SOCKET_PATH"
  printf 'OV_DASH_DATABASE_NAME=%s\n' "$DATABASE_NAME"
  printf 'OV_DASH_DATABASE_USER=%s\n' "$DATABASE_USER"
  printf 'OV_DASH_HEALTH_PORT=%s\n' "$HEALTH_PORT"
} > "$TEMP_DIR/updater.env"
install -m 0640 -o root -g ov-dash "$TEMP_DIR/updater.env" "$UPDATER_ENV_FILE.new"
mv -f "$UPDATER_ENV_FILE.new" "$UPDATER_ENV_FILE"
install -m 0644 -o root -g root "$STAGED_UPDATER_UNIT" /etc/systemd/system/ov-dash-updater.service
install -m 0644 -o root -g root "$STAGED_COMPOSE_UNIT" /etc/systemd/system/ov-dash-compose.service

if [ ! -f "$STATE_FILE" ]; then
  install -m 0600 -o root -g root "$TEMP_DIR/current.env" "$CURRENT_ENV_FILE.new"
  mv -f "$CURRENT_ENV_FILE.new" "$CURRENT_ENV_FILE"
  install -m 0600 -o root -g root "$TEMP_DIR/deployment.json" "$STATE_FILE.new"
  mv -f "$STATE_FILE.new" "$STATE_FILE"
fi

systemctl daemon-reload
systemctl enable ov-dash-updater.service ov-dash-compose.service
systemctl restart ov-dash-updater.service
systemctl restart ov-dash-compose.service
systemctl --no-pager --full status ov-dash-updater.service
systemctl --no-pager --full status ov-dash-compose.service
