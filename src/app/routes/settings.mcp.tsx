import { createFileRoute } from "@tanstack/react-router";

import { MCPSettingsPage } from "~pages/mcp-settings";

export const Route = createFileRoute("/settings/mcp")({
  component: MCPSettingsPage,
});
