#!/usr/bin/env bash
set -Eeuo pipefail

readonly project_name="writers-studio-e2e"
readonly base_url="http://127.0.0.1:18080"
readonly -a compose=(
  docker compose
  -p "${project_name}"
  -f docker-compose.yml
  -f docker-compose.e2e.yml
)

cleanup() {
  "${compose[@]}" down -v --remove-orphans
}

trap cleanup EXIT INT TERM
cleanup
"${compose[@]}" up -d --build --wait
if ! PLAYWRIGHT_BASE_URL="${base_url}" bunx playwright test --retries=0 "$@"; then
  "${compose[@]}" logs app
  exit 1
fi
