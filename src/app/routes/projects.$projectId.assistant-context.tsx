import { createFileRoute } from "@tanstack/react-router";

import { AssistantContextPage } from "~pages/assistant-context";

export const Route = createFileRoute("/projects/$projectId/assistant-context")({
  component: AssistantContextRoute,
});

function AssistantContextRoute() {
  const { projectId } = Route.useParams();
  return <AssistantContextPage projectId={projectId} view="overview" />;
}
