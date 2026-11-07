# Dictionary links and manuscript annotations

Annotations connect a precise manuscript quote to a real dictionary entry or an
editorial note. They are author metadata, separate from Tiptap JSON, reader text,
document schema versions and manual editing Undo/Redo.

## Data and anchoring

`document_annotations` records project/node, `dictionary_link` or `note`, optional
term ID, lifecycle (`open`, `resolved`, legacy `needs_attention`), independent
anchor state (`attached`/`detached`), note/target snapshot, revisions and timestamps.
An exact anchor contains block ID, Unicode code-point offsets, literal text and
prefix/suffix context. Creation/reattachment validates it against a base revision.
`document_annotation_history` keeps immutable before/after changes and provenance;
run-history deletion clears execution references without losing the audit content.

## Invariants

1. Decorations never enter canonical document JSON or mutate past revisions.
2. Manual revision and mapped anchors commit in one transaction.
3. An omitted `annotationAnchors` field means a legacy client: retain metadata and
   detach uncertain anchors. An explicit empty array means no anchors were mapped.
4. Deleted, partially overlapping or unprovable quotes retain their original text
   and detach; never guess a similar passage. Lifecycle remains independent.
5. Conflict copies and duplicates retain annotations. Deleting a dictionary term
   preserves its readable snapshot and clears the live reference.
6. Exact whole-range replacement updates anchor text/context deterministically;
   edits before the range shift it, and edits after it leave it unchanged.
7. Assistant Apply updates text, anchors, lifecycle and history atomically. Undo
   appends compensating document revisions and restores annotation snapshots.

## Interaction and API

A nonempty single-block selection opens context actions with right-click,
Shift+F10/context-menu key or touch long press. Selection alone adds no permanent
toolbar. Actions link an existing dictionary term, add a note or attach the quote
to the assistant. Multi-block ambiguity is explained rather than silently anchored.
Desktop uses a side panel; mobile uses a sheet with equivalent actions.

Project-wide annotation search filters type, lifecycle and document. Deep links
use `annotationId` and open the exact quote/panel. Resolved notes can reopen.

Routes include project/node annotation lists, node annotation creation,
annotation patch/delete and node revision saves with optional anchor mapping.
Every endpoint validates ownership through the project and omits content from logs.

Assistant `annotation.list`/`annotation.get` are scoped reads.
`annotation.propose_fixes` accepts at most 50 annotation IDs/revisions, expected
text, replacements and resolution choices. The model supplies no block offsets;
the server validates and groups fixes by document. Review displays note and exact
before/after with independent resolve toggles. Stale note/document versions fail
before any canonical change.

## Compatibility and acceptance

Additive migrations 013/015 do not rewrite document text/history. Annotation
transfers began in native archive v2 and remain supported through current v8 with
atomic node/term remapping. v1 imports without annotations. Invalid references,
anchors or future formats fail before partial creation.

| IDs | Required checks |
| --- | --- |
| AC-MA-01/02 | Single-block exact context actions; clear rejection of cross-block anchors |
| AC-MA-03/04 | Dictionary link and editable/resolvable note interaction |
| AC-MA-05/06 | Noncanonical decorations and transactional anchor mapping |
| AC-MA-07/08 | Deterministic detach and legacy-client preservation |
| AC-MA-09/10 | Duplicate/conflict retention and deleted-term snapshots |
| AC-MA-11/12/13 | Project search, exact deep link and reopening resolved notes |
| AC-MA-14 | Owner isolation |
| AC-MA-15/16/17 | Legacy/native transfer remapping and pre-commit rejection |
| AC-MA-18 | Critical coverage target, database integration and browser tests |
| AC-MA-19/20 | Scoped reads, no model offsets, up to 50 staged grouped fixes |
| AC-MA-21/22 | Atomic text/lifecycle/history and deterministic overlap detach |
| AC-MA-23 | Undo snapshots and audit retention after run-history deletion |

Workflow AC-143–AC-167 provide the corresponding end-to-end criteria.
