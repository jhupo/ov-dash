#!/usr/bin/env sh
set -eu

for migration in /migrations/*.sql; do
  [ -e "$migration" ] || continue
  echo "running migration: $migration"
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" -f "$migration"
done
