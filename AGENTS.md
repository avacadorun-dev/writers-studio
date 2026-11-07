# Contributor and agent instructions

This is the frozen Go/web edition of Writer's Studio. Document existing behavior;
do not treat older roadmap proposals as authorization to build new features.
The desktop successor is developed separately.

## Git and workspace

- Never create branches with the `codex/` prefix. Use the existing convention or
  descriptive `fix/`, `feature/`, `release/`, and `chore/` prefixes.
- Preserve existing local changes. Do not reset files, rewrite the source
  repository's history, publish, or push without task authorization.
- Do not commit local configuration, credentials, application state, backups,
  design-service metadata, raw test artifacts, personal recordings or generated build artifacts.
- Use English for repository documentation and new explanatory comments.

## Architecture and tools

- Backend: Go modules; commands under `cmd/`, application code under `internal/`.
- Frontend: React/TypeScript, Feature-Sliced Design, Bun, Vite and Biome.
- Follow [frontend boundaries](agent-guides/frontend-architecture.md),
  [UI conventions](agent-guides/ui-and-styling.md),
  [API/configuration rules](agent-guides/api-auth-and-env.md),
  [testing](agent-guides/testing.md), and
  [tooling checks](agent-guides/tooling-and-checks.md).
- Use Go contexts across blocking operations, explicit cancellation ownership,
  bounded resources, wrapped errors, and owner-scoped database access.
- Keep dependencies small and preserve `go.sum` and `bun.lock` integrity.

## Data guarantees

Source audio and transcript artifacts are immutable. Revisions are append-only;
restores and Undo create compensating revisions. Assistant writes remain staged
until authorized Apply. Preserve exact anchors, base revisions, atomic changes,
project ownership checks, import bounds, and backward-compatible formats.

## Verification

Run applicable tests and state failures or unavailable checks accurately:

```sh
bun run release:check
bun run check
bun run type-check
bun run test
bun run build
go test ./...
go test -race ./...
go vet ./...
```

Format changed Go files with `gofmt`. Run integration and browser tests for
changes to persistence or user workflows. Never present historical coverage
numbers as results from the current checkout.
