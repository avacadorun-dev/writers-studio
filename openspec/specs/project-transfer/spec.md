# Project transfer and exports

## Purpose

Export reader documents and transfer projects using native archives or portable Markdown trees.

## Requirements

### Requirement: Reader exports

Reader exports SHALL use a stable snapshot of the selected manuscript and its recursive order. A failed job SHALL report failure. Markdown SHALL preserve supported rich block and math syntax.

#### Scenario: Export while editing

- **WHEN** an author requests a reader export and then edits the manuscript
- **THEN** the export uses the revisions selected when the export was requested

### Requirement: Native archive compatibility

The native `.wstudio` export SHALL write ZIP format v8 (`application/vnd.writers-studio.project+zip`) with a `project.json` manifest and the `writers-studio-project` format identifier. Import SHALL accept v1 through v8 and create an independent copy with remapped references.

#### Scenario: Import an earlier archive

- **WHEN** the author imports v1
- **THEN** the project and supported history are copied without requiring newer annotations, Canvas or media fields

#### Scenario: Reject a damaged archive

- **WHEN** the archive has invalid paths, checksums, references, anchors or a future version
- **THEN** no partial project becomes visible

### Requirement: Native archive scope and bounds

Native archives SHALL carry canonical project data, supported history and available required media, with a 512 MiB upload limit and a 64 MiB manifest limit. Account credentials, provider connections, runtime sessions and assistant execution history SHALL stay outside the archive.

#### Scenario: Export with missing required audio

- **WHEN** declared source audio cannot be read or verified
- **THEN** export fails instead of returning an incomplete successful archive

#### Scenario: Restore unfinished capture from an archive

- **WHEN** an archive contains unfinished dictation state
- **THEN** the importer normalizes it without starting a new recognition job

### Requirement: Markdown tree import

Markdown import SHALL read a directory or ZIP as a new project, preserve Markdown semantics and source frontmatter, and resolve internal links and supported media where possible. Ambiguous references and skipped attachments SHALL be reported.

#### Scenario: Import a note with children

- **WHEN** the source has both `Note.md` and a matching `Note/` directory
- **THEN** the imported note contains the directory children

#### Scenario: Reject an invalid Markdown tree

- **WHEN** the upload includes traversal, symlinks, duplicate normalized paths or exceeds import limits
- **THEN** no partial project becomes visible

### Requirement: Markdown tree export

Markdown tree export SHALL produce a portable ZIP of active nodes with compatible names, relative internal links and project media. The exported tree SHALL include explicit empty directories and omit the manuscript publication filter.

#### Scenario: Export duplicate titles

- **WHEN** multiple entries collide by case-insensitive filename
- **THEN** each is written under a distinct portable name and relative links resolve to the exported targets
