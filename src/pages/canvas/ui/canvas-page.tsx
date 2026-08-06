import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { Copy, Pencil, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import {
  type CanvasDocument,
  type CanvasDocumentPreview,
  type CanvasWorkspace,
  duplicateCanvas,
  getCanvas,
  trashCanvas,
  updateCanvas,
} from "~entities/canvas";
import { getProjectOutline } from "~entities/project";
import { useCanvasAutosave } from "~features/canvas-autosave";
import { getCanvasSnapshot } from "~shared/lib";
import { Banner, Button, Spinner, useActionDialog, useToast } from "~shared/ui";
import { CanvasBoard } from "~widgets/canvas-board";
import { ProjectTree } from "~widgets/project-tree";
import { RequireSession } from "~widgets/require-session";
import { StudioHeader } from "~widgets/studio-header";

export function CanvasPage({ projectId, canvasId }: { projectId: string; canvasId: string }) {
  const project = useQuery({
    queryKey: ["project", projectId],
    queryFn: () => getProjectOutline(projectId),
  });
  const canvas = useQuery({
    queryKey: ["canvas", canvasId],
    queryFn: () => getCanvas(canvasId),
  });

  if (project.isPending || canvas.isPending) return <Spinner label="Открываем доску…" />;
  if (project.isError || canvas.isError || !project.data || !canvas.data) {
    return (
      <main className="mx-auto max-w-xl p-6">
        <Banner
          variant="danger"
          title="Не удалось открыть доску"
          body="Проверьте соединение и обновите страницу."
        />
      </main>
    );
  }
  if (canvas.data.canvas.projectId !== projectId || canvas.data.canvas.trashedAt) {
    return (
      <main className="mx-auto max-w-xl p-6">
        <Banner
          variant="warning"
          title="Доска недоступна"
          body="Вернитесь в проект и выберите другую доску."
        />
      </main>
    );
  }

  return (
    <LoadedCanvasPage
      key={canvasId}
      projectId={projectId}
      canvasId={canvasId}
      workspace={canvas.data}
      project={project.data}
    />
  );
}

function LoadedCanvasPage({
  projectId,
  canvasId,
  workspace,
  project,
}: {
  projectId: string;
  canvasId: string;
  workspace: CanvasWorkspace;
  project: Awaited<ReturnType<typeof getProjectOutline>>;
}) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { requestConfirmation, requestInput } = useActionDialog();
  const { showToast } = useToast();
  const [treeCollapsed, setTreeCollapsed] = useState(() => {
    const saved = localStorage.getItem("writers-studio-tree-collapsed");
    return saved === null ? window.innerWidth < 1280 : saved === "true";
  });
  const [revision, setRevision] = useState(workspace.revision.revision);
  const [title, setTitle] = useState(workspace.canvas.title);
  const [recoveryReady, setRecoveryReady] = useState(false);
  const [initialContent, setInitialContent] = useState<CanvasDocument>(workspace.revision.content);
  const autosave = useCanvasAutosave({ canvasId, projectId, revision, onRevision: setRevision });

  useEffect(() => {
    let cancelled = false;
    void getCanvasSnapshot(canvasId)
      .then((snapshot) => {
        if (cancelled || !snapshot || snapshot.synced || snapshot.projectId !== projectId) return;
        const content = snapshot.content as unknown as CanvasDocument;
        if (!Array.isArray(content.nodes) || !Array.isArray(content.edges)) return;
        setInitialContent(content);
        autosave.queue(content);
        showToast({
          title: "Восстановлена локальная версия доски",
          description: navigator.onLine
            ? "Она будет синхронизирована автоматически."
            : "Она сохранена на этом устройстве и синхронизируется после подключения.",
        });
      })
      .finally(() => {
        if (!cancelled) setRecoveryReady(true);
      });
    return () => {
      cancelled = true;
    };
  }, [autosave.queue, canvasId, projectId, showToast]);

  const invalidateCanvases = () =>
    queryClient.invalidateQueries({ queryKey: ["canvases", projectId] });
  const rename = useMutation({
    mutationFn: (nextTitle: string) => updateCanvas(canvasId, { title: nextTitle }),
    onSuccess: async ({ canvas }) => {
      setTitle(canvas.title);
      await Promise.all([
        invalidateCanvases(),
        queryClient.invalidateQueries({ queryKey: ["canvas", canvasId] }),
      ]);
      showToast({ title: "Доска переименована" });
    },
    onError: (error) =>
      showToast({ title: "Не удалось переименовать", description: error.message }),
  });
  const duplicate = useMutation({
    mutationFn: () => duplicateCanvas(canvasId),
    onSuccess: async ({ canvas }) => {
      await invalidateCanvases();
      showToast({ title: "Создана копия доски" });
      void navigate({
        to: "/projects/$projectId/canvases/$canvasId",
        params: { projectId, canvasId: canvas.id },
      });
    },
    onError: (error) =>
      showToast({ title: "Не удалось создать копию", description: error.message }),
  });
  const trash = useMutation({
    mutationFn: () => trashCanvas(canvasId),
    onSuccess: async () => {
      await invalidateCanvases();
      const firstNode = project.nodes.find((node) => !node.trashedAt);
      if (firstNode) {
        void navigate({
          to: "/projects/$projectId/studio/$nodeId",
          params: { projectId, nodeId: firstNode.id },
        });
      } else {
        void navigate({ to: "/projects/$projectId", params: { projectId } });
      }
    },
    onError: (error) =>
      showToast({ title: "Не удалось удалить доску", description: error.message }),
  });

  const documentTypeKeys = useMemo(
    () =>
      new Set(
        project.project.structure.nodeTypes
          .filter((item) => item.contentBearing)
          .map((item) => item.key),
      ),
    [project.project.structure.nodeTypes],
  );
  const projectDocuments = useMemo(
    () => project.nodes.filter((item) => documentTypeKeys.has(item.kind)),
    [documentTypeKeys, project.nodes],
  );
  const previews = useMemo<CanvasDocumentPreview[]>(() => {
    const resolved = new Map(workspace.documents.map((item) => [item.id, item]));
    return projectDocuments.map(
      (item) =>
        resolved.get(item.id) ?? {
          id: item.id,
          title: item.title,
          kind: item.kind,
          status: item.status,
          currentRevision: item.currentRevision,
          plainText: item.plainText ?? "",
          trashedAt: item.trashedAt,
        },
    );
  }, [projectDocuments, workspace.documents]);
  const firstNode = project.nodes.find((node) => !node.trashedAt);

  return (
    <RequireSession>
      <div className="flex h-full min-h-0 flex-col bg-[var(--surface-app)]">
        <StudioHeader project={project.project} nodeTitle={title} saveState={autosave.saveState} />
        {autosave.conflictCanvasId ? (
          <Banner
            className="m-3 mb-0 shrink-0"
            variant="warning"
            title="Сохранены оба варианта"
            body="Параллельные изменения вынесены в отдельную конфликтующую доску."
            action={
              <Button asChild variant="secondary" size="compact">
                <Link
                  to="/projects/$projectId/canvases/$canvasId"
                  params={{ projectId, canvasId: autosave.conflictCanvasId }}
                >
                  Открыть вариант
                </Link>
              </Button>
            }
          />
        ) : null}
        <div className="flex min-h-0 min-w-0 flex-1">
          <ProjectTree
            projectId={projectId}
            activeNodeId={firstNode?.id ?? ""}
            activeCanvasId={canvasId}
            nodes={project.nodes}
            structure={project.project.structure}
            collapsed={treeCollapsed}
            onToggle={() => {
              setTreeCollapsed((current) => {
                localStorage.setItem("writers-studio-tree-collapsed", String(!current));
                return !current;
              });
            }}
          />
          <main className="relative min-h-0 min-w-0 flex-1">
            <fieldset className="absolute top-16 right-3 z-20 flex items-center gap-1 rounded-[12px] border border-[var(--border-subtle)] bg-[var(--surface-raised)] p-1.5 shadow-lg lg:top-3">
              <legend className="sr-only">Действия с доской</legend>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="size-8"
                aria-label="Переименовать доску"
                title="Переименовать"
                onClick={() =>
                  void requestInput({
                    title: "Переименовать доску",
                    label: "Название",
                    defaultValue: title,
                    icon: Pencil,
                    confirmLabel: "Сохранить",
                  }).then((value) => {
                    if (value?.trim() && value.trim() !== title) rename.mutate(value.trim());
                  })
                }
              >
                <Pencil className="size-3.5" aria-hidden="true" />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="size-8"
                aria-label="Создать копию доски"
                title="Создать копию"
                onClick={() => duplicate.mutate()}
              >
                <Copy className="size-3.5" aria-hidden="true" />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="size-8 text-[var(--danger)]"
                aria-label="Переместить доску в корзину"
                title="В корзину"
                onClick={() =>
                  void requestConfirmation({
                    title: `Удалить «${title}»?`,
                    description: "Доску можно будет восстановить из корзины проекта.",
                    icon: Trash2,
                    tone: "danger",
                    confirmLabel: "В корзину",
                  }).then((confirmed) => confirmed && trash.mutate())
                }
              >
                <Trash2 className="size-3.5" aria-hidden="true" />
              </Button>
            </fieldset>
            {recoveryReady ? (
              <CanvasBoard
                projectId={projectId}
                content={initialContent}
                documents={previews}
                media={workspace.media}
                projectDocuments={projectDocuments}
                onChange={(content) => autosave.queue(content)}
                onOpenDocument={(nodeId) => {
                  void navigate({
                    to: "/projects/$projectId/studio/$nodeId",
                    params: { projectId, nodeId },
                  });
                }}
              />
            ) : (
              <Spinner label="Проверяем локальные изменения…" />
            )}
          </main>
        </div>
      </div>
    </RequireSession>
  );
}
