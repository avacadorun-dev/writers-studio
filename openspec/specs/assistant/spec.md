# Assistant and project agents

## Purpose

Let authors delegate bounded project work and retain control over all proposed manuscript changes.

## Requirements

### Requirement: Explicit input and context

The assistant SHALL show its target, read scope, model and active guide before execution. Voice and chat SHALL share ordered conversation history, and each instruction SHALL require explicit submission.

#### Scenario: Switch from voice to chat

- **WHEN** the author switches input mode while a voice draft is present
- **THEN** listening pauses and the draft and scoped context remain available without an automatic model request

#### Scenario: Identify an ambiguous target

- **WHEN** an instruction cannot be resolved to one exact target
- **THEN** the assistant asks for clarification before staging changes

### Requirement: Tool authorization and semantic mode

A turn SHALL declare exactly one mode through `assistant.begin_turn` before using project tools. Answer and clarification modes SHALL have read-only scope. Tool execution SHALL enforce the authorized project and available actions.

#### Scenario: Ignore untrusted instructions in project text

- **WHEN** source material asks the assistant to change credentials or reach a different project
- **THEN** the available tools and data scope remain restricted

#### Scenario: Hand off a sensitive action

- **WHEN** an assistant operation requires archive, export or restoration confirmation
- **THEN** the user is offered the applicable native confirmation flow

### Requirement: Staged mutations and Undo

Assistant edits SHALL stay staged until the author applies a dependency-consistent selection. Apply SHALL verify revisions, expected text, anchors and protected ranges, then commit the selection atomically. Undo SHALL create compensating revisions.

#### Scenario: Review an exact proposal

- **WHEN** the author opens review
- **THEN** the instruction, affected objects, operation groups, and exact before and after are visible

#### Scenario: Reject a stale change set

- **WHEN** a document changes after a proposal was prepared
- **THEN** Apply is blocked without changing any affected object

#### Scenario: Undo grouped changes

- **WHEN** an author undoes an applied set that updated text and structured project data
- **THEN** the previous states are restored through compensating history

### Requirement: Annotation fixes

The assistant SHALL accept up to 50 versioned annotation fixes per proposal. It SHALL use existing verified anchors and SHALL apply text, anchor, resolution and annotation history changes as one unit.

#### Scenario: Choose which notes to resolve

- **WHEN** the author reviews proposed annotation fixes
- **THEN** each fix shows its note and exact text changes with an independent resolution choice

#### Scenario: Apply against a changed annotation

- **WHEN** a referenced annotation changes before Apply
- **THEN** the set leaves both annotations and manuscript unchanged

### Requirement: Conversation isolation and recovery

Conversations SHALL retain independent drafts, targets, attachments and review state. Switching or hiding an assistant panel SHALL preserve the mounted manuscript and avoid cancelling work or submitting drafts. Stop SHALL prevent further calls.

#### Scenario: Work in different conversations

- **WHEN** the author switches away from a working conversation
- **THEN** that run can continue and the other conversation retains its own draft and target

#### Scenario: Resume after an interruption

- **WHEN** a reload or server interruption occurs during execution
- **THEN** durable activity and staged work remain available for review or retry without replaying applied changes

### Requirement: Draft previews and partial work

Unapplied previews SHALL identify staged changes separately from the manuscript and its undo history. Multi-part output SHALL retain an explicit partial status until complete. Failures SHALL keep available instructions and staged work.

#### Scenario: Edit an overlapping range manually

- **WHEN** the author edits a range shown in a preview
- **THEN** the proposal becomes stale while the manual edit remains

#### Scenario: Time out after staging changes

- **WHEN** the provider fails after some operations were staged
- **THEN** the author can review the partial set without any automatic application

### Requirement: Provider connections

Custom assistant connections SHALL require a successful capability probe before use. Credentials SHALL be masked on reads and the selected provider and model SHALL remain pinned to a run.

#### Scenario: Probe an incomplete connection

- **WHEN** a reachable provider cannot complete the required sequence of tool calls
- **THEN** the connection is unavailable for assistant selection

#### Scenario: Remove a provider

- **WHEN** the owner deletes a custom connection
- **THEN** new runs cannot use it and existing history stays readable

### Requirement: Guidance and project memory

A project SHALL support one optional active guide and versioned project memory. A confirmed turn SHALL use its pinned guide revision. Assistant-authored guide, dictionary and memory changes SHALL follow staged review.

#### Scenario: Restore guidance during a run

- **WHEN** the owner restores an earlier guide while a turn is executing
- **THEN** a new guide revision becomes active without changing the running turn

#### Scenario: Capture a conflicting fact

- **WHEN** the assistant proposes a memory change that overlaps an existing fact
- **THEN** review requires an explicit merge or update decision before changing committed memory

### Requirement: Authorized remote project agents

When enabled, the `/mcp` endpoint SHALL accept scoped OAuth or manual bearer grants. Each request SHALL revalidate the grant. Archived projects SHALL be read-only, and trashed projects SHALL be inaccessible through the gateway.

#### Scenario: Revoke an active grant

- **GIVEN** an external agent has opened an MCP session and an authorized project
- **WHEN** the owner revokes its grant
- **THEN** the following request fails even if the MCP session was previously active

#### Scenario: Attempt to change the active project through a tool argument

- **WHEN** a tool argument names a project outside the authorized workspace
- **THEN** the request cannot reach that project's content or bypass its permissions

### Requirement: Remote proposal review and actions

The gateway SHALL retain staged work after an interrupted turn. A remote request to Apply, Reject, or Undo SHALL use a current, single-use review token bound to the grant and the proposal being reviewed.

#### Scenario: Resume a partial remote proposal

- **WHEN** an external session ends before finishing a turn with staged changes
- **THEN** the staged work is available as a partial proposal for review
- **AND** its changes remain unapplied

#### Scenario: Reject a reused review token

- **WHEN** a client retries a review action with a consumed token
- **THEN** the action is rejected without a duplicate mutation
