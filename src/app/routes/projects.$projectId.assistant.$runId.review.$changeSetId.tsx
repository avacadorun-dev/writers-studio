import { createFileRoute } from "@tanstack/react-router";

import { AssistantReviewPage } from "~pages/assistant-review";

export const Route = createFileRoute("/projects/$projectId/assistant/$runId/review/$changeSetId")({
  component: AssistantReviewRoute,
});

function AssistantReviewRoute() {
  const { projectId, runId, changeSetId } = Route.useParams();
  return <AssistantReviewPage projectId={projectId} runId={runId} changeSetId={changeSetId} />;
}
