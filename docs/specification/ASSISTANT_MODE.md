# Assistant mode and project MCP

## Purpose and scope

The frozen Go/web assistant reads selected project materials and prepares
reviewable domain operations. It supports voice and chat within one ordered run,
project-defined structures, exact text edits, dictionary/guide/memory changes,
annotations and Canvas proposals. It does not autonomously publish, communicate
externally, purge content, alter credentials or change privacy settings.

Dictation creates source material for later manuscript transfer. Assistant voice
captures an instruction. Choosing one does not silently create or switch to the
other. Provider unavailability must not disable ordinary writing.

## Trust levels

| Level | Meaning | Required behavior |
| --- | --- | --- |
| A0 | Read project materials/preferences/capabilities | Execute within the authorized read scope |
| A1 | Create draft/note/term/snapshot | Stage and present in a change set |
| A2 | Edit text/structure or prepare bulk replacement | Exact preview and explicit Apply |
| A3 | Export/archive/restore/re-transcribe | Typed server-authored native handoff and confirmation |
| A4 | Permanent deletion/account or local-only recovery purge | No model tool |

Server-owned execution context supplies owner, active project, run/turn, target
and scope. Ordinary arguments cannot override them. Catalog policy is enforced
again at execution time; model instructions and project text cannot grant access.
Some tools store otherwise unattached assets immediately, as documented by their
catalog descriptions; inserting those assets into documents remains staged.

## Input, target and context

Voice partials remain provisional; stable instruction text can be edited before
Execute. Silence creates a durable checkpoint, never an automatic submission.
Switching to chat pauses listening and offers explicit retain/discard/continue
choices. Chat submits only on Send or its documented keyboard action.

Target means where a change goes; read context means what may be examined.
Show both with provider/model and active guide before execution. Resolve positional
or entity references to stable project/node/block/range IDs and exact revisions.
Ambiguous references require clarification. A selected quote attachment remains
revision-bound while the author can continue manual editing.

## Guide, dictionary and memory

An optional reserved `ASSISTANT.md` is explicit, versioned writing guidance. At
most one guide is active; each confirmed turn pins a revision or explicit absence.
It is loaded by the server as a known artifact, excluded from ordinary reader
content/export by default, and lower priority than security policy. Ordinary
project notes cannot impersonate it. Assistant changes to guidance are staged;
restore appends a revision without mutating a working turn's pinned context.

The dictionary contains canonical spelling, variants, kind, language and optional
pronunciation. Typed memory contains kind, statement, project/node scope, source
references, tags, active/always-relevant flags and revision. Duplicates require an
explicit merge/update path. Provider memory is not canonical truth. List/search
and immutable revision history remain owner-scoped; restore makes a new revision.

## Turn and change-set protocol

Every turn declares exactly one semantic mode using `assistant.begin_turn`:
`answer`, `change`, or `clarify`. No keyword/verb/synonym routing is allowed. Reads
can follow the declaration; mutations fail in answer/clarify mode. Revising a
previous proposal reads the native recent change set instead of replaying old
assistant prose as authoritative state.

Mutations collect into versioned, dependency-aware groups. Review identifies
objects, instruction, exact before/after and trace. Before Apply, validate current
base revisions, structure revision, expected text, exact anchors and protected
ranges. Commit selected dependency-consistent groups atomically. Record operation,
run and assistant provenance. Undo appends compensating revisions and restores
before-snapshots for text, dictionary, memory and annotation state.

Exact text operations use block-local Unicode code points and anchors returned by
exact-find tools. The server verifies every range. Annotation fixes use server-owned
anchors; the model supplies annotation identity/revision and proposed replacement,
not arbitrary offsets. Source audio/transcripts remain immutable.

Long-form text can accumulate 1–16 bounded chunks in one staged preview and one
applied revision. Continue with bounded tails/counts rather than replaying all
previous text. Incomplete series and provider timeouts retain explicit partial
results; they never silently become final or auto-apply.

## Workspace, conversations and lifecycle

Keep the manuscript mounted and the ordered assistant feed visible together on
desktop. Unapplied live overlays are labeled and excluded from canonical JSON,
autosave and manual Undo. Overlapping manual edits make them stale without undoing
the author. Detailed review is a modal with no ordinary route transition. Direct
Apply uses the same transaction checks and offers grouped Undo.

