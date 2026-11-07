# Architecture reference (arc42)

## 1. Goals and scope

Writer's Studio is a self-hosted Go/web workspace for long-form writing, voice
capture, source review, visual planning and controlled assistant actions. This
reference describes the frozen edition, not a future desktop roadmap.

The main quality goals are preservation of confirmed work, useful live feedback,
author control over revisions, and bounded agent permissions. A recognizer, model,
late job or reconnect must not silently replace manual edits.

## 2. Constraints

The server is a modular monolith. React/TypeScript, Tiptap/ProseMirror and React
Flow form the browser client. PostgreSQL stores canonical metadata and revisions;
S3-compatible storage holds binary objects. IndexedDB retains unfinished browser
work. External STT and LLM services are accessed through server-side adapters.

The supported deployment model is a single-author account with owner-scoped
projects. Concurrent assistant runs are supported; collaborative real-time
editing and automatic public publishing are outside this edition.

## 3. Context and trust boundaries

| Boundary | Responsibility |
| --- | --- |
| Browser → server | Authenticated HTTP/WebSocket, validation and owner scope |
| Server → PostgreSQL | Transactions, revisions, jobs, grants and durable history |
| Server → object storage | Private audio/media/export objects and integrity metadata |
| Server → speech provider | Explicit audio processing, normalized events and errors |
| Server → model provider | Scoped selected context, strict tools and credential isolation |
| External agent → MCP | Authenticated project grants, server-owned session context |

Manuscripts, audio, transcripts, assistant history and provider credentials are
sensitive. Technical logs must omit their content. Project text and tool results
are untrusted data and cannot expand the assistant's permissions.

## 4. Solution strategy

Keep capture, recognition, editorial work and assistant proposals separate.
Preserve immutable source artifacts and append document revisions. Use optimistic
base revisions to detect conflicts; preserve alternate work rather than losing it.
Run background jobs independently of browser pages. Stage model mutations and
apply selected dependency-consistent groups atomically.

## 5. Building blocks

| Package / area | Role |
| --- | --- |
| `cmd/server` | Server composition and process lifecycle |
| `internal/httpapi` | HTTP routes, authentication, validation and WebSocket gateways |
| `internal/store` | Owner-scoped PostgreSQL operations and additive migrations |
| `internal/stt` | Capability-based speech adapters and event normalization |
| `internal/llm` | Provider adapters, probes and encrypted credentials |
| `internal/projectmcp` | Internal project tool catalog and execution policy |
| `internal/assistant` | Run/turn/change-set models and orchestration contracts |
| `internal/editor` | Canonical rich-document validation and exact operations |
| `internal/canvas` | Snapshot validation, typed metadata and transfer contracts |
| `internal/export` | Snapshot-based reader output |
| `internal/markdowntree` | Bounded semantic Markdown tree migration |
| `src/app` | Providers, routes and application composition |
| `src/pages`, `widgets`, `features`, `entities`, `shared` | Feature-Sliced frontend layers |

Dependencies flow downward through frontend layers. Public slice entrypoints hide
implementation details. Shared UI is domain-agnostic. Bun and Biome are the
frontend runtime/package-manager and formatter/linter conventions.

## 6. Runtime scenarios

### Manual editing

Load a revision, edit locally, persist an emergency snapshot and submit a new
revision with its base. Acknowledgement updates save status without resetting
editor content, selection or undo history. A stale write produces a conflict
path/copy. Version restoration appends a compensating revision.

### Dictation

Preflight verifies microphone access, recording capability, available storage,
destination and recognition configuration. MediaRecorder stores source chunks;
AudioWorklet sends live PCM frames independently. Local capture continues during
recognizer/network failure. Uploads are idempotent and acknowledged separately.

Successful Deepgram finalization promotes stable live segments to the immutable
source transcript. A quality pass or re-transcription is a separate artifact;
manual drafts remain unchanged until an explicit comparison/acceptance step.
Source references connect manuscript blocks to bounded audio intervals.

### Assistant

A run pins provider/model, visible target, permitted context and guide revision.
Every turn starts with semantic `assistant.begin_turn`. Read tools retrieve bounded
context; write tools accumulate proposals. The user reviews exact changes before
Apply. Base revisions, expected text, anchors and protected ranges are checked
again in the transaction. Undo restores before-snapshots using new revisions.

Voice and chat share a run. Editable voice drafts survive reconnect and never
submit merely because input mode changed. Hiding a panel or switching conversations
does not cancel background execution. Stop explicitly cancels further work.

### Export and import

Reader exports use a stable ordered snapshot rather than rereading moving data.
Native archives transfer the complete project with remapped IDs and validated
binary objects. Markdown migration preserves semantic text, paths and references,
while excluding account/provider state and complete runtime history.

## 7. Persistence and deployment

PostgreSQL stores projects, document trees/revisions, named versions, annotations,
transcript artifacts/drafts, jobs, guide/memory history, assistant activity and MCP
grants. Object storage stores audio, project media and generated exports.
IndexedDB stores local audio manifests/chunks and emergency editor/Canvas snapshots.

The standard Compose stack is a local development environment. Production uses
separate credentials and trusted HTTPS. Database and object-storage backups must
be coordinated; encryption keys are backed up securely outside Git.

## 8. Cross-cutting contracts

- Authentication establishes the owner; resource lookup validates project ownership.
- Scope and active project are server-owned, never accepted from model arguments.
- Mutations validate base versions and use idempotency where retries are allowed.
- Source audio/transcripts are immutable; restores and Undo are compensating writes.
- Document annotations are separate metadata with Unicode-safe exact anchors.
- Browser recovery distinguishes local-only content from acknowledged server data.
- Provider errors are normalized without exposing credentials or raw private payloads.
- Token/grant storage is hash-only; saved provider credentials use encryption at rest.
- Purge requires explicit confirmation; external backup retention remains operational.

## 9. Decisions

See [ADR-001](../adr/ADR-001-speech-to-text.md),
[ADR-002](../adr/ADR-002-editor-engine.md),
[ADR-003](../adr/ADR-003-assistant-mcp.md),
[ADR-007](../adr/ADR-007-assistant-continuous-workspace.md) and
[ADR-008](../adr/ADR-008-remote-mcp-gateway.md).

## 10. Quality scenarios

Verify interrupted recording/upload, reload recovery, late recognition, stale
editor/assistant writes, partial provider output, process restart, exact anchor
mapping, archive round-trips and purge cleanup. Browser checks cover wide desktop,
compact desktop, tablet and mobile. Accessibility includes keyboard operation,
visible focus, textual statuses and touch targets.

## 11. Risks and limitations

Browser quotas, explicit site-data deletion and device failure can destroy local
recovery. External providers have changing capabilities, costs and data policies.
A frozen repository has no guaranteed security update stream. Automated tests and
historical specifications do not establish production readiness for every host.

## 12. Terminology

A source artifact is immutable recognition output. A draft is editable working
text. A revision is a durable document snapshot. A named version is an explicit
snapshot separate from autosave. A change set is staged assistant work. A guide
contains writing instructions; memory contains typed project facts; the dictionary
contains canonical terms. Document schema versions and archive versions are
independent compatibility contracts.
