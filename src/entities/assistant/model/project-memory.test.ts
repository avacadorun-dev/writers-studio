import { describe, expect, test } from "bun:test";
import { projectMemoryUpdatePayload } from "./project-memory";
import type { ProjectMemoryRecord } from "./types";

describe("project memory API payload", () => {
  test("sends only writable fields accepted by the strict update endpoint", () => {
    const record: ProjectMemoryRecord = {
      id: "memory-1",
      projectId: "project-1",
      kind: "fact",
      statement: "Элина боится воды.",
      scope: "project",
      sourceReferences: [],
      tags: ["Элина"],
      active: true,
      alwaysRelevant: false,
      revision: 4,
      source: "assistant",
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-02T00:00:00Z",
    };
    expect(projectMemoryUpdatePayload(record)).toEqual({
      kind: "fact",
      statement: "Элина боится воды.",
      scope: "project",
      scopeNodeId: undefined,
      sourceReferences: [],
      tags: ["Элина"],
      active: true,
      alwaysRelevant: false,
      baseRevision: 4,
    });
    expect("id" in projectMemoryUpdatePayload(record)).toBe(false);
    expect("createdAt" in projectMemoryUpdatePayload(record)).toBe(false);
  });
});
