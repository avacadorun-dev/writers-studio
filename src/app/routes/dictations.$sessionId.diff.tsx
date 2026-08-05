import { createFileRoute } from "@tanstack/react-router";

import { TranscriptDiffPage } from "~pages/transcript-diff";

export const Route = createFileRoute("/dictations/$sessionId/diff")({
  component: DiffRoute,
});

function DiffRoute() {
  return <TranscriptDiffPage sessionId={Route.useParams().sessionId} />;
}
