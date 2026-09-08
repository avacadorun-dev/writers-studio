import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { Bot, Copy, Pencil, Tags, Trash2 } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import { listAssistantRuns } from "~entities/assistant";
import {
  type CanvasDocument,
  type CanvasDocumentPreview,
  type CanvasWorkspace,
  duplicateCanvas,
  getCanvas,
  getCanvasTypeSchema,
  promoteCanvasNode,
  resolveCanvasTypeSchema,
  saveCanvasTypeSchema,
  trashCanvas,
  updateCanvas,
} from "~entities/canvas";
import { getProjectOutline } from "~entities/project";
import { useCanvasAutosave } from "~features/canvas-autosave";
import { CanvasSchemaEditor } from "~features/canvas-schema-editor";
import { AssistantSessionPage } from "~pages/assistant-session";
import { getCanvasSnapshot } from "~shared/lib";
import { Banner, Button, Spinner, useActionDialog, useToast } from "~shared/ui";
import {
  AssistantDockShell,
  type AssistantDockView,
  AssistantNewChatPanel,
  AssistantThreadSwitcher,
  loadAssistantDockState,
  saveAssistantDockState,
} from "~widgets/assistant-threads";
import { CanvasBoard } from "~widgets/canvas-board";
import { ProjectTree } from "~widgets/project-tree";
import { RequireSession } from "~widgets/require-session";
import { StudioHeader } from "~widgets/studio-header";

export function CanvasPage({
  projectId,
  canvasId,
  focusNodeId,
}: {
  projectId: string;
  canvasId: string;
  focusNodeId?: string;
}) {
  const project = useQuery({
    queryKey: ["project", projectId],
    queryFn: () => getProjectOutline(projectId),
  });
  const canvas = useQuery({
    queryKey: ["canvas", canvasId],
    queryFn: () => getCanvas(canvasId),
  });
  const typeSchema = useQuery({
    queryKey: ["canvas-schema", projectId],
    queryFn: () => getCanvasTypeSchema(projectId),
  });

  if (project.isPending || canvas.isPending || typeSchema.isPending)
    return <Spinner label="Открываем доску…" />;
  if (
    project.isError ||
    canvas.isError ||
    typeSchema.isError ||
    !project.data ||
    !canvas.data ||
    !typeSchema.data
  ) {
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
      typeSchema={resolveCanvasTypeSchema(typeSchema.data.schema)}
      focusNodeId={focusNodeId}
    />
  );
}

