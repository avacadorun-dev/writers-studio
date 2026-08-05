import { createFileRoute } from "@tanstack/react-router";
import { z } from "zod";

import { VersionsPage } from "~pages/versions";

export const Route = createFileRoute("/nodes/$nodeId/versions")({
  validateSearch: z.object({
    versionId: z.string().optional(),
    expectedCurrentRevision: z.number().int().nonnegative().optional(),
  }),
  component: VersionsRoute,
});

function VersionsRoute() {
  const search = Route.useSearch();
  return (
    <VersionsPage
      nodeId={Route.useParams().nodeId}
      preparedVersionId={search.versionId}
      expectedCurrentRevision={search.expectedCurrentRevision}
    />
  );
}
