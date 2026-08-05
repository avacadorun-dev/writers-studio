import type { AssistantEvent, AssistantLivePreview, AssistantRun } from "~entities/assistant";

export function assistantLivePreview(
  run: AssistantRun | undefined,
  events: AssistantEvent[],
  forcedStale: boolean,
): AssistantLivePreview | undefined {
  const pendingOperation = run?.pendingChangeSet?.operations.find(
    (operation) => operation.targetKind === "document" && operation.targetId,
  );
  if (pendingOperation?.targetId) {
    return {
      operationId: pendingOperation.id,
      targetNodeId: pendingOperation.targetId,
      baseRevision: pendingOperation.baseRevision,
      toolName: pendingOperation.toolName,
      arguments: pendingOperation.arguments,
      beforeSnapshot: pendingOperation.beforeSnapshot,
      proposedSnapshot: pendingOperation.proposedSnapshot,
      trace: pendingOperation.trace as Record<string, unknown> | undefined,
      canonical: false,
      stale: forcedStale,
    };
  }
  let preview: AssistantLivePreview | undefined;
  for (const event of events) {
    if (event.type === "writers.proposal.updated") {
      preview = previewFromEvent(event, forcedStale) ?? preview;
      continue;
    }
    if (
      preview &&
      (event.type === "writers.change_set.applied" ||
        event.type === "writers.change_set.rejected" ||
        event.type === "writers.change_set.undone")
    ) {
      preview = undefined;
    } else if (preview && event.type === "writers.change_set.stale") {
      preview = { ...preview, stale: true };
    }
  }
  return preview;
}

function previewFromEvent(event: AssistantEvent, stale: boolean): AssistantLivePreview | undefined {
  const data = event.data;
  if (
    typeof data.operationId !== "string" ||
    typeof data.targetNodeId !== "string" ||
    typeof data.toolName !== "string" ||
    !data.arguments ||
    typeof data.arguments !== "object" ||
    !data.proposedSnapshot ||
    typeof data.proposedSnapshot !== "object"
  ) {
    return undefined;
  }
  return {
    operationId: data.operationId,
    targetNodeId: data.targetNodeId,
    baseRevision: typeof data.baseRevision === "number" ? data.baseRevision : undefined,
    toolName: data.toolName,
    arguments: data.arguments as Record<string, unknown>,
    beforeSnapshot:
      data.beforeSnapshot && typeof data.beforeSnapshot === "object"
        ? (data.beforeSnapshot as Record<string, unknown>)
        : undefined,
    proposedSnapshot: data.proposedSnapshot as Record<string, unknown>,
    trace:
      data.trace && typeof data.trace === "object"
        ? (data.trace as Record<string, unknown>)
        : undefined,
    canonical: false,
    stale,
  };
}
