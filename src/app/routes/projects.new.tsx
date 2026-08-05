import { createFileRoute } from "@tanstack/react-router";

import { CreateProjectPage } from "~pages/create-project";

export const Route = createFileRoute("/projects/new")({
  component: CreateProjectPage,
});
