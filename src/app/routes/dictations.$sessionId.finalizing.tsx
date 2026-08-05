import { createFileRoute } from "@tanstack/react-router";

import { FinalizingPage } from "~pages/finalizing";

export const Route = createFileRoute("/dictations/$sessionId/finalizing")({
  component: FinalizingRoute,
});

function FinalizingRoute() {
  return <FinalizingPage sessionId={Route.useParams().sessionId} />;
}
