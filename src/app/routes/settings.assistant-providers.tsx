import { createFileRoute } from "@tanstack/react-router";

import { AssistantProvidersPage } from "~pages/assistant-providers";

export const Route = createFileRoute("/settings/assistant-providers")({
  component: AssistantProvidersPage,
});
