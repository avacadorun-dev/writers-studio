import { createFileRoute } from "@tanstack/react-router";

import { AssistantContextPage } from "~pages/assistant-context";

export const Route = createFileRoute("/projects/$projectId/assistant-guide")({
  component: AssistantGuideRoute,
});

function AssistantGuideRoute() {
  const { projectId } = Route.useParams();
  return <AssistantContextPage projectId={projectId} view="guide" />;
}
