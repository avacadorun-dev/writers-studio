# Verification guide

This frozen repository contains historical implementation evidence. It does not
promise that old test counts, coverage percentages, screenshots or private-server
smoke results describe your checkout. Run the checks below for a new installation
or fork. Publication-specific results are recorded separately in the local handoff.

## Frontend and documentation

```sh
bun install --frozen-lockfile
bun run release:check
bun run check
bun run type-check
bun run test
bun run build
bun run test:canvas:coverage
bun run test:rich-blocks:coverage
```

The documentation checks validate frontmatter, navigation, links, local images and
example downloads. The release check rejects known private-material patterns;
a full-history secret scanner is an additional requirement before publication.

## Go

```sh
go test ./...
go test -race ./...
go vet ./...
```

Use a disposable PostgreSQL database for persistence tests:

```sh
TEST_DATABASE_URL='postgres://writers:writers@localhost:5432/writers_studio_test?sslmode=disable' \
  go test -tags=integration ./internal/store -count=1
```

Do not point integration tests at real author data. Optional `govulncheck` and
`staticcheck` need separately installed tools and may identify dependency issues
that ordinary unit tests cannot find.

## Browser and provider tests

```sh
bun run e2e:compose
```

The isolated Compose harness uses explicitly gated test adapters and temporary
containers/volumes; it must not target a production installation. Playwright covers
wide/compact desktop, tablet and mobile. Visual/accessibility review remains useful
for dialogs, independent scroll, rich blocks, source playback and recovery.

Real speech tests need a consented/synthetic WAV, configured provider keys and the
`e2e:real-stt` command. They can incur external cost and transmit test audio. A
fixture pass is not evidence of live provider availability or accuracy.

## Interpretation

Record command, checkout, environment, exit status and unavailable dependencies.
A build passing does not prove data recovery, integration behavior, security or
provider compatibility. Preserve real failures and limitations in the handoff
rather than converting old evidence into a release approval.
