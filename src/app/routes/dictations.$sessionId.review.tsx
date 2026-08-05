import { createFileRoute } from "@tanstack/react-router";

import { ReviewPage } from "~pages/review";

export const Route = createFileRoute("/dictations/$sessionId/review")({
  component: ReviewRoute,
});

function ReviewRoute() {
  return <ReviewPage sessionId={Route.useParams().sessionId} />;
}