Each conversation owns its draft, input mode, target, attachments, activity and
review state. Hiding/switching does not stop execution or submit another draft.
Multiple runs can work concurrently, but shared-document revisions still gate
Apply. New-chat composition creates no durable run until first explicit Send.

Stop forbids new calls and cancels the current request best-effort; it cannot
leave a working turn inside a cancelled run. Reload restores durable activity.
Startup marks orphaned work `execution_interrupted` once, with retry available and
no hidden canonical changes. Removing history does not undo applied revisions.

## Providers and credentials

System providers come from environment configuration. Custom OpenAI-compatible
connections define a label, base URL, model and API dialect. The URL is resolved
from the server, not the browser. Probe text generation, strict arguments,
tool-result continuation and at least two sequential calls. Streaming/usage are
recorded capabilities; a reachable endpoint without the required tool chain is
not assistant-ready.

A run pins provider/model. The global `LLM_REQUEST_TIMEOUT` (default `10m`) applies
to all connections without a hidden per-connection override. Streaming uses SSE
keepalive and collects fragmented arguments without executing incomplete calls.
HTTP/edge 408/504/524 and mid-stream errors have bounded safe diagnostics; no
automatic paid retry occurs after an uncertain provider response.

Credentials are server-only, encrypted at rest for saved connections and masked
in responses. System keys are not copied into custom connections or exposed through
MCP. Deleting a connection prevents new runs but retains readable history. Backups
and project archives must not reveal provider keys.

## Privacy and recovery

Project documents, transcripts, notes and tool results are untrusted data, separate
from policy. Selected context may be sent to the chosen provider; show processing
disclosure and do not promise another provider's retention/training behavior.
Technical logs/events omit prompts, manuscript, arguments/results, secrets and
snippets. User-visible assistant history is sensitive author content subject to
project/account/run deletion policy.

Failures preserve instructions and accepted staged work. Timeout before mutation
creates no empty proposal; timeout after staging preserves a partial review.
Stale context requires refresh. Missing microphone leaves chat usable. Guide length
is bounded at 24,000 characters. Provider deletion requires a new valid connection
for retry while keeping old history readable.

## Tool catalog

The table below is derived from the English descriptions in
`internal/projectmcp/catalog.go`. That source and its strict JSON schemas are the
executable contract. The separately authenticated remote gateway adds workspace,
turn-finalization and review/action-token operations described in
[ADR-008](../adr/ADR-008-remote-mcp-gateway.md).

