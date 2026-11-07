# Project navigation and transfer

## Navigation

Every secondary screen provides a visible, accessible return action with at least
a 44×44 px target. Root login/library screens are exceptions. Global utilities
return to the library; project utilities return to the project/Studio; dictation,
versions and review return to their relevant document/project. Direct URLs must
have a safe fallback rather than relying only on browser history.

## Native project archive

The `.wstudio` container is an unencrypted ZIP with MIME
`application/vnd.writers-studio.project+zip`. `project.json` declares
`format: "writers-studio-project"` and `formatVersion`. The current exporter writes
v8; the importer accepts v1–v8 and rejects future versions before mutation.

| Version | Added compatibility contract |
| --- | --- |
| v1 | Project structure, revisions, versions, dictionary, dictation/audio, guide/memory |
| v2 | Document annotations and exact anchors |
| v3 | Project images and reference remapping |
| v4 | Reader export profile and cover reference |
| v5 | Markdown frontmatter/source paths and canonical internal links |
| v6 | Canvas boards, trash and complete snapshot history |
| v7 | Typed Canvas schemas |
| v8 | Expanded Canvas media/resource support |

Exports include available source audio and binary media with paths, sizes, SHA-256
and MIME metadata. They exclude accounts/passwords/cookies, API keys, provider
connections, server sessions, temporary chunks/jobs/idempotency keys and assistant
execution history. Reader exports are reproducible and are not embedded as a
substitute for canonical content. A project archive contains private author work;
it is not safe to publish automatically.

## Import guarantees

Import creates an independent active copy, including when a project name matches.
Validate the unique manifest, version, size/expansion limits, safe ZIP paths, enums,
references, hierarchy, IDs and binary integrity before exposing a project.
Unexpected files, symlinks, traversal, invalid anchors and future versions fail.
The upload limit is 512 MiB and manifest limit is 64 MiB.

Remap all project/entity IDs and internal references in one transaction. Never
import source ownership/provider/run identities. Remove uploaded objects if the
transaction fails. Normalize unfinished dictation to safe terminal states without
starting jobs. Legacy versions omit their unavailable features rather than
modifying old text/revisions. Fail export when declared required audio is missing
rather than returning a misleading partial success.

## Acceptance references

- AC-NAV-01/02: search, recovery and every secondary route expose an explicit return.
- AC-PT-01: current archives validate and supported legacy versions import.
- AC-PT-02: copied projects preserve canonical material and applicable history/media.
- AC-PT-03: copied IDs differ and every internal reference remains valid.
- AC-PT-04: invalid format/reference/checksum leaves no partial project.
- AC-PT-05: no account-level credential or provider connection enters the archive.
