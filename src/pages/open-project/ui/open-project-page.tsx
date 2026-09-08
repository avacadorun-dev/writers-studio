import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { useCallback, useEffect } from "react";

import {
  createNode,
  type DocumentNode,
  getProject,
  type ProjectWorkspace,
  restoreNode,
} from "~entities/project";
import { Banner, Button, Spinner } from "~shared/ui";
import { RequireSession } from "~widgets/require-session";

export function selectProjectLandingNode(nodes: DocumentNode[]): DocumentNode | undefined {
  return nodes.find((node) => !node.trashedAt);
}

export function selectProjectRootKind(workspace: ProjectWorkspace): string | undefined {
  const types = workspace.project.structure.nodeTypes;
  return (
    types.find((type) => type.canBeRoot && type.contentBearing)?.key ??
    types.find((type) => type.canBeRoot)?.key
  );
}

export function OpenProjectPage({ projectId }: { projectId: string }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const search = useSearch({ from: "/projects/$projectId/" });
  const project = useQuery({
    queryKey: ["project", projectId],
    queryFn: () => getProject(projectId),
  });
  const openNode = useCallback(
    (nodeId: string) =>
      navigate({
        to: "/projects/$projectId/studio/$nodeId",
        params: { projectId, nodeId },
        search: search.dictate ? { dictate: true } : {},
        replace: true,
      }),
    [navigate, projectId, search.dictate],
  );
  const restoreFirst = useMutation({
    mutationFn: (nodeId: string) => restoreNode(nodeId),
    onSuccess: (_result, nodeId) => {
      queryClient.setQueryData<ProjectWorkspace>(["project", projectId], (current) =>
        current
          ? {
              ...current,
              nodes: current.nodes.map((node) =>
                node.id === nodeId ? { ...node, trashedAt: undefined } : node,
              ),
            }
          : current,
      );
      void openNode(nodeId);
    },
  });
  const createFirst = useMutation({
    mutationFn: async () => {
      if (!project.data) throw new Error("project is unavailable");
      const kind = selectProjectRootKind(project.data);
      if (!kind) throw new Error("project has no root node type");
      return createNode(projectId, { kind, title: "Новый материал" });
    },
    onSuccess: ({ node }) => {
      queryClient.setQueryData<ProjectWorkspace>(["project", projectId], (current) =>
        current ? { ...current, nodes: [...current.nodes, node] } : current,
      );
      void openNode(node.id);
    },
  });

  useEffect(() => {
    const firstNode = project.data && selectProjectLandingNode(project.data.nodes);
    if (!firstNode) {
      return;
    }
    void openNode(firstNode.id);
  }, [openNode, project.data]);

  if (project.isError) {
    return (
      <RequireSession>
        <main className="mx-auto grid min-h-full max-w-xl content-center gap-4 p-6">
          <Banner
            variant="danger"
            title="Не удалось открыть проект"
            body="Проект недоступен или был удалён. Вернитесь к списку либо повторите запрос."
            action={
              <Button size="compact" variant="secondary" onClick={() => void project.refetch()}>
                Повторить
              </Button>
            }
          />
          <Button asChild variant="ghost">
            <Link to="/projects">К проектам</Link>
          </Button>
        </main>
      </RequireSession>
    );
  }

  if (project.data && !selectProjectLandingNode(project.data.nodes)) {
    const recoverable = project.data.nodes.find((node) => node.trashedAt);
    const canCreate = Boolean(selectProjectRootKind(project.data));
    return (
      <RequireSession>
        <main className="mx-auto grid min-h-full max-w-xl content-center gap-5 p-6 text-center">
          <div>
            <p className="text-xs font-semibold tracking-[0.12em] text-[var(--ink-tertiary)] uppercase">
              Проект
            </p>
            <h1 className="font-display mt-2 text-3xl">{project.data.project.title}</h1>
            <p className="mt-3 text-sm text-[var(--ink-secondary)]">
              В рукописи пока нет активных материалов.
            </p>
          </div>
          {recoverable ? (
            <Banner
              variant="warning"
              title="Материалы находятся в корзине"
              body={`Можно восстановить «${recoverable.title}» и сразу продолжить работу.`}
              action={
                <Button
                  size="compact"
                  variant="secondary"
                  disabled={restoreFirst.isPending}
                  onClick={() => restoreFirst.mutate(recoverable.id)}
                >
                  {restoreFirst.isPending ? "Возвращаем…" : "Восстановить"}
                </Button>
              }
            />
          ) : null}
          {restoreFirst.isError || createFirst.isError ? (
            <p role="alert" className="text-sm text-[var(--danger)]">
              Не удалось подготовить материал. Повторите действие.
            </p>
          ) : null}
          <div className="flex flex-wrap justify-center gap-3">
            <Button
              disabled={!canCreate || createFirst.isPending}
              onClick={() => createFirst.mutate()}
            >
              {createFirst.isPending ? "Создаём…" : "Создать материал"}
            </Button>
            <Button asChild variant="ghost">
              <Link to="/projects">К проектам</Link>
            </Button>
          </div>
        </main>
      </RequireSession>
    );
  }

  return (
    <RequireSession>
      <main className="min-h-full">
        <Spinner label="Открываем рукопись…" />
      </main>
    </RequireSession>
  );
}
