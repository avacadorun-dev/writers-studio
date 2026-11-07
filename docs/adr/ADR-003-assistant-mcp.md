# ADR-003: Local project MCP for the assistant

Status: accepted and implemented.

## Context

The assistant must use real project operations without receiving database,
object-storage or account credentials. Its work must share domain validation,
revision checks, staging and Undo with ordinary application operations.

## Decision

Run a server-side orchestrator with an in-process project MCP catalog. Adapt tool
schemas/results to the provider's Responses or Chat Completions dialect. Provider
support for hosted remote MCP is optional and is not required by the baseline.

Start each turn with exactly one semantic `assistant.begin_turn` declaration:
`answer`, `change` or `clarify`. Do not route intent with verb/keyword regexes.
Use current instructions and native recent change-set context rather than treating
historical chat prose as authoritative project mutations.

An execution context contains server-owned owner/project/run/target and read
scope. Tools cannot substitute another owner or project. All domain writes stage
operations in one change set. Apply validates base revisions, exact expected text,
protected ranges and group dependencies atomically. Undo creates compensating
revisions from stored before-snapshots.

## Data and interaction

Runs, turns, tool events and change sets are durable. Voice and chat share target,
provider, context and history. Guide revisions are pinned when a turn is confirmed.
Dictionary entries and typed project memory are separate stores with revisions.
Project content remains untrusted input; the optional reserved `ASSISTANT.md` guide
is lower priority than system/security policy and never expands permissions.

Read actions stay within selected context. Creation and edits require staged
review. Sensitive exports/restores/re-transcription use typed native handoffs;
irreversible deletion and credential/settings changes are unavailable to models.

## Provider and secret boundaries

Probe text generation, strict tool arguments, tool-result continuation and multiple
sequential calls before selecting a provider. Store custom provider credentials
encrypted; return fingerprints only. Pin provider/model through a run. System
credentials stay in environment configuration. Never put prompts, manuscript,
arguments/results or credentials in technical logs.

## Alternatives and consequences

Direct CRUD calls from the model and auto-committing each tool call were rejected:
they bypass meaningful review and make partial failure hard to reconcile.
Sending the whole project in every prompt was rejected in favor of bounded,
scoped retrieval. Requiring provider-hosted MCP would create unnecessary coupling.

The local catalog adds schema/orchestration work but keeps domain invariants
consistent. Remote MCP is a separately authenticated gateway described in
[ADR-008](ADR-008-remote-mcp-gateway.md). Detailed behavior is in the
[assistant specification](../specification/ASSISTANT_MODE.md).
