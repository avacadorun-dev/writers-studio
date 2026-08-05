import { describe, expect, test } from "bun:test";

import type { AssistantEvent, AssistantRun } from "~entities/assistant";

import { assistantLivePreview } from "./assistant-live-preview";

function event(sequence: number, type: string, data: Record<string, unknown>): AssistantEvent {
  return {
    id: `event-${sequence}`,
    runId: "run",
    sequence,
    type,
    data,
    createdAt: new Date(0).toISOString(),
  };
}

const proposal = {
  operationId: "operation",
  targetNodeId: "node",
  baseRevision: 4,
  toolName: "document.replace_range",
  arguments: { edits: [] },
  proposedSnapshot: { type: "doc", content: [] },
};

describe("assistant live manuscript preview", () => {
  test("uses a real proposal event while provider work is still running", () => {
    expect(
      assistantLivePreview(undefined, [event(1, "writers.proposal.updated", proposal)], false),
    ).toMatchObject({
      operationId: "operation",
      targetNodeId: "node",
      canonical: false,
      stale: false,
    });
  });

  test("marks the proposal stale and removes it after a terminal decision", () => {
    const stale = assistantLivePreview(
      undefined,
      [event(1, "writers.proposal.updated", proposal), event(2, "writers.change_set.stale", {})],
      false,
    );
    expect(stale?.stale).toBe(true);
    expect(
      assistantLivePreview(
        undefined,
        [
          event(1, "writers.proposal.updated", proposal),
          event(2, "writers.change_set.applied", {}),
        ],
        false,
      ),
    ).toBeUndefined();
  });

  test("prefers the durable staged operation after reconnect", () => {
    const run = {
      pendingChangeSet: {
        operations: [
          {
            id: "durable-operation",
            targetKind: "document",
            targetId: "node",
            toolName: "document.replace_range",
            arguments: {},
            proposedSnapshot: { type: "doc", content: [] },
          },
        ],
      },
    } as unknown as AssistantRun;
    expect(assistantLivePreview(run, [], false)?.operationId).toBe("durable-operation");
  });
});
