# Markdown / Obsidian tree migration

Writers Studio supports a semantic two-way migration between a project and a
portable Markdown directory tree.

## Import

`POST /api/projects/import/markdown-tree` accepts `multipart/form-data` with
`title`, `language`, and `source`. A ZIP source uses the `archive` part. A local
directory uses a JSON `manifest` (`rootName`, path/kind/size entries) and
`entry.<id>` file parts. The browser directory-picker path preserves explicit
empty directories; the `webkitdirectory` compatibility path cannot.

The importer creates a `free` project with the `markdown_vault` structure.
Folders become structural `folder` nodes, `.md` files become content-bearing
`document` nodes, and a `Note.md` + `Note/` pair becomes one document with
children. Raw YAML frontmatter is stored separately from Tiptap JSON. Internal
Markdown links, wikilinks, headings/block fragments, and local PNG/JPEG/GIF
embeds are resolved only after the complete path and ID index has been built.
Unresolved or ambiguous references remain visible and are reported.

The canonical in-app link form is
`/projects/<project-id>/studio/<node-id>#fragment`. Arbitrary HTML is not
executed. `.obsidian`, `.trash`, `.git`, `.DS_Store`, and `Thumbs.db` are
ignored; unsupported attachments are skipped with a warning.

The operation is bounded and atomic at the project database level. ZIP path
traversal, symlinks, normalized duplicates, excessive expansion, depth, path
length, node count, media count, and individual file limits are rejected before
the project is created. Uploaded media is compensatingly deleted if the database
transaction fails.

## Export

`POST /api/projects/{id}/exports` with `{"format":"markdown_tree"}` creates an
asynchronous ZIP export of every active node. Publishing-profile selection and
the manual publication queue do not apply.

Structural nodes become directories. A content node without children becomes
`Title.md`; a content node with children becomes both `Title.md` and `Title/`.
Empty directories are emitted explicitly. Raw frontmatter precedes semantic
Markdown, canonical links and project-image URLs become relative portable
paths, imported image paths are retained, and newly uploaded images use
`_attachments/`. Trashed nodes are excluded.

Names are sanitized for Windows/macOS/Obsidian, including reserved device names,
UTF-8 segment bounds, and case-insensitive collision suffixes. The worker writes
the ZIP to a temporary file, reads one image at a time, and streams the result to
object storage.

## Compatibility

Document schema v3 adds headings 4–6 while keeping v1/v2 readable. `.wstudio`
format v5 carries `sourceFrontmatter` and media `sourcePath`, and remaps canonical
internal document links when importing a project copy. `.wstudio` v1–v4 remains
accepted; complete history, versions, annotations, audio, and settings are still
transferred only by `.wstudio`, not Markdown.
