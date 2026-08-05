import { createFileRoute } from "@tanstack/react-router";

import { RecordingPage } from "~pages/recording";

export const Route = createFileRoute("/projects/$projectId/dictate/$localId")({
  component: RecordingRoute,
});

function RecordingRoute() {
  const { projectId, localId } = Route.useParams();
  return <RecordingPage projectId={projectId} localId={localId} />;
}
