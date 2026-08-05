import { createFileRoute } from "@tanstack/react-router";
import { z } from "zod";

import { OpenProjectPage } from "~pages/open-project";

export const Route = createFileRoute("/projects/$projectId/")({
  validateSearch: z.object({
    dictate: z.boolean().optional(),
  }),
  component: ProjectIndexRoute,
});

function ProjectIndexRoute() {
  return <OpenProjectPage projectId={Route.useParams().projectId} />;
}
