# ADR-002: Tiptap/ProseMirror manuscript editor

Status: accepted and implemented.

## Context and decision

The manuscript needs ordinary writing features, structured content, reliable
transactions, selection mapping, source references and exact assistant changes.
Use Tiptap 3 and ProseMirror rather than a textarea, opaque HTML editor or a custom
editing engine.

Canonical content is validated Tiptap JSON, not arbitrary HTML. Each content-bearing
project node has its own document revisions. Stable block IDs support provenance,
annotations and exact operations. Plain text and Markdown are derived views.

## Schema and compatibility

Schema v1 covers paragraphs, headings, lists, quotations, scene breaks and ordinary
marks. v2 adds rich blocks and inline code; v3 adds heading levels 4–6; v4 adds
inline/block mathematics. Missing `schemaVersion` means v1. Opening or editing plain
text does not eagerly upgrade old documents. The server validates supported
versions and derives revision schema from content.

Paste uses an allowlist. Rich images are uploaded durably before insertion;
canonical references use project media IDs and authorized same-origin URLs.
Mermaid runs in strict mode and KaTeX uses `trust: false`.

## Persistence and integration

Autosave appends revisions without resetting mounted editor content, selection,
scroll or undo history on acknowledgement. Local snapshots provide emergency
recovery; base revision conflicts preserve alternate work. Named versions remain
separate from autosave history. Restore appends a revision.

Transcript transfer is one explicit editor operation at a selected destination,
with provenance and Undo. Late transcription cannot apply a hidden transaction.
Assistant previews and annotation decorations are transient until an authorized
canonical mutation. Project annotations are not embedded in document JSON.

## Consequences and verification

The engine provides schema-aware transactions and mapping at the cost of explicit
extension design and validation. Test rich content round-trips, paste sanitization,
exact Unicode ranges, block-ID remapping, selection preservation, source playback,
conflict recovery and export fallbacks. See
[rich-document specification](../specification/RICH_DOCUMENT_EDITOR.md).
