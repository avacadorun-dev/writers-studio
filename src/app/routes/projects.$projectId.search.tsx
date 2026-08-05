import { createFileRoute } from "@tanstack/react-router";
import { z } from "zod";

import { SearchPage } from "~pages/search";

export const Route = createFileRoute("/projects/$projectId/search")({
  validateSearch: z.object({
    q: z.string().optional(),
    scope: z.string().optional(),
  }),
  component: SearchRoute,
});

function SearchRoute() {
  const { projectId } = Route.useParams();
  const search = Route.useSearch();
  return <SearchPage projectId={projectId} initialQuery={search.q} initialScope={search.scope} />;
}
