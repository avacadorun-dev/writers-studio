# Dictionary and manuscript annotations

## Purpose

Keep exact manuscript quotations associated with author notes and terminology.

## Requirements

### Requirement: Dictionary scope

The project dictionary SHALL store canonical terms, variants, types, languages and optional pronunciation. Changes SHALL inform future recognition without silently rewriting existing text.

#### Scenario: Change a term

- **WHEN** the owner edits a canonical spelling
- **THEN** subsequent eligible recognition uses the updated hints and prior manuscript revisions retain their text

### Requirement: Exact contextual anchoring

A note or dictionary link SHALL reference a verified, nonempty quote in one block using Unicode code-point offsets. The author SHALL be able to open context actions with a pointer, keyboard or touch.

#### Scenario: Create a note on a single-block selection

- **WHEN** the author invokes a context action on an exact quote
- **THEN** the author can add a note or link an existing term

#### Scenario: Select across blocks

- **WHEN** a selection crosses block boundaries
- **THEN** the workspace explains why an exact annotation cannot be created

### Requirement: Anchor lifecycle and preservation

Annotation mapping SHALL save with its document revision. Unprovable anchors SHALL detach and retain their quote and independent resolution status. A deleted dictionary term SHALL leave a readable snapshot.

#### Scenario: Edit across an anchor

- **WHEN** an edit partially overlaps an annotated quote
- **THEN** the annotation becomes detached without attaching to a similar passage

#### Scenario: Accept a legacy save

- **WHEN** a client saves a revision without anchor mappings
- **THEN** annotations survive and uncertain anchors detach

### Requirement: Search and history

Authors SHALL be able to find project annotations by document, type and lifecycle, reopen resolved notes, and deep-link to attached quotes. Duplicates and conflict copies SHALL retain applicable annotations.

#### Scenario: Follow a search result

- **WHEN** an author opens a result with an attached anchor
- **THEN** the matching manuscript and exact quoted range open

#### Scenario: Remove assistant history

- **WHEN** the owner deletes a run associated with an annotation edit
- **THEN** the annotation and its readable change history remain available
