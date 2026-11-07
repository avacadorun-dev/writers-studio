# Publication audit

Date: 2026-11-08. Scope: source-publication privacy review of the frozen Go/web
edition, the complete eight-commit `main` ancestry, candidate documentation,
synthetic demo archive and documentation screenshots.

This is not a penetration test, a dependency-vulnerability certification or a
guarantee that every possible sensitive string has been recognized.

## Findings and resolution

| ID | Severity | Finding | Publication treatment |
| --- | --- | --- | --- |
| PUB-01 | High | A design source contained opaque service metadata reported as a possible key | Remove all design source files from every published commit; no token value is reproduced |
| PUB-02 | Medium | Historical documentation/configuration contained private deployment addresses and personal filesystem paths | Replace them in a separate rewritten publication history; use operator-supplied configuration at the tip |
| PUB-03 | Medium | A private execution prompt and personal publishing references were present | Remove the prompt from all published history and replace personal examples |
| PUB-04 | Low | Commit identities can disclose personal account metadata | Use a generic contributor identity in the publication copy |

The opaque design metadata was not tested against an external service and is not
asserted to be an active credential. If it grants access in the originating design
service, invalidate it there; deleting Git history does not revoke credentials.

## Methods and evidence boundaries

- Enumerated every reachable commit and 946 unique blob objects in the source.
- Scanned history with checksum-verified Gitleaks 8.30.1, including archive and
  encoded-content inspection. No history match was found for the current local
  provider credentials checked privately; their values were never printed.
- Classified one remaining generic-key finding as a synthetic audio object path
  in an integration fixture. `.gitleaksignore` records only its precise sanitized
  history fingerprint; it does not exempt the whole test file or credential rules.
- Checked private address/path references, personal links, SSH references, tracked
  environments/keys, author/committer metadata and the native demo archive.
- Inspected screenshot content and replaced all twelve figures using browser-only
  synthetic fixtures in the light Fog and Graphite theme.
- Rewrote a separate clone. The source repository, ignored environment and local
  recovery/audit files are not part of the publication copy.

The source history remains private and unchanged. Publish only the reviewed
rewritten copy; do not push the original refs or copy its `.git` directory.
Historic documents can remain in their original language in earlier commits;
the current public documentation is English. No desktop successor source is included.

## Reproduction and continuing checks

Run `bun run release:check` and the full-history Gitleaks command in
[the publication guide](PUBLICATION.md). Inspect binary/media changes and new
identifiers manually. Run the source checks in `.github/workflows/ci.yml` and the
browser/integration checks appropriate to later changes. A clean pattern scan is
useful evidence with explicit limits, not a universal absence-of-secrets proof.

## Checks performed for the publication snapshot

| Check | Result |
| --- | --- |
| English help navigation/links/assets | 14 pages checked |
| Biome, TypeScript and frontend production build | Passed |
| Frontend unit suite | 241 tests passed |
| Documentation browser/accessibility checks | 8 tests passed across four viewport configurations |
| Go unit suite and race-enabled suite | Passed |
| Final HTTP package race check after context cleanup | Passed |
| `go vet ./...` | Passed |
| Docker/PostgreSQL integration and full application E2E | Not run: local Docker daemon unavailable |

Browser documentation checks used installed Chrome against a local static preview.
Screenshots use synthetic API fixtures and do not certify backend integration.
Private detailed logs remain outside the publication snapshot.
