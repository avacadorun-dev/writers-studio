import { createFileRoute, Outlet } from "@tanstack/react-router";
import { z } from "zod";

export const Route = createFileRoute("/settings")({
  validateSearch: z.object({
    projectId: z.string().optional(),
    nodeId: z.string().optional(),
  }),
  component: () => <Outlet />,
});
