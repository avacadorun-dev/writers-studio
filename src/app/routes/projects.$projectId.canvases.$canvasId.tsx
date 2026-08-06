import { createFileRoute } from "@tanstack/react-router";

import { CanvasPage } from "~pages/canvas";

export const Route = createFileRoute("/projects/$projectId/canvases/$canvasId")({
  component: CanvasRoute,
});

function CanvasRoute() {
  const { projectId, canvasId } = Route.useParams();
  return <CanvasPage projectId={projectId} canvasId={canvasId} />;
}
