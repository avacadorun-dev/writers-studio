# Manuscript editing and history

## Purpose

Edit, find and restore long-form content while protecting acknowledged work and concurrent drafts.

## Requirements

### Requirement: Rich document editing

Content-bearing nodes SHALL support headings, lists, quotes, links, scene breaks, marks, tables, inline and fenced code, project images with alternative text and captions, Mermaid, callouts and LaTeX.

#### Scenario: Save and reopen a formatted draft

- **WHEN** an author adds text and rich blocks and the revision is acknowledged
- **THEN** reopening the draft restores content and formatting

#### Scenario: Paste unsafe markup

- **WHEN** pasted content contains executable HTML or unsafe URLs
- **THEN** those elements cannot execute in the manuscript

#### Scenario: Drop an image

- **WHEN** the author drops an accepted PNG, JPEG or GIF of at most 12 MiB
- **THEN** the image is linked at the drop position after durable upload and remains private to the project

### Requirement: Revisions and conflicts

Autosave SHALL preserve the mounted editor, selection and manual undo history. A stale write SHALL preserve the competing work. Named versions SHALL be immutable snapshots, and restoration SHALL append a revision.

#### Scenario: Acknowledge a draft update

- **WHEN** an autosave response arrives while the author continues working
- **THEN** the save status updates without resetting caret or scroll

#### Scenario: Save concurrent edits

- **WHEN** a write uses a stale revision
- **THEN** the earlier acknowledged content and alternate work both remain recoverable

#### Scenario: Compare and restore a version

- **WHEN** an author compares a named version and then restores it
- **THEN** comparison leaves the manuscript unchanged
- **AND** restore appends a revision and retains newer history

### Requirement: Compatible rich document format

The server SHALL read document schemas v1 through v4. Ordinary edits SHALL retain the applicable schema. Rich blocks and inline code require v2, headings 4 through 6 require v3, and mathematics requires v4.

#### Scenario: Edit an older revision

- **WHEN** an author adds plain text to a v1 manuscript and later adds a formula
- **THEN** the first edit remains readable as v1
- **AND** the formula revision uses v4 and earlier revisions remain intact

### Requirement: Search and replacement

Search SHALL navigate to the exact manuscript match. Replacement SHALL require a current preview and SHALL apply only its confirmed scope.

#### Scenario: Reject a stale preview

- **WHEN** affected documents change after the author previews a replacement
- **THEN** the author is asked to preview again before any replacement is applied

### Requirement: Responsive presentation

The workspace SHALL provide visible focus, accessible control names, textual statuses, and keyboard editing. Formatting controls SHALL support compact and expanded presentation.

#### Scenario: Edit in a narrow workspace

- **WHEN** available editing width decreases
- **THEN** formatting actions remain reachable through compact controls and overflow without horizontal toolbar scrolling
