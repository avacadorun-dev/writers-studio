import { describe, expect, test } from "bun:test";

import type { ProjectMemoryRecord } from "~entities/assistant";
import type { DocumentNode } from "~entities/project";

import {
  createEmptyMemoryDraft,
  filterProjectMemory,
  memoryDraftFromRecord,
  memoryFieldsFromDraft,
  memoryKindLabel,
  memoryNodeTitle,
  memorySourceLabel,
  normalizeProjectMemoryRecord,
  parseMemoryTags,
  toggleMemorySource,
} from "./memory";

const node = { id: "node-1", title: "Сцена у реки" } as DocumentNode;
const record: ProjectMemoryRecord = {
  id: "memory-1",
  projectId: "project-1",
  kind: "character",
  statement: "Элина боится глубокой воды.",
  scope: "node",
  scopeNodeId: node.id,
  sourceReferences: [{ kind: "project_node", id: node.id, label: node.title }],
  tags: ["Элина", "канон"],
  active: true,
  alwaysRelevant: true,
  revision: 2,
  source: "assistant",
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-02T00:00:00Z",
};

describe("project memory presentation", () => {
  test("round-trips editable fields without aliasing source references", () => {
    const draft = memoryDraftFromRecord(record);
    const draftReference = draft.sourceReferences[0];
    if (!draftReference) throw new Error("source reference is missing");
    draftReference.label = "Изменено только в черновике";
    expect(record.sourceReferences[0]?.label).toBe(node.title);
    draftReference.label = node.title;
    expect(memoryFieldsFromDraft(draft)).toEqual({
      kind: "character",
      statement: record.statement,
      scope: "node",
      scopeNodeId: node.id,
      sourceReferences: record.sourceReferences,
      tags: record.tags,
      active: true,
      alwaysRelevant: true,
    });
  });

  test("normalizes tags and omits a node id for project scope", () => {
    expect(parseMemoryTags(" Канон, элина, КАНОН, , тайна ")).toEqual(["Канон", "элина", "тайна"]);
    const draft = createEmptyMemoryDraft();
    draft.scopeNodeId = node.id;
    expect(memoryFieldsFromDraft(draft).scopeNodeId).toBeUndefined();
  });

  test("normalizes null collections returned by legacy imports", () => {
    const legacy = {
      ...record,
      sourceReferences: null,
      tags: null,
    } as unknown as ProjectMemoryRecord;
    expect(normalizeProjectMemoryRecord(legacy)).toMatchObject({
      sourceReferences: [],
      tags: [],
    });
    expect(memoryDraftFromRecord(legacy).sourceReferences).toEqual([]);
    expect(filterProjectMemory([legacy], "", "all", "all")).toHaveLength(1);
  });

  test("adds and removes project-node sources while preserving unrelated references", () => {
    const external = { kind: "note", id: "external", label: "Интервью" };
    const added = toggleMemorySource([external], node);
    expect(added).toEqual([external, { kind: "project_node", id: node.id, label: node.title }]);
    expect(toggleMemorySource(added, node)).toEqual([external]);
  });

  test("searches metadata and combines kind and status filters", () => {
    const inactive = {
      ...record,
      id: "memory-2",
      kind: "plot" as const,
      statement: "Ключ уже у отца.",
      active: false,
      alwaysRelevant: false,
      tags: ["улика"],
      sourceReferences: [],
    };
    expect(filterProjectMemory([record, inactive], "реки", "all", "all")).toEqual([record]);
    expect(filterProjectMemory([record, inactive], "улика", "inactive", "plot")).toEqual([
      inactive,
    ]);
    expect(filterProjectMemory([record, inactive], "", "always", "all")).toEqual([record]);
    expect(filterProjectMemory([record, inactive], "", "active", "plot")).toEqual([]);
  });

  test("formats controlled vocabulary and node metadata", () => {
    expect(memoryKindLabel("world_rule")).toBe("Правило мира");
    expect(memoryKindLabel("other")).toBe("Другое");
    expect(memorySourceLabel("assistant")).toBe("предложено ассистентом");
    expect(memorySourceLabel("manual")).toBe("добавлено вручную");
    expect(memoryNodeTitle([node], node.id)).toBe(node.title);
    expect(memoryNodeTitle([node], "missing")).toBeUndefined();
  });
});
