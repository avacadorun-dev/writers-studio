import type { ProjectMemoryRecord } from "~entities/assistant";
import type { DocumentNode } from "~entities/project";

export type MemoryStatusFilter = "all" | "active" | "inactive" | "always";
export type MemoryKindFilter = "all" | ProjectMemoryRecord["kind"];

export interface MemoryDraft {
  kind: ProjectMemoryRecord["kind"];
  statement: string;
  scope: ProjectMemoryRecord["scope"];
  scopeNodeId: string;
  sourceReferences: ProjectMemoryRecord["sourceReferences"];
  tagsText: string;
  active: boolean;
  alwaysRelevant: boolean;
}

export function createEmptyMemoryDraft(): MemoryDraft {
  return {
    kind: "fact",
    statement: "",
    scope: "project",
    scopeNodeId: "",
    sourceReferences: [],
    tagsText: "",
    active: true,
    alwaysRelevant: false,
  };
}

export function normalizeProjectMemoryRecord(record: ProjectMemoryRecord): ProjectMemoryRecord {
  return {
    ...record,
    sourceReferences: record.sourceReferences ?? [],
    tags: record.tags ?? [],
  };
}

export function memoryDraftFromRecord(record: ProjectMemoryRecord): MemoryDraft {
  const normalized = normalizeProjectMemoryRecord(record);
  return {
    kind: normalized.kind,
    statement: normalized.statement,
    scope: normalized.scope,
    scopeNodeId: normalized.scopeNodeId ?? "",
    sourceReferences: normalized.sourceReferences.map((reference) => ({ ...reference })),
    tagsText: normalized.tags.join(", "),
    active: normalized.active,
    alwaysRelevant: normalized.alwaysRelevant,
  };
}

export function memoryFieldsFromDraft(draft: MemoryDraft) {
  return {
    kind: draft.kind,
    statement: draft.statement.trim(),
    scope: draft.scope,
    scopeNodeId: draft.scope === "node" ? draft.scopeNodeId || undefined : undefined,
    sourceReferences: draft.sourceReferences.map((reference) => ({ ...reference })),
    tags: parseMemoryTags(draft.tagsText),
    active: draft.active,
    alwaysRelevant: draft.alwaysRelevant,
  };
}

export function parseMemoryTags(value: string): string[] {
  const seen = new Set<string>();
  return value
    .split(",")
    .map((tag) => tag.trim())
    .filter((tag) => {
      if (!tag) return false;
      const normalized = tag.toLocaleLowerCase("ru");
      if (seen.has(normalized)) return false;
      seen.add(normalized);
      return true;
    });
}

export function toggleMemorySource(
  references: ProjectMemoryRecord["sourceReferences"],
  node: DocumentNode,
): ProjectMemoryRecord["sourceReferences"] {
  if (references.some((reference) => reference.id === node.id)) {
    return references.filter((reference) => reference.id !== node.id);
  }
  return [...references, { kind: "project_node", id: node.id, label: node.title }];
}

export function filterProjectMemory(
  records: ProjectMemoryRecord[],
  query: string,
  status: MemoryStatusFilter,
  kind: MemoryKindFilter,
): ProjectMemoryRecord[] {
  const normalizedQuery = query.trim().toLocaleLowerCase("ru");
  return records.map(normalizeProjectMemoryRecord).filter((record) => {
    if (kind !== "all" && record.kind !== kind) return false;
    if (status === "active" && !record.active) return false;
    if (status === "inactive" && record.active) return false;
    if (status === "always" && !record.alwaysRelevant) return false;
    if (!normalizedQuery) return true;
    const searchable = [
      record.statement,
      record.kind,
      record.scope,
      ...record.tags,
      ...record.sourceReferences.flatMap((reference) => [
        reference.label ?? "",
        reference.quote ?? "",
      ]),
    ]
      .join(" ")
      .toLocaleLowerCase("ru");
    return searchable.includes(normalizedQuery);
  });
}

export function memoryNodeTitle(nodes: DocumentNode[], nodeId?: string): string | undefined {
  if (!nodeId) return undefined;
  return nodes.find((node) => node.id === nodeId)?.title;
}

export function memoryKindLabel(kind: ProjectMemoryRecord["kind"]): string {
  const labels: Record<ProjectMemoryRecord["kind"], string> = {
    fact: "Факт",
    character: "Персонаж",
    world_rule: "Правило мира",
    plot: "Сюжет",
    style: "Стиль",
    constraint: "Ограничение",
    decision: "Решение",
    other: "Другое",
  };
  return labels[kind];
}

export function memorySourceLabel(source: string): string {
  return source === "assistant" ? "предложено ассистентом" : "добавлено вручную";
}
