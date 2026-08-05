import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";

import { getAssistantRun } from "~entities/assistant";
import { getProject } from "~entities/project";
import { StudioPage } from "~pages/studio";
import { Banner, Spinner } from "~shared/ui";

export const Route = createFileRoute("/projects/$projectId/assistant/$runId/")({
  component: AssistantSessionRoute,
});

function AssistantSessionRoute() {
  const { projectId, runId } = Route.useParams();
  const run = useQuery({
    queryKey: ["assistant-run", runId],
    queryFn: () => getAssistantRun(runId),
  });
  const project = useQuery({
    queryKey: ["project", projectId],
    queryFn: () => getProject(projectId),
  });
  if (run.isPending || project.isPending) return <Spinner label="Открываем workspace…" />;
  if (run.isError || project.isError || !run.data || !project.data) {
    return (
      <Banner
        variant="danger"
        title="Сессия недоступна"
        body="Вернитесь в проект и откройте другую сессию."
      />
    );
  }
  const targetNodeId = run.data.run.targetNodeId;
  const node =
    project.data.nodes.find((item) => item.id === targetNodeId && !item.trashedAt) ??
    project.data.nodes.find((item) => {
      if (item.trashedAt) return false;
      return project.data.project.structure.nodeTypes.some(
        (type) => type.key === item.kind && type.contentBearing,
      );
    });
  if (!node) {
    return (
      <Banner
        variant="warning"
        title="В проекте нет открываемого документа"
        body="Создайте документ и снова откройте сессию ассистента."
      />
    );
  }
  return <StudioPage projectId={projectId} nodeId={node.id} assistantRunId={runId} />;
}
