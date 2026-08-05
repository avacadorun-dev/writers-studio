import { createFileRoute } from "@tanstack/react-router";

import { ProjectAnnotationsPage } from "~pages/annotations";

export const Route = createFileRoute("/projects/$projectId/annotations")({
  component: ProjectAnnotationsRoute,
});

function ProjectAnnotationsRoute() {
  return <ProjectAnnotationsPage projectId={Route.useParams().projectId} />;
}
