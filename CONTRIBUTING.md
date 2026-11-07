# Contributing

This Go/web edition is frozen while development moves to a separate desktop
application. Forks are welcome. There is no promise of new releases, pull-request
review, or security support. Discuss a substantial change in an issue before
investing time in a proposed upstream contribution.

## Setup and checks

Use Go 1.26+, Bun, Docker Engine and Compose v2. Follow the root README for setup.
Use Bun for frontend commands and Biome for TypeScript formatting/linting.

```sh
bun install --frozen-lockfile
bun run check
bun run type-check
bun run test
bun run build
go test ./...
go test -race ./...
go vet ./...
```

Integration tests use a dedicated PostgreSQL database supplied through
`TEST_DATABASE_URL`; never point them at a database containing real work.
`bun run e2e:compose` creates an isolated Compose project and temporary test volumes.
Real-provider tests are opt-in and may send test audio to external services.

## Change guidelines

- Follow [AGENTS.md](AGENTS.md) and the focused guides in `agent-guides/`.
- Use descriptive branch prefixes such as `fix/`, `feature/`, or `chore/`.
- Keep changes focused. Explain the observed problem, resulting behavior, checks,
  and material limitations in the pull request.
- Preserve old document revisions, transferable project formats and explicit
  assistant review. Database changes use additive migrations.
- Add meaningful tests for changed behavior. Do not include generated output,
  dependencies, credentials, machine-specific configuration or private data.
- Write contributor documentation and new comments in English. Russian dialogue
  in synthetic fixtures and the existing application UI is intentional.

## Reporting bugs

Provide reproduction steps, version, environment and expected/actual behavior.
Use synthetic text and audio. Redact cookies, authorization headers, tokens,
provider configuration, usernames and server addresses from logs and screenshots.
Follow [SECURITY.md](SECURITY.md) for potential vulnerabilities.

## License

Contributions are offered under this repository's MIT license. Keep third-party
notices and license terms for any reused material; the project license does not
replace dependency licenses.
