# ADR-008: Remote MCP gateway with project-scoped grants

## Status

Accepted and implemented for LAN/VPN deployments. The feature is disabled by
default with `MCP_ENABLED=false`.

## Context

The built-in Assistant already uses a in-process MCP tool catalog and durable
Assistant change sets. A local external agent needs the same project abilities,
including review, Apply, Reject and Undo, without opening the Writer's Studio UI.
The external client must never choose an owner or smuggle a different project ID
through ordinary tool arguments.

## Decision

- `/mcp` is a stateful MCP Streamable HTTP endpoint with a 30-minute idle timeout.
- The remote catalog is a strict superset of the internal catalog.
- OAuth Authorization Code + mandatory S256 PKCE is validated at the protocol
  boundary by ORY Fosite. Protected-resource/authorization-server discovery,
  Client ID Metadata Documents and Dynamic Client Registration are supported;
  durable codes and opaque access/refresh tokens remain hash-only in PostgreSQL.
  One-time-displayed manual bearer tokens use the same project allowlist and
  explicit scopes.
- Every HTTP request revalidates the token or grant. Revocation therefore affects
  existing MCP sessions immediately on their next request.
- Every MCP session has its own server-owned active project, target, run and turn.
  Project tools cannot receive or override owner scope.
- Remote staged operations are mirrored to `assistant_turn_drafts` before a tool
  result is returned. Explicit finish moves them to an ordinary Assistant change
  set; DELETE, idle timeout and startup recovery publish non-empty drafts as partial
  change sets.
- Review returns exact snapshots and a hashed, one-time, ten-minute token bound to
  grant, change set, state and content digest. Apply, Reject and Undo consume it.
- Export and retranscription have separate one-time action tokens. Ready exports
  are served only as authorized `writers-studio://exports/{id}` MCP resources.
- Remote runs use `origin=remote_mcp`, `inputMode=mcp` and an agent reference, and
  appear in the existing Assistant history and review UI.

## Security properties

- Grant project lists are immutable; replacement requires revoke and reissue.
- Trash projects are never listed or opened. Archived projects are read-only.
- Host and Origin are checked against `PUBLIC_BASE_URL`; the Go MCP SDK also binds
  stateful session IDs to the authenticated grant ID.
- No bearer, OAuth, review or action token is stored in plaintext. HTTP and MCP
  audit logs contain no prompts, tool arguments/results, manuscript data or diffs.
- A turn is limited to 24 project calls, calls in one session are serialized, and a
  grant can have at most five active MCP sessions.

## Consequences

The web UI remains useful for issuing access and inspecting external runs, but it
is not part of the agent execution path. LAN clients must install the local CA and
provide DNS/hosts resolution for the configured Writer's Studio host.
