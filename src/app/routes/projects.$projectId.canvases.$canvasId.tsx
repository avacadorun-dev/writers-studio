import { createFileRoute } from "@tanstack/react-router";
import { z } from "zod";

import { CanvasPage } from "~pages/canvas";

export const Route = createFileRoute("/projects/$projectId/canvases/$canvasId")({
  validateSearch: z.object({ focus: z.string().optional() }),
  component: CanvasRoute,
});

function CanvasRoute() {
  const { projectId, canvasId } = Route.useParams();
  const { focus } = Route.useSearch();
  return <CanvasPage projectId={projectId} canvasId={canvasId} focusNodeId={focus} />;
}
