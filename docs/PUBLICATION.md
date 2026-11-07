# Preparing a public repository

This Go/web edition is frozen and MIT-licensed. Keep the public documentation in
English and state its maintenance status honestly. Publishing code does not mean
publishing deployment configuration, recordings, account data or design sources.

## Review the snapshot

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

The release check inspects tracked and nonignored candidate files. It rejects known
private key/credential patterns, private filesystem/network references, design
metadata, private artifacts and non-English documentation. Synthetic private-network
rejection tests are narrowly exempted. Optional `PUBLICATION_PRIVATE_MARKERS`
(comma-separated, supplied locally) lets an operator check personal identifiers
without committing those identifiers into the checker.

Do not include `.env`, local CA certificates/keys, backups, `.git` metadata, local
agent state, generated build/test output, design source files or local desktop
wrapper artifacts. The separate desktop successor is not part of this repository.
Inspect binary examples and screenshots as well as text. Screenshots should use
synthetic fixtures; `bun run docs:screenshots` captures them from a local preview.
Build the frontend first with `bun run build` and install the browser with
`bunx --bun playwright install chromium`. Alternatively, use
`PLAYWRIGHT_CHANNEL=chrome bun run docs:screenshots` with an installed Chrome.

## Review every commit that will be published

Gitleaks is a separate executable; install it from its official distribution and
verify its checksum. Example with a recent CLI:

```sh
gitleaks git --log-opts="--all --full-history" --redact=100 \
  --max-archive-depth=3 --max-decode-depth=3
```

Also check branch/tag names, commit messages and author/committer metadata, embedded
archives and machine/user-specific references. A file deleted at the tip can still
exist in history. Do not suppress an unexplained token finding. Test-only exceptions
must be specific, documented and verified as synthetic.

## Clean a separate publication copy

Keep the source checkout/history intact. Rewrite a separate clone to remove private
files from every commit and replace deployment identifiers. Removing a file from
history changes commit IDs; preserve an audit mapping privately. Re-scan the
rewritten history, then add the reviewed English documentation/current source in
one publication-preparation commit.

Never copy the source `.git` directory, reflogs, private remotes, local tool refs,
ignored files or audit reports into that copy. Never use `git push --mirror` from
the private checkout. If a real credential was exposed, revoke/rotate it first;
rewriting history does not revoke it or erase someone else's copies.

## Publish explicitly

Use the reviewed copy and a deliberately selected public remote. Inspect
`git remote -v`, `git status`, the complete history and the final diff. Push only
the intended branch, not all local refs or the source checkout's internal remotes.
Do not force-push a nonempty public repository without coordinating its existing
history. Keep detailed pre-cleanup evidence and private identifiers in local,
Git-ignored audit files. Public reports should describe findings without reproducing
the removed data.

References: [Gitleaks](https://github.com/gitleaks/gitleaks) and
[GitHub's sensitive-data removal guide](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/removing-sensitive-data-from-a-repository).
