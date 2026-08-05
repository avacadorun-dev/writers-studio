#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
compose_file="$project_dir/docker-compose.server.yml"

cd "$project_dir"
if [[ ! -f .env ]]; then
  echo "Missing $project_dir/.env; run deploy/init-server-env.sh first" >&2
  exit 1
fi

if docker compose -f "$compose_file" ps -q db | grep -q .; then
  "$project_dir/deploy/backup-server.sh"
fi

docker compose -f "$compose_file" up -d --build --remove-orphans

for _ in $(seq 1 60); do
  health="$(docker inspect --format '{{.State.Health.Status}}' writers-studio-app-1 2>/dev/null || true)"
  if [[ "$health" == "healthy" ]]; then
    docker compose -f "$compose_file" ps
    exit 0
  fi
  sleep 2
done

docker compose -f "$compose_file" ps
docker compose -f "$compose_file" logs --tail=100 app
echo "Application did not become healthy" >&2
exit 1
