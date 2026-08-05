#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
compose_file="$project_dir/docker-compose.server.yml"
backup_dir="$project_dir/backups"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"

cd "$project_dir"
set -a
source ./.env
set +a
mkdir -p "$backup_dir"

db_container="$(docker compose -f "$compose_file" ps -q db)"
if [[ -n "$db_container" ]] && [[ "$(docker inspect -f '{{.State.Running}}' "$db_container")" == "true" ]]; then
  docker compose -f "$compose_file" exec -T db \
    pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" |
    gzip >"$backup_dir/postgres-$timestamp.sql.gz"
fi

if docker volume inspect writers-studio-object-data >/dev/null 2>&1; then
  docker run --rm \
    --user "$(id -u):$(id -g)" \
    -v writers-studio-object-data:/data:ro \
    -v "$backup_dir:/backup" \
    alpine:3.22 \
    tar -czf "/backup/object-data-$timestamp.tar.gz" -C /data .
fi

echo "Backup completed in $backup_dir"
