import { createFileRoute } from "@tanstack/react-router";

import { DictionaryPage } from "~pages/dictionary";

export const Route = createFileRoute("/projects/$projectId/dictionary")({
  component: DictionaryRoute,
});

function DictionaryRoute() {
  return <DictionaryPage projectId={Route.useParams().projectId} />;
}
