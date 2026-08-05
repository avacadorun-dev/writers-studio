import { describe, expect, test } from "bun:test";

import type { AssistantRun } from "~entities/assistant";
import {
  assistantThreadBucket,
  assistantThreadCountLabel,
  assistantThreadStatus,
  assistantThreadTitle,
  assistantThreadUpdatedLabel,
  groupAssistantThreads,
} from "./assistant-threads";

function run(overrides: Partial<AssistantRun> = {}): AssistantRun {
  return {
    id: "run",
    projectId: "project",
    title: "Сделать сцену тревожнее",
    targetKind: "node",
    targetNodeId: "node",
    inputMode: "chat",
    status: "active",
    silenceCheckpointMs: 5000,
    origin: "internal",
    provider: {
      kind: "system",
      name: "System",
      model: "writer",
      dialect: "responses",
      capabilities: {
        text: true,
        strictTools: true,
        toolResults: true,
        sequentialToolCalls: true,
        streaming: true,
        parallelToolCalls: false,
      },
    },
    readContext: {},
    startedAt: "2026-08-04T00:00:00Z",
    updatedAt: "2026-08-04T00:00:00Z",
    lastViewedAt: "2026-08-04T00:00:00Z",
    unread: false,
    ...overrides,
  };
}

describe("assistant project threads", () => {
  test("groups review, working and ordinary runs by lifecycle priority", () => {
    const review = run({ id: "review", status: "awaiting_review" });
    const working = run({ id: "working", status: "working" });
    const recent = run({ id: "recent", status: "completed" });
    const groups = groupAssistantThreads([recent, working, review]);
    expect(groups.attention.map((item) => item.id)).toEqual(["review"]);
    expect(groups.working.map((item) => item.id)).toEqual(["working"]);
    expect(groups.recent.map((item) => item.id)).toEqual(["recent"]);
  });

  test("keeps a stale parallel proposal in attention even after the run becomes active", () => {
    const stale = run({
      status: "active",
      errorCode: "stale_change_set",
      latestTurn: { id: "turn", errorCode: "stale_change_set" } as AssistantRun["latestTurn"],
      latestChangeSet: { state: "stale", turnId: "turn" } as AssistantRun["latestChangeSet"],
    });
    expect(assistantThreadBucket(stale)).toBe("attention");
    expect(assistantThreadStatus(stale)).toBe("ОБНОВИТЬ");
  });

  test("derives a useful fallback title and calm relative time", () => {
    const untitled = run({
      title: "Новый чат",
      latestTurn: { instruction: "  Уточнить   мотивацию героя  " } as AssistantRun["latestTurn"],
    });
    expect(assistantThreadTitle(untitled)).toBe("Уточнить мотивацию героя");
    expect(
      assistantThreadUpdatedLabel("2026-08-04T11:58:00Z", Date.parse("2026-08-04T12:00:00Z")),
    ).toBe("2 мин");
  });

  test("declines the visible thread counter", () => {
    expect(assistantThreadCountLabel(1)).toBe("1 чат");
    expect(assistantThreadCountLabel(2)).toBe("2 чата");
    expect(assistantThreadCountLabel(5)).toBe("5 чатов");
    expect(assistantThreadCountLabel(12)).toBe("12 чатов");
    expect(assistantThreadCountLabel(21)).toBe("21 чат");
  });
});
