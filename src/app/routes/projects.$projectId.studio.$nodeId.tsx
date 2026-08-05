import { createFileRoute } from "@tanstack/react-router";
import { z } from "zod";

import { StudioPage } from "~pages/studio";

export const Route = createFileRoute("/projects/$projectId/studio/$nodeId")({
  validateSearch: z.object({
    dictate: z.boolean().optional(),
    assistant: z.boolean().optional(),
    highlight: z.string().optional(),
    assistantRunId: z.string().optional(),
    assistantReviewId: z.string().optional(),
    annotationId: z.string().optional(),
  }),
  component: StudioRoute,
});

function StudioRoute() {
  const { projectId, nodeId } = Route.useParams();
  const { dictate, assistant, highlight, assistantRunId, assistantReviewId, annotationId } =
    Route.useSearch();
  return (
    <StudioPage
      projectId={projectId}
      nodeId={nodeId}
      dictate={dictate}
      assistant={assistant}
      highlight={highlight}
      assistantRunId={assistantRunId}
      assistantReviewId={assistantReviewId}
      annotationId={annotationId}
    />
  );
}
