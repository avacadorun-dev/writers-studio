import { createFileRoute } from "@tanstack/react-router";
import { z } from "zod";

import { ExportPage } from "~pages/export";

export const Route = createFileRoute("/projects/$projectId/export")({
  validateSearch: z.object({
    format: z.enum(["docx", "epub", "pdf", "markdown", "txt"]).optional(),
    nodeIds: z.array(z.string()).optional(),
    chapterTitles: z.boolean().optional(),
    assistantPrepared: z.boolean().optional(),
  }),
  component: ExportRoute,
});

function ExportRoute() {
  const search = Route.useSearch();
  return (
    <ExportPage
      projectId={Route.useParams().projectId}
      initialOptions={
        search.assistantPrepared
          ? {
              format: search.format,
              nodeIds: search.nodeIds,
              chapterTitles: search.chapterTitles,
            }
          : undefined
      }
    />
  );
}
