# Configuration

The Go server reads environment variables. Docker Compose reads `.env` for
substitution; `go run` does not automatically load it. Keep real values in a
Git-ignored local file or a secret manager. `.env.example` contains public examples.

| Variable | Meaning / default |
| --- | --- |
| `APP_ENV` | Environment; `development`; `production` rejects test adapters |
| `HTTP_ADDR` | Listen address; `:8080` |
| `STATIC_DIR` | Built frontend directory; `dist` |
| `DATABASE_URL` | PostgreSQL DSN; local development credentials by default |
| `PUBLIC_BASE_URL` | External application origin; `http://localhost:8080` |
| `PUBLIC_HOST` | HTTPS proxy hostname, configured by the deployment operator |
| `OBJECT_STORAGE_ENDPOINT` | S3-compatible endpoint; `localhost:9000` |
| `OBJECT_STORAGE_ACCESS_KEY` | Storage access key; development example `writers` |
| `OBJECT_STORAGE_SECRET_KEY` | Storage secret; development example `writers-secret` |
| `OBJECT_STORAGE_BUCKET` | Bucket; `writers-studio` |
| `OBJECT_STORAGE_USE_SSL` | Storage TLS; `false` for local MinIO |
| `STT_ADAPTER` | `auto`, `disabled`, `openrouter`, or test-only `fake` |
| `LIVE_STT_ADAPTER` | `auto`, `disabled`, `deepgram`, `openrouter`, or test-only `fake` |
| `DEEPGRAM_API_KEY` | Server-only live speech credential |
| `DEEPGRAM_STT_MODEL` | Live model; `nova-3` |
| `OPENROUTER_API_KEY` | Server-only file speech credential |
| `OPENROUTER_STT_MODEL` | File transcription model; `openai/whisper-large-v3` |
| `LLM_ADAPTER` | `auto`, `disabled`, `openai_compatible`, or test-only `fixture` |
| `LLM_BASE_URL` | Server-side model endpoint; unset by default |
| `LLM_API_KEY` | Server-only system model credential |
| `LLM_MODEL` | Exact provider model identifier |
| `LLM_DIALECT` | `chat_completions` or `responses`; default `chat_completions` |
| `LLM_CREDENTIAL_ENCRYPTION_KEY` | Stable key protecting user provider credentials |
| `LLM_ALLOW_PRIVATE_ENDPOINTS` | Explicit opt-in for private model endpoints; `false` |
| `LLM_REQUEST_TIMEOUT` | Shared provider timeout, `1s`–`10m`; default `10m` |
| `MCP_ENABLED` | Optional remote MCP gateway; `false` |
| `MCP_OAUTH_SECRET` | Separate consent secret; at least 32 characters when enabled |
| `ALLOW_TEST_ADAPTERS` | Explicit non-production fixture permission; `false` |

`auto` chooses configured providers; missing keys disable the corresponding
capability. A completed Deepgram session does not automatically send audio to a
second transcription provider. User-defined LLM base URLs refer to the server's
network, not the browser's network.

The production Compose file also requires database/storage credentials and
`PUBLIC_BASE_URL`. Keep `LLM_CREDENTIAL_ENCRYPTION_KEY` consistent across restarts
and backups when using saved provider connections. Review `internal/config` and
Compose files for validation and deployment-specific values.