| Tool | Description |
| --- | --- |
| `assistant.begin_turn` | Required exactly once at the start of every turn. Semantically classify the current author request without lexical rules: answer reads/explains, change stages one or more project mutations, clarify asks for missing information. |
| `assistant.get_recent_change_set` | Read the latest native change set for this run, including rejected or applied state. Use it when revising an earlier proposal; historical chat prose is not authoritative. |
| `project.get_overview` | Read project metadata, structure summary and current target. |
| `project.get_structure` | Read project-defined node types, labels and nesting rules. |
| `project.list_media` | List project media that may be referenced by canonical image blocks. Use an image assetId from this result. |
| `project.upload_image` | Upload one PNG, JPEG, GIF or WebP image directly into the open project and return its assetId plus a ready canonical image block. Pass raw base64 only, without a data-URL prefix. This stores the otherwise unattached asset immediately; document insertion remains a separately reviewed change. |
| `project.list_recent` | List recently changed project artifacts. |
| `project.search` | Search documents, notes, drafts and transcripts in this project. |
| `project.get_activity` | Read recent native and assistant project activity. |
| `canvas.list` | List active Canvas boards in this project. |
| `canvas.get` | Read one Canvas snapshot with its current revision and resolved project resources. |
| `canvas.search` | Search standalone Canvas text, typed fields and linked metadata in this project. |
| `canvas.analyze_graph` | Analyze one Canvas graph for components, orphans, roots, leaves and cycles without changing it. |
| `canvas.propose_patch` | Stage typed add, update and delete operations for one Canvas revision. valueJson must contain one complete JSON Canvas node or edge for add/update and be an empty string for delete. Canonical Canvas stays unchanged until Apply. |
| `canvas.promote_cards` | Stage atomic promotion of selected Canvas text cards into project documents. Canonical Canvas and project tree stay unchanged until Apply. |
| `tree.list` | List the complete project tree without document bodies. |
| `tree.get_node` | Read node metadata and revision. |
| `tree.create_node` | Stage creation of a project node. |
| `tree.create_nodes_batch` | Stage 1 to 50 project nodes together with their complete initial content. Use this for every new content-bearing node and for every multi-node request; never call tree.create_node and then document.append for an unapplied node. All items share one canonical parent and are inserted in array order after positionAfterId. Each item becomes an independent tree.create_node review operation. |
| `tree.rename_node` | Stage a node title change. |
| `tree.move_node` | Stage moving a node in the project tree. |
| `tree.set_status` | Stage a node workflow status change. |
| `tree.duplicate_node` | Stage duplication of a node. |
| `tree.prepare_archive` | Prepare a high-risk archive operation for review. |
| `document.get` | Read the current structured document, canonical blocks and plain text. Blocks preserve native code, lists, separators, tables, project images, Mermaid diagrams and info/idea/warning/todo callouts without flattening them into paragraphs. |
| `document.find_exact` | Locate an exact literal substring and return verified block-local Unicode anchors for document.replace_range. This is deterministic indexing, not semantic inference. Use it instead of counting offsets. |
| `document.find_exact_batch` | Locate several exact literal substrings in one document revision and return verified block-local Unicode anchors for one atomic document.replace_range call. Use this instead of one document.find_exact call per paragraph. |
| `document.get_range` | Read an exact text range from a document revision. |
| `document.list_references` | List source references attached to document blocks. |
| `document.insert` | Stage structured content insertion at an exact position. Content supports canonical prose, native code, tables, project images, Mermaid diagrams and info/idea/warning/todo callouts. |
| `document.replace_range` | Stage one atomic replacement of one or more exact block-local text ranges. Every anchor must come from document.find_exact or document.find_exact_batch; never calculate offsets manually. Combine every edit for the same document in this call so review, apply and undo stay atomic. |
| `document.append` | Stage appending canonical prose, native code, tables, project images, Mermaid diagrams or info/idea/warning/todo callouts to a document. |
| `document.append_chunk` | Stage one bounded long-form chunk in an existing document. Use this repeatedly instead of document.append whenever the requested new prose may exceed 1500 words. Chunks are accumulated in sequence into one reviewable document operation; canonical state remains unchanged until Apply. Keep each chunk between roughly 600 and 1200 words, copy the author's approximate total into targetWords, start sequence at 1, and set finalChunk only when the accumulated draft reaches the target. |
| `document.set_title` | Stage a document title change. |
| `document.apply_marks` | Stage formatting marks on an exact range. |
| `document.create_draft` | Stage a new draft document with canonical prose, native code, tables, project images, Mermaid diagrams and info/idea/warning/todo callouts. |
| `note.create` | Stage a project note. |
| `note.update` | Stage a note update. |
| `annotation.list` | List inline manuscript annotations across the whole project. Use this before a project-wide note cleanup; results include immutable annotation revisions and exact server-owned anchors. |
| `annotation.get` | Read one inline annotation together with its versioned Assistant reanchoring history. |
| `annotation.propose_fixes` | Stage exact manuscript fixes for 1 to 50 inline notes. Supply annotation IDs and revisions from annotation.list/get; the server derives every text range from the current anchor, groups edits atomically by document, and reanchors notes itself. Never supply offsets. resolve=true marks the note resolved only after Apply; notes are never deleted. |
| `document.prepare_transfer` | Prepare copying content between any two content-bearing project nodes. |
| `draft.prepare_transfer` | Prepare transferring a draft into a manuscript node. |
| `document.prepare_replace_all` | Prepare a high-risk project-wide replacement. |
| `dictionary.list` | List active and inactive project dictionary terms. |
| `dictionary.find` | Find dictionary terms by canonical name or variant. |
| `dictionary.add` | Stage one new dictionary term. If a canonical name or variant already exists, returns a typed conflict and does not stage a duplicate. Use dictionary.add_batch whenever the request contains two or more new terms. |
| `dictionary.add_batch` | Stage 2 to 100 new dictionary terms in one call. Prefer this tool for every multi-term request so large author instructions stay within the safe tool-call limit. Each staged item remains an independent reviewable operation; duplicates return typed per-item conflicts. |
| `dictionary.update` | Stage an update to a dictionary term. |
| `dictionary.set_active` | Stage activating or deactivating a dictionary term. |
| `mentions.find` | Find mentions of a term across project text. |
| `assistant_guide.get` | Read the pinned assistant writing guide revision. |
| `assistant_guide.propose_create` | Stage creation of the project assistant guide. |
| `assistant_guide.propose_update` | Stage a new guide revision. |
| `project_memory.list` | List project memory records. |
| `project_memory.search` | Search project memory records. |
| `project_memory.get` | Read one project memory record. |
| `project_memory.add` | Stage one project memory record. If the same normalized statement already exists, returns a typed conflict and does not stage a duplicate. Use project_memory.add_batch whenever the request contains two or more new records. |
| `project_memory.add_batch` | Stage 2 to 100 project memory records in one call. Prefer this tool for every multi-record request so large author instructions stay within the safe tool-call limit. Each staged item remains an independent reviewable operation; duplicates return typed per-item conflicts. |
| `project_memory.update` | Stage an update to a project memory record. |
| `project_memory.set_active` | Stage activating or deactivating memory. |
| `project_memory.set_always_relevant` | Stage always-relevant memory status. |
| `version.list` | List document named versions and revisions. |
| `version.compare` | Compare two document revisions. |
| `version.create_snapshot` | Stage creation of a named document snapshot. |
| `version.prepare_restore` | Prepare a high-risk version restore. |
| `dictation.list_sessions` | List dictation sessions linked to this project. |
| `transcript.get` | Read transcript and draft metadata. |
| `transcript.get_uncertain` | Read low-confidence transcript ranges. |
| `audio.get_source_range` | Get authorized audio source range metadata. |
| `transcript.prepare_transfer` | Prepare transferring a transcript draft. |
| `transcript.prepare_retranscribe` | Prepare explicit transcript retranscription. |
| `export.preview` | Preview a project export without creating it. |
| `export.prepare` | Prepare creating an export artifact. |
| `recovery.get_status` | Read recovery and unsynchronized-data status. |
| `settings.get_writer_preferences` | Read the run-pinned non-secret writer preferences snapshot. |
| `assistant.get_capabilities` | Read assistant/provider/tool capabilities. |
| `tree.archive` | Stage a high-risk node archive operation. Canonical state changes only after change_set.apply and supports Undo. |
| `version.restore` | Stage restoring a named version as a new document revision. Requires the exact current revision and supports Undo. |
| `workspace.list_projects` | List projects allowed by this access grant. Trash is never exposed. |
| `workspace.open_project` | Open one grant-authorized project in this MCP session. The server binds all subsequent project tools to it. |
| `workspace.get_context` | Read the server-bound project, target, turn and grant context for this session. |
| `workspace.close_project` | Close the active project after its turn has been finished or discarded. |
| `assistant.finish_turn` | Finish the remote turn and atomically convert its durable draft into a normal Assistant change set. |
| `assistant.discard_turn` | Discard only the current unapplied durable draft and close the active turn. |
| `change_set.list` | List change sets created by this grant. An empty projectId lists all authorized projects. |
| `change_set.get` | Get one change set created by this grant. |
| `change_set.review` | Read exact operation groups and snapshots, then receive a one-time 10-minute review token bound to the current content and state. |
| `change_set.apply` | Atomically consume a review token and apply selected operation groups with stale and dependency checks. |
| `change_set.reject` | Consume a review token and reject a staged change set. |
| `change_set.undo` | Consume a fresh review token for an applied change set and undo it. |
| `transcript.retranscribe` | Consume the one-time token from transcript.prepare_retranscribe and queue retranscription idempotently. |
| `export.create` | Consume the one-time token from export.prepare and create an export job idempotently. |
| `export.get` | Poll an export job created by this grant. A ready job exposes its authorized MCP resource URI. |

See workflow AC-56–AC-176 and the specialized annotation/Canvas specifications.
