import { useQuery } from "@tanstack/react-query";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { useEffect } from "react";

import { getProject } from "~entities/project";
import { Spinner } from "~shared/ui";
import { RequireSession } from "~widgets/require-session";

export function OpenProjectPage({ projectId }: { projectId: string }) {
  const navigate = useNavigate();
  const search = useSearch({ from: "/projects/$projectId/" });
  const project = useQuery({
    queryKey: ["project", projectId],
    queryFn: () => getProject(projectId),
  });

  useEffect(() => {
    const firstNode = project.data?.nodes.find((node) => !node.trashedAt);
    if (!firstNode) {
      return;
    }
    void navigate({
      to: "/projects/$projectId/studio/$nodeId",
      params: { projectId, nodeId: firstNode.id },
      search: search.dictate ? { dictate: true } : {},
      replace: true,
    });
  }, [navigate, project.data, projectId, search.dictate]);

  return (
    <RequireSession>
      <main className="min-h-full">
        <Spinner label="Открываем рукопись…" />
      </main>
    </RequireSession>
  );
}
