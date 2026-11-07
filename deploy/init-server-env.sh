#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
providers_file="${1:-$project_dir/.env.providers}"
target_file="$project_dir/.env"

if [[ ! -f "$providers_file" ]]; then
  echo "Provider environment file not found: $providers_file" >&2
  exit 1
fi

set -a
source "$providers_file"
set +a

if [[ -z "${DEEPGRAM_API_KEY:-}" ]]; then
  echo "DEEPGRAM_API_KEY is required" >&2
  exit 1
fi

: "${PUBLIC_HOST:?Set PUBLIC_HOST to your deployment hostname}"
: "${PUBLIC_BASE_URL:?Set PUBLIC_BASE_URL to your HTTPS origin}"

postgres_password="$(openssl rand -hex 24)"
storage_secret="$(openssl rand -hex 24)"
mcp_oauth_secret="$(openssl rand -hex 32)"
temporary_file="$(mktemp "$project_dir/.env.XXXXXX")"
trap 'rm -f "$temporary_file"' EXIT
umask 077

{
  printf 'COMPOSE_PROJECT_NAME=writers-studio\n'
  printf 'APP_IMAGE_TAG=latest\n'
  printf 'APP_PORT=8080\n'
  printf 'POSTGRES_DB=writers_studio\n'
  printf 'POSTGRES_USER=writers\n'
  printf 'POSTGRES_PASSWORD=%s\n' "$postgres_password"
  printf 'DATABASE_URL=postgres://writers:%s@db:5432/writers_studio?sslmode=disable\n' "$postgres_password"
  printf 'OBJECT_STORAGE_ACCESS_KEY=writers\n'
  printf 'OBJECT_STORAGE_SECRET_KEY=%s\n' "$storage_secret"
  printf 'OBJECT_STORAGE_BUCKET=writers-studio\n'
  printf 'STT_ADAPTER=openrouter\n'
  printf 'LIVE_STT_ADAPTER=deepgram\n'
  printf 'OPENROUTER_API_KEY=%s\n' "${OPENROUTER_API_KEY:-}"
  printf 'OPENROUTER_STT_MODEL=%s\n' "${OPENROUTER_STT_MODEL:-openai/whisper-large-v3}"
  printf 'DEEPGRAM_API_KEY=%s\n' "$DEEPGRAM_API_KEY"
  printf 'DEEPGRAM_STT_MODEL=%s\n' "${DEEPGRAM_STT_MODEL:-nova-3}"
  printf 'PUBLIC_BASE_URL=%s\n' "$PUBLIC_BASE_URL"
  printf 'PUBLIC_HOST=%s\n' "$PUBLIC_HOST"
  printf 'MCP_ENABLED=false\n'
  printf 'MCP_OAUTH_SECRET=%s\n' "$mcp_oauth_secret"
} >"$temporary_file"

chmod 600 "$temporary_file"
mv "$temporary_file" "$target_file"
rm -f "$providers_file"
trap - EXIT
echo "Production environment initialized at $target_file"
