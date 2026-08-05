import { createFileRoute } from "@tanstack/react-router";

import { AssistantContextPage } from "~pages/assistant-context";

export const Route = createFileRoute("/projects/$projectId/memory")({
  component: ProjectMemoryRoute,
});

function ProjectMemoryRoute() {
  const { projectId } = Route.useParams();
  return <AssistantContextPage projectId={projectId} view="memory" />;
}
