import { createFileRoute, Outlet } from "@tanstack/react-router";

export const Route = createFileRoute("/projects/$projectId/assistant/$runId")({
  component: AssistantRunRoute,
});

function AssistantRunRoute() {
  return <Outlet />;
}
