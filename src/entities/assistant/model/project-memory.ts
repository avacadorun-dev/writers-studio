import type { ProjectMemoryRecord } from "./types";

export function projectMemoryUpdatePayload(record: ProjectMemoryRecord) {
  return {
    kind: record.kind,
    statement: record.statement,
    scope: record.scope,
    scopeNodeId: record.scopeNodeId,
    sourceReferences: record.sourceReferences,
    tags: record.tags,
    active: record.active,
    alwaysRelevant: record.alwaysRelevant,
    baseRevision: record.revision,
  };
}
