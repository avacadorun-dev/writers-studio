# Projects and navigation

## Purpose

Organize source material and manuscripts into independently managed projects.

## Requirements

### Requirement: Library and project creation

A signed-in author SHALL have access to project creation, existing projects, and local recovery. A new project SHALL store its selected type, title, language and initial structure as one unit.

#### Scenario: Create and reopen a project

- **WHEN** the author creates a valid project and reloads the library
- **THEN** the project and its initial structure are available

### Requirement: Document tree

The workspace SHALL enforce the active project structure when authors create, rename, reorder, move, or trash nodes. Supported node types and nesting SHALL come from the project structure.

#### Scenario: Move or trash a document

- **WHEN** the author completes a valid reversible action and then chooses Undo
- **THEN** the original document identity, content and history are restored in the previous location

#### Scenario: Reject incompatible nesting

- **WHEN** a move would violate the project structure
- **THEN** the tree remains in its previous state

### Requirement: Lifecycle and ownership

Projects SHALL have distinct active, archived and trashed states. Permanent deletion SHALL require exact confirmation. Account and project reads and writes SHALL be restricted to authorized owners.

#### Scenario: Restore from trash

- **WHEN** an owner restores a trashed project
- **THEN** the project becomes accessible with its tree and revision history

#### Scenario: Attempt to confirm with a different name

- **WHEN** an owner supplies an incorrect confirmation for permanent deletion
- **THEN** the action is rejected and data remains available

### Requirement: Return actions

Each secondary screen SHALL provide an accessible return control with a target of at least 44 by 44 CSS pixels and a safe destination when the screen was opened directly.

#### Scenario: Open a document utility directly

- **WHEN** a signed-in user opens a document version or review URL directly
- **THEN** the return action goes to the relevant document or project
