# Canvas boards

## Purpose

Arrange visual notes and project references on boards with portable, recoverable snapshots.

## Requirements

### Requirement: Board organization

A project SHALL support independent named boards with creation, duplication, trash, restore and revisions. Boards SHALL retain their cards, edges and compatible extension metadata across reload.

#### Scenario: Reopen a populated board

- **WHEN** the author reloads a saved board with nested boards, notes, edges or resource links
- **THEN** the saved arrangement and links are available

### Requirement: Visual and typed editing

Desktop Canvas SHALL support spatial editing, drawing, template content, media, nested boards, and typed cards and relationships. Alternative projections SHALL leave canonical geometry unchanged. Mobile SHALL support view and navigation.

#### Scenario: Copy selected cards

- **WHEN** the author duplicates a selection
- **THEN** copied cards receive new identifiers and retain only copied edges with both endpoints in the selection

#### Scenario: Move and remove a group

- **WHEN** an author moves a spatial group and later removes it
- **THEN** enclosed cards move with the group and survive its removal

#### Scenario: Open a board on mobile

- **WHEN** an author follows a linked document on a mobile viewport
- **THEN** the link opens and the layout remains intact

### Requirement: Validation and recovery

A saved board SHALL validate project-owned references, geometry, unique IDs and edge endpoints. Snapshots SHALL support up to 2,000 nodes and 4,000 edges, with at most 100,000 Unicode code points per text card.

#### Scenario: Recover from a stale save

- **WHEN** the server detects a conflicting board revision
- **THEN** the workspace preserves an alternate board instead of overwriting the existing revision

#### Scenario: Reconnect with pending work

- **WHEN** an author returns online after editing locally
- **THEN** recoverable unsynced work is retried

### Requirement: Board transfer

Canvas SHALL support portable JSON Canvas files. Native project archives SHALL preserve supported board history, typed schemas and references.

#### Scenario: Import a project containing linked boards

- **WHEN** a native archive is imported as a new project
- **THEN** board, document and media references resolve within the imported project