function LoadedCanvasPage({
  projectId,
  canvasId,
  workspace,
  project,
  typeSchema,
  focusNodeId,
}: {
  projectId: string;
  canvasId: string;
  workspace: CanvasWorkspace;
  project: Awaited<ReturnType<typeof getProjectOutline>>;
  typeSchema: ReturnType<typeof resolveCanvasTypeSchema>;
  focusNodeId?: string;
}) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { requestConfirmation, requestInput, requestSelect } = useActionDialog();
  const { showToast } = useToast();
  const [treeCollapsed, setTreeCollapsed] = useState(() => {
    const saved = localStorage.getItem("writers-studio-tree-collapsed");
    return saved === null ? window.innerWidth < 1280 : saved === "true";
  });
  const [revision, setRevision] = useState(workspace.revision.revision);
  const revisionRef = useRef(workspace.revision.revision);
  const [title, setTitle] = useState(workspace.canvas.title);
  const [recoveryReady, setRecoveryReady] = useState(false);
  const [initialContent, setInitialContent] = useState<CanvasDocument>(workspace.revision.content);
  const restoredSnapshotKey = useRef<string | undefined>(undefined);
  const [schemaEditorOpen, setSchemaEditorOpen] = useState(false);
  const [assistantPanelOpen, setAssistantPanelOpen] = useState(false);
  const [assistantPanelView, setAssistantPanelView] = useState<AssistantDockView>(
    () => loadAssistantDockState(projectId).view,
  );
  const [activeAssistantRunId, setActiveAssistantRunId] = useState<string | undefined>(
    () => loadAssistantDockState(projectId).runId,
  );
  const assistantReturnFocusRef = useRef<HTMLElement | null>(null);
  const [assistantContext, setAssistantContext] = useState({
    selectionIds: [] as string[],
    viewport: { x: 0, y: 0, zoom: 1 },
  });
  const autosave = useCanvasAutosave({
    canvasId,
    projectId,
    revision,
    onRevision: (nextRevision) => {
      revisionRef.current = nextRevision;
      setRevision(nextRevision);
    },
  });
  const assistantRuns = useQuery({
    queryKey: ["assistant-runs", projectId],
    queryFn: () => listAssistantRuns(projectId),
    refetchInterval: ({ state }) =>
      state.data?.runs.some((run) => run.status === "working") ? 1_200 : 10_000,
  });

  const openAssistantPanel = () => {
    assistantReturnFocusRef.current = document.activeElement as HTMLElement | null;
    setAssistantPanelOpen(true);
    setAssistantPanelView(
      activeAssistantRunId
        ? "session"
        : (assistantRuns.data?.runs.length ?? 0) > 0
          ? "threads"
          : "new",
    );
  };

  useEffect(() => {
    saveAssistantDockState(projectId, {
      runId: activeAssistantRunId,
      view: assistantPanelView,
    });
  }, [activeAssistantRunId, assistantPanelView, projectId]);

  useEffect(() => {
    let cancelled = false;
    void getCanvasSnapshot(canvasId)
      .then((snapshot) => {
        if (cancelled || !snapshot || snapshot.synced || snapshot.projectId !== projectId) return;
        const content = snapshot.content as unknown as CanvasDocument;
        if (!Array.isArray(content.nodes) || !Array.isArray(content.edges)) return;
        const snapshotKey = `${canvasId}:${snapshot.baseRevision}:${snapshot.savedAt}`;
        if (restoredSnapshotKey.current === snapshotKey) return;
        restoredSnapshotKey.current = snapshotKey;
        setInitialContent(content);
        autosave.queue(content);
        showToast({
          title: "Восстановлена локальная версия доски",
          description: navigator.onLine
            ? "Она будет синхронизирована автоматически."
            : "Она сохранена на этом устройстве и синхронизируется после подключения.",
        });
      })
      .catch(() => {
        if (cancelled) return;
        showToast({
          title: "Доска открыта без локального восстановления",
          description:
            "Локальное хранилище не ответило. Серверная версия доступна, и можно продолжить работу.",
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
  const saveTypeSchema = useMutation({
    mutationFn: (schema: ReturnType<typeof resolveCanvasTypeSchema>) =>
      saveCanvasTypeSchema(projectId, schema),
    onSuccess: ({ schema }) => {
      queryClient.setQueryData(["canvas-schema", projectId], { schema });
      setSchemaEditorOpen(false);
      showToast({ title: "Каталог Canvas сохранён" });
    },
    onError: (error) =>
      showToast({ title: "Не удалось сохранить каталог", description: error.message }),
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

  const promoteNode = async (nodeId: string, suggestedTitle: string) => {
    const titleValue = await requestInput({
      title: "Превратить идею в документ",
      label: "Название документа",
      defaultValue: suggestedTitle,
      confirmLabel: "Дальше",
    });
    const nextTitle = titleValue?.trim();
    if (!nextTitle) return undefined;
    const contentTypes = project.project.structure.nodeTypes.filter((item) => item.contentBearing);
    const kind = await requestSelect({
      title: "Тип документа",
      label: "Куда превратить карточку",
      options: contentTypes.map((item) => ({ value: item.key, label: item.label })),
      confirmLabel: "Дальше",
    });
    if (!kind) return undefined;
    const selectedType = contentTypes.find((item) => item.key === kind);
    const parentOptions = project.nodes
      .filter((node) => {
        if (node.trashedAt) return false;
        const parentType = project.project.structure.nodeTypes.find(
          (item) => item.key === node.kind,
        );
        return parentType?.allowedChildren.includes(kind);
      })
      .map((node) => ({ value: node.id, label: node.title }));
    if (selectedType?.canBeRoot) parentOptions.unshift({ value: "root", label: "В корне проекта" });
    if (parentOptions.length === 0) {
      showToast({
        title: "Некуда поместить документ",
        description: "Добавьте подходящий раздел в структуру проекта.",
      });
      return undefined;
    }
    const parent =
      parentOptions.length === 1
        ? parentOptions[0]?.value
        : await requestSelect({
            title: "Расположение документа",
            label: "Раздел проекта",
            options: parentOptions,
            confirmLabel: "Создать документ",
          });
    if (!parent) return undefined;
    try {
      const result = await promoteCanvasNode(canvasId, {
        baseRevision: revisionRef.current,
        nodeId,
        title: nextTitle,
        kind,
        parentId: parent === "root" ? undefined : parent,
      });
      revisionRef.current = result.revision.revision;
      setRevision(result.revision.revision);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["canvas", canvasId] }),
      ]);
      showToast({ title: "Идея стала документом", description: `Создан «${nextTitle}»` });
      return result;
    } catch (error) {
      showToast({
        title: "Не удалось создать документ",
        description: error instanceof Error ? error.message : undefined,
      });
      return undefined;
    }
  };

  return (
    <RequireSession>
      <div className="flex h-full min-h-0 min-w-0 flex-col overflow-hidden bg-[var(--surface-app)]">
        <StudioHeader
          project={project.project}
          nodeTitle={title}
          saveState={autosave.saveState}
          onAssistant={openAssistantPanel}
        />
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
          <main className="relative flex min-h-0 min-w-0 flex-1">
            <div className="relative min-h-0 min-w-0 flex-1">
              <fieldset className="absolute top-16 right-3 z-20 flex items-center gap-1 rounded-[12px] border border-[var(--border-subtle)] bg-[var(--surface-raised)] p-1.5 shadow-lg lg:top-3">
                <legend className="sr-only">Действия с доской</legend>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="size-8"
                  aria-label="Открыть Ассистента для этой доски"
                  title="Ассистент Canvas"
                  onClick={openAssistantPanel}
                >
                  <Bot className="size-3.5" aria-hidden="true" />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="size-8"
                  aria-label="Настроить типы карточек и связей"
                  title="Каталог Canvas"
                  onClick={() => setSchemaEditorOpen(true)}
                >
                  <Tags className="size-3.5" aria-hidden="true" />
                </Button>
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
                  title={title}
                  content={initialContent}
                  documents={previews}
                  media={workspace.media}
                  projectDocuments={projectDocuments}
                  typeSchema={typeSchema}
                  promotionEnabled={autosave.saveState === "saved"}
                  onPromoteNode={promoteNode}
                  initialFocusNodeId={focusNodeId}
                  onChange={(content) => autosave.queue(content)}
                  onAssistantContextChange={setAssistantContext}
                  onOpenDocument={(nodeId) => {
                    void navigate({
                      to: "/projects/$projectId/studio/$nodeId",
                      params: { projectId, nodeId },
                    });
                  }}
                  onOpenCanvas={(nextCanvasId) => {
                    void navigate({
                      to: "/projects/$projectId/canvases/$canvasId",
                      params: { projectId, canvasId: nextCanvasId },
                    });
                  }}
                />
              ) : (
                <Spinner label="Проверяем локальные изменения…" />
              )}
              <CanvasSchemaEditor
                open={schemaEditorOpen}
                schema={typeSchema}
                pending={saveTypeSchema.isPending}
                onOpenChange={setSchemaEditorOpen}
                onSave={(schema) => saveTypeSchema.mutate(schema)}
              />
            </div>
            {assistantPanelOpen ? (
              <AssistantDockShell
                testId="canvas-assistant-panel"
                returnFocusRef={assistantReturnFocusRef}
              >
                {assistantPanelView === "session" && activeAssistantRunId ? (
                  <AssistantSessionPage
                    key={activeAssistantRunId}
                    embedded
                    projectId={projectId}
                    runId={activeAssistantRunId}
                    targetLabel={title}
                    onOpenThreads={() => setAssistantPanelView("threads")}
                    onNewThread={() => setAssistantPanelView("new")}
                    onClose={() => setAssistantPanelOpen(false)}
                  />
                ) : assistantPanelView === "threads" ? (
                  <AssistantThreadSwitcher
                    projectId={projectId}
                    activeRunId={activeAssistantRunId}
                    nodes={project.nodes}
                    onSelect={(runId) => {
                      setActiveAssistantRunId(runId);
                      setAssistantPanelView("session");
                    }}
                    onNew={() => setAssistantPanelView("new")}
                    onClose={() => setAssistantPanelOpen(false)}
                  />
                ) : (
                  <AssistantNewChatPanel
                    project={project.project}
                    canvas={{ id: canvasId, title }}
                    canvasContext={assistantContext}
                    hasThreads={(assistantRuns.data?.runs.length ?? 0) > 0}
                    backgroundWorkingCount={
                      assistantRuns.data?.runs.filter((run) => run.status === "working").length ?? 0
                    }
                    onStarted={(runId) => {
                      setActiveAssistantRunId(runId);
                      setAssistantPanelView("session");
                      setAssistantPanelOpen(true);
                    }}
                    onBack={() => setAssistantPanelView("threads")}
                    onClose={() => setAssistantPanelOpen(false)}
                  />
                )}
              </AssistantDockShell>
            ) : null}
          </main>
        </div>
      </div>
    </RequireSession>
  );
}
