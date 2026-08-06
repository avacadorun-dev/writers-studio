import type { QueryClient } from "@tanstack/react-query";

import type { AssistantChangeSet } from "./types";

export function assistantChangedDocumentIds(changeSet: AssistantChangeSet): string[] {
  return [
    ...new Set(
      changeSet.operations.flatMap((operation) =>
        operation.targetId && ["document", "node"].includes(operation.targetKind)
          ? [operation.targetId]
          : [],
      ),
    ),
  ];
}

export async function refreshAssistantChangedDocuments(
  queryClient: QueryClient,
  projectId: string,
  changeSet?: AssistantChangeSet,
): Promise<void> {
  const nodeIds = changeSet ? assistantChangedDocumentIds(changeSet) : undefined;
  if (nodeIds) {
    await Promise.all(
      nodeIds.map((nodeId) =>
        queryClient.invalidateQueries({ queryKey: ["node", nodeId], exact: true }),
      ),
    );
  } else {
    // Durable SSE events do not carry operation bodies. Refresh every active
    // document query before the outline so Studio never combines a new summary
    // revision with an old document body.
    await queryClient.invalidateQueries({ queryKey: ["node"] });
  }
  await queryClient.invalidateQueries({ queryKey: ["project", projectId] });
}
