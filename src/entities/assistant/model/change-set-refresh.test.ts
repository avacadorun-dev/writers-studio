import { describe, expect, mock, test } from "bun:test";
import type { QueryClient } from "@tanstack/react-query";

import {
  assistantChangedDocumentIds,
  refreshAssistantChangedDocuments,
} from "./change-set-refresh";
import type { AssistantChangeSet } from "./types";

function changeSet(): AssistantChangeSet {
  return {
    id: "change-set",
    runId: "run",
    turnId: "turn",
    state: "applied",
    title: "Правки",
    summary: "Две сцены",
    riskLevel: "low",
    operations: [
      operation("first", "document", "scene-a"),
      operation("second", "document", "scene-a"),
      operation("third", "node", "scene-b"),
      operation("canvas", "canvas", "canvas-a"),
    ],
    createdAt: "2026-08-06T00:00:00Z",
    updatedAt: "2026-08-06T00:00:00Z",
  };
}

function operation(id: string, targetKind: string, targetId: string) {
  return {
    id,
    position: 0,
    groupKey: "text",
    selected: true,
    dependencies: [],
    toolName: "document.patch",
    targetKind,
    targetId,
    arguments: {},
    proposedSnapshot: {},
    riskLevel: "low" as const,
    applied: true,
  };
}

describe("assistant change refresh", () => {
  test("returns unique document targets only", () => {
    expect(assistantChangedDocumentIds(changeSet())).toEqual(["scene-a", "scene-b"]);
  });

  test("refreshes document bodies before the project outline", async () => {
    const calls: Array<readonly unknown[]> = [];
    const queryClient = {
      invalidateQueries: mock(async ({ queryKey }: { queryKey: readonly unknown[] }) => {
        calls.push(queryKey);
      }),
    } as unknown as QueryClient;

    await refreshAssistantChangedDocuments(queryClient, "project", changeSet());

    expect(calls).toEqual([
      ["node", "scene-a"],
      ["node", "scene-b"],
      ["project", "project"],
    ]);
  });

  test("refreshes all active document queries for an operation-less SSE event", async () => {
    const calls: Array<readonly unknown[]> = [];
    const queryClient = {
      invalidateQueries: mock(async ({ queryKey }: { queryKey: readonly unknown[] }) => {
        calls.push(queryKey);
      }),
    } as unknown as QueryClient;

    await refreshAssistantChangedDocuments(queryClient, "project");

    expect(calls).toEqual([["node"], ["project", "project"]]);
  });
});
