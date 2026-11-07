# Rich document editor

Status: implemented through the frozen 0.7.0 edition. Current document schema: v4.

The same Tiptap/ProseMirror editor serves every content-bearing node, including
novels, screenplays, notes, free structures and game-design projects.

## Supported content and presentation

- Tables with headers and row/column insertion/deletion.
- Inline code and fenced blocks with a language identifier.
- PNG/JPEG/GIF images up to 12 MiB, with alt text and captions.
- Image drop at the manuscript position after durable project-media upload.
- Mermaid diagrams, callouts (`info`, `idea`, `warning`, `todo`), inline/block LaTeX.
- Existing headings, lists, quotes, links, scene breaks and text marks.

Mermaid uses strict security mode; KaTeX uses `trust: false` without HTML, external
URLs or user-defined commands. Invalid formulas show a safe source fallback.
Paste removes scripts, iframes, SVG, forms and event handlers and validates URLs.
Toolbar modes are automatic, expanded and compact. Overflow avoids horizontal
scrolling; typography settings separately control line, paragraph and list spacing.

## Compatibility

Migration 019 adds project media without rewriting document history. Missing
schema version means v1. Plain editing does not eagerly upgrade it: rich blocks
and inline code require v2, heading levels 4–6 require v3, mathematics requires v4.
The backend validates v1–v4 and derives revision schema from the JSON itself.

Search, replacement, duplication, transfer, named versions and Undo preserve schema
and context. Copied rich blocks receive independent block IDs. Formula-bearing
revisions need 0.7.0 or newer; old revisions remain intact.

Native `.wstudio` archive v8 carries project media, document/history, guidance,
memory and Canvas schemas while accepting supported older versions. Future
formats fail before partial import. Archive and document schema versions differ.

## Media and export

Upload images to private project storage before writing `assetId`, authorized
same-origin `src`, dimensions, alt text and caption into the document. Reads check
ownership and integrity; account/project purge includes media. Full backups include
metadata and objects.

Markdown preserves GFM tables, fences, Mermaid, callouts, images and `$...$` /
`$$...$$` math. Reader formats use safe textual fallbacks where required. Native
archives are the complete portable copy, including binary resources.
