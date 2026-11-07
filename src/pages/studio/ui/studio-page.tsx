import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import {
  BookMarked,
  Download,
  Focus,
  History,
  MessageSquarePlus,
  Mic,
  Save,
  Search,
  SearchCode,
  SpellCheck2,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  type AssistantLivePreview,
  type AssistantSelectionAttachment,
  listAssistantRuns,
} from "~entities/assistant";
import {
  type AnnotationAnchor,
  createDocumentAnnotation,
  createVersion,
  type DictionaryTerm,
  type DocumentNode,
  getNode,
  getProjectOutline,
  listDictionary,
  listDocumentAnnotations,
  type ProjectWorkspace,
  updateNode,
} from "~entities/project";
import { AssistantReviewPage } from "~pages/assistant-review";
import { AssistantSessionPage } from "~pages/assistant-session";
import { cn } from "~shared/lib";
import {
  Banner,
  Button,
  Input,
  ModalDialog,
  type SaveState,
  SaveStatus,
  Spinner,
  useActionDialog,
  useToast,
} from "~shared/ui";
import {
  AssistantDockShell,
  type AssistantDockView,
  AssistantNewChatPanel,
  AssistantThreadSwitcher,
  loadAssistantDockState,
  saveAssistantDockState,
} from "~widgets/assistant-threads";
import { DictationPreflight } from "~widgets/dictation-preflight";
import { ManuscriptAnnotationsPanel } from "~widgets/manuscript-annotations";
import { ManuscriptEditor } from "~widgets/manuscript-editor";
import { ProjectContextRail } from "~widgets/project-context-rail";
import { ProjectTree } from "~widgets/project-tree";
import { RequireSession } from "~widgets/require-session";
import { StudioHeader } from "~widgets/studio-header";

interface StudioPageProps {
  projectId: string;
  nodeId: string;
  dictate?: boolean;
  assistant?: boolean;
  highlight?: string;
  assistantRunId?: string;
  assistantReviewId?: string;
  annotationId?: string;
}

function AnnotationComposerDialog({
  pending,
  note,
  onNote,
  dictionaryQuery,
  onDictionaryQuery,
  terms,
  selectedTermId,
  onSelectTerm,
  saving,
  onClose,
  onSave,
}: {
  pending?: {
    kind: "note" | "dictionary_link";
    anchor: AnnotationAnchor;
    baseRevision: number;
  };
  note: string;
  onNote: (value: string) => void;
  dictionaryQuery: string;
  onDictionaryQuery: (value: string) => void;
  terms: DictionaryTerm[];
  selectedTermId?: string;
  onSelectTerm: (value: string) => void;
  saving: boolean;
  onClose: () => void;
  onSave: () => void;
}) {
  const normalized = dictionaryQuery.trim().toLocaleLowerCase();
  const filteredTerms = terms.filter(
    (term) =>
      term.active &&
      (!normalized ||
        term.canonical.toLocaleLowerCase().includes(normalized) ||
        term.variants.some((variant) => variant.toLocaleLowerCase().includes(normalized))),
  );
  const isNote = pending?.kind === "note";
  return (
    <ModalDialog
      open={Boolean(pending)}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={isNote ? "Заметка на полях" : "Связать со словарём"}
      description={
        pending
          ? `Фрагмент «${pending.anchor.exactText}» останется обычным текстом рукописи.`
          : undefined
      }
      icon={isNote ? MessageSquarePlus : BookMarked}
      footer={
        <>
          <Button type="button" variant="ghost" onClick={onClose}>
            Отмена
          </Button>
          <Button
            type="button"
            disabled={saving || (isNote ? !note.trim() : !selectedTermId)}
            onClick={onSave}
          >
            {saving ? "Сохраняем…" : isNote ? "Добавить заметку" : "Связать"}
          </Button>
        </>
      }
    >
      {isNote ? (
        <label className="block text-sm font-semibold">
          Что нужно исправить или проверить
          <textarea
            value={note}
            onChange={(event) => onNote(event.target.value)}
            rows={5}
            maxLength={10000}
            placeholder="Например: уточнить мотивацию героя и сократить повтор"
            className="mt-2 w-full resize-y rounded-[var(--radius-md)] border border-[var(--border-strong)] bg-[var(--surface-paper)] px-3 py-2 font-normal leading-6 outline-none focus:border-[var(--focus)]"
          />
        </label>
      ) : (
        <div>
          <label htmlFor="annotation-dictionary-search" className="block text-sm font-semibold">
            Найти термин
            <Input
              id="annotation-dictionary-search"
              className="mt-2"
              value={dictionaryQuery}
              onChange={(event) => onDictionaryQuery(event.target.value)}
              placeholder="Имя, место или термин"
            />
          </label>
          <div className="mt-3 max-h-64 space-y-1 overflow-y-auto" role="listbox">
            {filteredTerms.length > 0 ? (
              filteredTerms.map((term) => (
                <button
                  key={term.id}
                  type="button"
                  role="option"
                  aria-selected={selectedTermId === term.id}
                  className={cn(
                    "flex w-full items-center justify-between rounded-[var(--radius-sm)] px-3 py-2 text-left text-sm",
                    selectedTermId === term.id
                      ? "bg-[var(--surface-selected)] text-[var(--accent-ink)]"
                      : "hover:bg-[var(--surface-subtle)]",
                  )}
                  onClick={() => onSelectTerm(term.id)}
                >
                  <span>
                    <span className="block font-semibold">{term.canonical}</span>
                    <span className="text-xs text-[var(--ink-secondary)]">
                      {termKindLabel(term.kind)}
                    </span>
                  </span>
                  {selectedTermId === term.id ? <span aria-hidden="true">✓</span> : null}
                </button>
              ))
            ) : (
              <p className="rounded-[var(--radius-md)] bg-[var(--surface-subtle)] p-3 text-sm text-[var(--ink-secondary)]">
                Подходящих активных терминов нет. Добавьте термин на странице словаря и повторите.
              </p>
            )}
          </div>
        </div>
      )}
    </ModalDialog>
  );
}

function termKindLabel(kind: DictionaryTerm["kind"]): string {
  return { character: "Персонаж", place: "Место", term: "Термин", other: "Другое" }[kind];
}

export function StudioPage({
  projectId,
  nodeId,
  dictate = false,
  assistant = false,
  highlight,
  assistantRunId,
  assistantReviewId,
  annotationId,
}: StudioPageProps) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { showToast } = useToast();
  const { requestInput } = useActionDialog();
  const [saveState, setSaveState] = useState<SaveState>("saved");
  const [mobileTreeOpen, setMobileTreeOpen] = useState(false);
  const [treeCollapsed, setTreeCollapsed] = useState(() => {
    const savedState = localStorage.getItem("writers-studio-tree-collapsed");
    return savedState === null ? window.innerWidth < 1280 : savedState === "true";
  });
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [activeAssistantRunId, setActiveAssistantRunId] = useState(
    () => assistantRunId ?? loadAssistantDockState(projectId).runId,
  );
  const [assistantPanelOpen, setAssistantPanelOpen] = useState(
    Boolean(assistantRunId || assistant),
  );
  const [assistantPanelView, setAssistantPanelView] = useState<AssistantDockView>(() =>
    assistantRunId ? "session" : loadAssistantDockState(projectId).view,
  );
  const assistantReturnFocusRef = useRef<HTMLElement | null>(null);
  const [activeAssistantReviewId, setActiveAssistantReviewId] = useState(assistantReviewId);
  const [assistantSelection, setAssistantSelection] = useState<AssistantSelectionAttachment>();
  const [assistantSelectionOwner, setAssistantSelectionOwner] = useState<string>();
  const [assistantPreviews, setAssistantPreviews] = useState<
    Record<string, { preview?: AssistantLivePreview; stale: boolean }>
  >({});
  const [annotationPanelOpen, setAnnotationPanelOpen] = useState(Boolean(annotationId));
  const [selectedAnnotationId, setSelectedAnnotationId] = useState(annotationId);
  const [pendingAnnotation, setPendingAnnotation] = useState<{
    kind: "note" | "dictionary_link";
    anchor: AnnotationAnchor;
    baseRevision: number;
  }>();
  const [noteDraft, setNoteDraft] = useState("");
  const [dictionaryQuery, setDictionaryQuery] = useState("");
  const [selectedTermId, setSelectedTermId] = useState<string>();
  const [paletteQuery, setPaletteQuery] = useState("");
  const project = useQuery({
    queryKey: ["project", projectId],
    queryFn: () => getProjectOutline(projectId),
  });
  const activeDocument = useQuery({
    queryKey: ["node", nodeId],
    queryFn: () => getNode(nodeId),
  });
  const assistantRuns = useQuery({
    queryKey: ["assistant-runs", projectId],
    queryFn: () => listAssistantRuns(projectId),
    refetchInterval: ({ state }) =>
      state.data?.runs.some((run) => run.status === "working") ? 1_200 : 10_000,
  });
  const annotations = useQuery({
    queryKey: ["annotations", "node", nodeId],
    queryFn: () => listDocumentAnnotations(nodeId),
  });
  const dictionary = useQuery({
    queryKey: ["dictionary", projectId],
    queryFn: () => listDictionary(projectId),
    enabled: pendingAnnotation?.kind === "dictionary_link",
  });
  const renameNodeMutation = useMutation({
    mutationFn: (title: string) => updateNode(nodeId, { title }),
    onMutate: async (title) => {
      await Promise.all([
        queryClient.cancelQueries({ queryKey: ["project", projectId] }),
        queryClient.cancelQueries({ queryKey: ["node", nodeId] }),
      ]);
      const previousProject = queryClient.getQueryData<ProjectWorkspace>(["project", projectId]);
      const previousNode = queryClient.getQueryData<{ node: DocumentNode }>(["node", nodeId]);
      queryClient.setQueryData<ProjectWorkspace>(["project", projectId], (current) =>
        current
          ? {
              ...current,
              nodes: current.nodes.map((item) => (item.id === nodeId ? { ...item, title } : item)),
            }
          : current,
      );
      queryClient.setQueryData<{ node: DocumentNode }>(["node", nodeId], (current) =>
        current ? { ...current, node: { ...current.node, title } } : current,
      );
      return { previousProject, previousNode };
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["node", nodeId] }),
      ]);
      showToast({ title: "Название сохранено" });
    },
    onError: (error, _title, context) => {
      if (context?.previousProject) {
        queryClient.setQueryData(["project", projectId], context.previousProject);
      }
      if (context?.previousNode) {
        queryClient.setQueryData(["node", nodeId], context.previousNode);
      }
      showToast({ title: "Не удалось переименовать", description: error.message });
    },
  });
  useEffect(() => {
    if (!annotationId) return;
    setSelectedAnnotationId(annotationId);
    setAnnotationPanelOpen(true);
  }, [annotationId]);
  useEffect(() => {
    saveAssistantDockState(projectId, {
      runId: activeAssistantRunId,
      view: assistantPanelView,
    });
  }, [activeAssistantRunId, assistantPanelView, projectId]);
  useEffect(() => {
    if (assistantRunId) {
      setActiveAssistantRunId(assistantRunId);
      setAssistantPanelOpen(true);
      setAssistantPanelView("session");
    }
  }, [assistantRunId]);
  useEffect(() => {
    if (!assistant || assistantRunId) return;
    setAssistantPanelOpen(true);
    setAssistantPanelView("new");
  }, [assistant, assistantRunId]);
  useEffect(() => {
    setActiveAssistantReviewId(assistantReviewId);
  }, [assistantReviewId]);

  const syncAssistantLocation = useCallback(
    (runId?: string, reviewId?: string) => {
      void navigate({
        to: "/projects/$projectId/studio/$nodeId",
        params: { projectId, nodeId },
        search: {
          highlight,
          assistantRunId: runId,
          assistantReviewId: reviewId,
          annotationId: selectedAnnotationId,
        },
        replace: true,
      });
    },
    [highlight, navigate, nodeId, projectId, selectedAnnotationId],
  );
  const selectAnnotation = useCallback(
    (nextId: string) => {
      setSelectedAnnotationId(nextId);
      setAnnotationPanelOpen(true);
      setAssistantPanelOpen(false);
      void navigate({
        to: "/projects/$projectId/studio/$nodeId",
        params: { projectId, nodeId },
        search: {
          highlight,
          assistantRunId: activeAssistantRunId,
          assistantReviewId: activeAssistantReviewId,
          annotationId: nextId,
        },
        replace: true,
      });
    },
    [activeAssistantReviewId, activeAssistantRunId, highlight, navigate, nodeId, projectId],
  );
  const closeAnnotations = useCallback(() => {
    setAnnotationPanelOpen(false);
    setSelectedAnnotationId(undefined);
    void navigate({
      to: "/projects/$projectId/studio/$nodeId",
      params: { projectId, nodeId },
      search: {
        highlight,
        assistantRunId: activeAssistantRunId,
        assistantReviewId: activeAssistantReviewId,
      },
      replace: true,
    });
  }, [activeAssistantReviewId, activeAssistantRunId, highlight, navigate, nodeId, projectId]);
  const createAnnotation = useMutation({
    mutationFn: async () => {
      if (!pendingAnnotation) throw new Error("Нет выделенного фрагмента");
      return createDocumentAnnotation(nodeId, {
        kind: pendingAnnotation.kind,
        anchor: pendingAnnotation.anchor,
        baseRevision: pendingAnnotation.baseRevision,
        note: pendingAnnotation.kind === "note" ? noteDraft.trim() : undefined,
        dictionaryTermId: pendingAnnotation.kind === "dictionary_link" ? selectedTermId : undefined,
      });
    },
    onSuccess: async ({ annotation }) => {
      setPendingAnnotation(undefined);
      setNoteDraft("");
      setDictionaryQuery("");
      setSelectedTermId(undefined);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["annotations", "node", nodeId] }),
        queryClient.invalidateQueries({ queryKey: ["annotations", "project", projectId] }),
      ]);
      selectAnnotation(annotation.id);
      showToast({
        title: annotation.kind === "note" ? "Заметка добавлена" : "Ссылка на словарь добавлена",
      });
    },
    onError: (error) =>
      showToast({ title: "Не удалось добавить пометку", description: error.message }),
  });
  const handleDocumentSaved = useCallback(() => {
    void Promise.all([
      queryClient.invalidateQueries({ queryKey: ["project", projectId] }),
      queryClient.invalidateQueries({ queryKey: ["node", nodeId] }),
      queryClient.invalidateQueries({ queryKey: ["annotations", "node", nodeId] }),
      queryClient.invalidateQueries({ queryKey: ["annotations", "project", projectId] }),
    ]);
  }, [nodeId, projectId, queryClient]);
  const closePreflight = useCallback(() => {
    void navigate({
      to: "/projects/$projectId/studio/$nodeId",
      params: { projectId, nodeId },
      search: highlight ? { highlight } : {},
      replace: true,
    });
  }, [highlight, navigate, nodeId, projectId]);
  const openPreflight = useCallback(() => {
    void navigate({
      to: "/projects/$projectId/studio/$nodeId",
      params: { projectId, nodeId },
      search: { dictate: true, highlight },
      replace: true,
    });
  }, [highlight, navigate, nodeId, projectId]);
  const saveVersion = useMutation({
    mutationFn: (name: string) => createVersion(nodeId, { name, note: "Горячая клавиша" }),
    onSuccess: () => showToast({ title: "Именованная версия создана" }),
    onError: (error) =>
      showToast({ title: "Не удалось сохранить версию", description: error.message }),
  });
  const requestVersion = useCallback(() => {
    if (saveState !== "saved") {
      showToast({
        title: "Дождитесь сохранения текста",
        description: "Версия должна точно соответствовать текущей редакции на сервере.",
      });
      return;
    }
    const defaultName = `Версия ${new Intl.DateTimeFormat("ru", {
      day: "2-digit",
      month: "short",
      hour: "2-digit",
      minute: "2-digit",
    }).format(new Date())}`;
    void requestInput({
      title: "Сохранить именованную версию",
      description: "Текущая редакция останется в истории и получит узнаваемое название.",
      label: "Название версии",
      defaultValue: defaultName,
      icon: Save,
      confirmLabel: "Сохранить версию",
    }).then((name) => {
      if (name?.trim()) {
        saveVersion.mutate(name.trim());
      }
    });
  }, [requestInput, saveState, saveVersion, showToast]);
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      const modifier = event.metaKey || event.ctrlKey;
      if (!modifier) {
        if (event.key === "Escape") {
          setPaletteOpen(false);
        }
        return;
      }
      if (event.key.toLocaleLowerCase() === "k" && !event.shiftKey) {
        event.preventDefault();
        setPaletteOpen(true);
      } else if (event.key.toLocaleLowerCase() === "f" && event.shiftKey) {
        event.preventDefault();
        void navigate({
          to: "/projects/$projectId/search",
          params: { projectId },
        });
      } else if (event.key.toLocaleLowerCase() === "s" && event.shiftKey) {
        event.preventDefault();
        requestVersion();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [navigate, projectId, requestVersion]);

  if (project.isPending || activeDocument.isPending) {
    return <Spinner label="Открываем проект…" />;
  }
  if (project.isError || activeDocument.isError || !project.data || !activeDocument.data) {
    return (
      <main className="mx-auto max-w-xl p-6">
        <Banner
          variant="danger"
          title="Не удалось открыть проект"
          body="Проверьте соединение и обновите страницу."
        />
      </main>
    );
  }
  const nodeSummary = project.data.nodes.find((item) => item.id === nodeId && !item.trashedAt);
  const node = nodeSummary
    ? {
        ...activeDocument.data.node,
        ...nodeSummary,
        content: activeDocument.data.node.content,
        currentRevision: activeDocument.data.node.currentRevision,
        updatedAt: activeDocument.data.node.updatedAt,
      }
    : activeDocument.data.node;
  if (!nodeSummary || node.trashedAt || node.projectId !== projectId) {
    return (
      <main className="mx-auto max-w-xl p-6">
        <Banner
          variant="warning"
          title="Документ не найден"
          body="Вернитесь в проект и выберите другой раздел."
        />
      </main>
    );
  }
  const nodeTypeLabel =
    project.data.project.structure.nodeTypes.find((type) => type.key === node.kind)?.label ??
    "Документ";
  const activeAssistantPreview = activeAssistantRunId
    ? assistantPreviews[activeAssistantRunId]
    : undefined;
  const activePreviewStale = Boolean(
    activeAssistantPreview?.stale ||
      (activeAssistantPreview?.preview?.baseRevision !== undefined &&
        activeAssistantPreview.preview.baseRevision !== node.currentRevision),
  );
  const integratedAssistantWorkspace = assistantPanelOpen;
  const integratedWorkspace = integratedAssistantWorkspace || annotationPanelOpen;
  const visibleAnnotations = annotations.data?.annotations;
  const activeAnnotationCount =
    visibleAnnotations?.filter((item) => item.status !== "resolved").length ?? 0;

  return (
    <RequireSession>
      <div className="studio-workspace flex h-full min-h-0 min-w-0 flex-col overflow-hidden">
        <StudioHeader
          documentsOpen={mobileTreeOpen}
          onOpenDocuments={() => setMobileTreeOpen(true)}
          project={project.data.project}
          nodeId={nodeId}
          nodeTitle={node.title}
          saveState={saveState}
          onRenameNode={(title) => renameNodeMutation.mutate(title)}
          onDictate={openPreflight}
          onAssistant={() => {
            assistantReturnFocusRef.current = document.activeElement as HTMLElement | null;
            setAnnotationPanelOpen(false);
            setAssistantPanelOpen(true);
            setAssistantPanelView(
              activeAssistantRunId
                ? "session"
                : (assistantRuns.data?.runs.length ?? 0) > 0
                  ? "threads"
                  : "new",
            );
            if (activeAssistantRunId) {
              syncAssistantLocation(activeAssistantRunId, activeAssistantReviewId);
            }
          }}
        />
        <div className="flex min-h-10 items-center border-b border-[var(--border-subtle)] bg-[var(--surface-panel)] px-3 md:hidden">
          <SaveStatus state={saveState} />
        </div>
        <div className="flex min-h-0 min-w-0 flex-1">
          <ProjectTree
            mobileOpen={mobileTreeOpen}
            onMobileOpenChange={setMobileTreeOpen}
            projectId={projectId}
            activeNodeId={nodeId}
            nodes={project.data.nodes}
            structure={project.data.project.structure}
            collapsed={treeCollapsed}
            integratedWorkspace={integratedWorkspace}
            onToggle={() => {
              setTreeCollapsed((current) => {
                localStorage.setItem("writers-studio-tree-collapsed", String(!current));
                return !current;
              });
            }}
          />
          <ManuscriptEditor
            node={node}
            nodeTypeLabel={nodeTypeLabel}
            highlight={highlight}
            onSaveState={setSaveState}
            onDictate={openPreflight}
            onRenameNode={(title) => renameNodeMutation.mutate(title)}
            assistantSelection={assistantSelection}
            assistantPreview={
              activeAssistantPreview?.preview?.targetNodeId === nodeId
                ? { ...activeAssistantPreview.preview, stale: activePreviewStale }
                : undefined
            }
            onAssistantSelectionChange={(selection) => {
              setAssistantSelection(selection);
              if (!selection) {
                setAssistantSelectionOwner(undefined);
                return;
              }
              const owner = activeAssistantRunId ?? "new";
              assistantReturnFocusRef.current = document.activeElement as HTMLElement | null;
              setAssistantSelectionOwner(owner);
              setAnnotationPanelOpen(false);
              setAssistantPanelOpen(true);
              setAssistantPanelView(activeAssistantRunId ? "session" : "new");
              if (activeAssistantRunId) {
                syncAssistantLocation(activeAssistantRunId, activeAssistantReviewId);
              }
            }}
            onAssistantPreviewInvalidated={() => {
              if (!activeAssistantRunId) return;
              setAssistantPreviews((current) => ({
                ...current,
                [activeAssistantRunId]: {
                  ...current[activeAssistantRunId],
                  stale: true,
                },
              }));
            }}
            integratedWorkspace={integratedWorkspace}
            annotations={visibleAnnotations}
            activeAnnotationId={selectedAnnotationId}
            onAnnotationSelect={selectAnnotation}
            onCreateNote={(anchor, baseRevision) => {
              setPendingAnnotation({ kind: "note", anchor, baseRevision });
              setNoteDraft("");
            }}
            onLinkDictionary={(anchor, baseRevision) => {
              setPendingAnnotation({ kind: "dictionary_link", anchor, baseRevision });
              setSelectedTermId(undefined);
              setDictionaryQuery(anchor.exactText);
            }}
            onRevisionSaved={handleDocumentSaved}
          />
          {!assistantPanelOpen ? (
            annotationPanelOpen && visibleAnnotations ? (
              <ManuscriptAnnotationsPanel
                projectId={projectId}
                nodeId={nodeId}
                annotations={visibleAnnotations}
                selectedId={selectedAnnotationId}
                onSelect={selectAnnotation}
                onClose={closeAnnotations}
              />
            ) : (
              <ProjectContextRail
                nodeId={nodeId}
                annotationCount={activeAnnotationCount}
                onAnnotations={() => setAnnotationPanelOpen(true)}
              />
            )
          ) : null}
          {assistantPanelOpen ? (
            <AssistantDockShell returnFocusRef={assistantReturnFocusRef}>
              {assistantPanelView === "session" && activeAssistantRunId ? (
                <AssistantSessionPage
                  key={activeAssistantRunId}
                  embedded
                  projectId={projectId}
                  runId={activeAssistantRunId}
                  selectionAttachment={
                    assistantSelectionOwner === activeAssistantRunId
                      ? assistantSelection
                      : undefined
                  }
                  previewStale={activePreviewStale}
                  onSelectionConsumed={() => {
                    setAssistantSelection(undefined);
                    setAssistantSelectionOwner(undefined);
                  }}
                  onLivePreviewChange={(preview) => {
                    setAssistantPreviews((current) => {
                      const previous = current[activeAssistantRunId];
                      return {
                        ...current,
                        [activeAssistantRunId]: {
                          preview,
                          stale:
                            preview && previous?.preview?.operationId !== preview.operationId
                              ? false
                              : (previous?.stale ?? false),
                        },
                      };
                    });
                  }}
                  onReview={(changeSetId) => setActiveAssistantReviewId(changeSetId)}
                  onOpenThreads={() => setAssistantPanelView("threads")}
                  onNewThread={() => setAssistantPanelView("new")}
                  onClose={() => {
                    setAssistantPanelOpen(false);
                    syncAssistantLocation();
                  }}
                />
              ) : assistantPanelView === "threads" ? (
                <AssistantThreadSwitcher
                  projectId={projectId}
                  activeRunId={activeAssistantRunId}
                  nodes={project.data.nodes}
                  onSelect={(runId) => {
                    setActiveAssistantRunId(runId);
                    setActiveAssistantReviewId(undefined);
                    setAssistantPanelView("session");
                    syncAssistantLocation(runId);
                  }}
                  onNew={() => setAssistantPanelView("new")}
                  onClose={() => {
                    setAssistantPanelOpen(false);
                    syncAssistantLocation();
                  }}
                />
              ) : (
                <AssistantNewChatPanel
                  project={project.data.project}
                  node={node}
                  selectionAttachment={
                    assistantSelectionOwner === "new" ? assistantSelection : undefined
                  }
                  hasThreads={(assistantRuns.data?.runs.length ?? 0) > 0}
                  backgroundWorkingCount={
                    assistantRuns.data?.runs.filter((run) => run.status === "working").length ?? 0
                  }
                  onStarted={(runId) => {
                    setActiveAssistantRunId(runId);
                    setAssistantPanelView("session");
                    setAssistantPanelOpen(true);
                    syncAssistantLocation(runId);
                  }}
                  onSelectionConsumed={() => {
                    setAssistantSelection(undefined);
                    setAssistantSelectionOwner(undefined);
                  }}
                  onBack={() => setAssistantPanelView("threads")}
                  onClose={() => {
                    setAssistantPanelOpen(false);
                    syncAssistantLocation();
                  }}
                />
              )}
            </AssistantDockShell>
          ) : null}
        </div>
        {dictate ? (
          <DictationPreflight
            project={project.data.project}
            nodes={project.data.nodes}
            destinationNodeId={nodeId}
            onClose={closePreflight}
            onStarted={(localId) => {
              void navigate({
                to: "/projects/$projectId/dictate/$localId",
                params: { projectId, localId },
              });
            }}
          />
        ) : null}
        <AnnotationComposerDialog
          pending={pendingAnnotation}
          note={noteDraft}
          onNote={setNoteDraft}
          dictionaryQuery={dictionaryQuery}
          onDictionaryQuery={setDictionaryQuery}
          terms={dictionary.data?.terms ?? []}
          selectedTermId={selectedTermId}
          onSelectTerm={setSelectedTermId}
          saving={createAnnotation.isPending}
          onClose={() => setPendingAnnotation(undefined)}
          onSave={() => createAnnotation.mutate()}
        />
        {activeAssistantRunId && activeAssistantReviewId ? (
          <AssistantReviewPage
            embedded
            projectId={projectId}
            runId={activeAssistantRunId}
            changeSetId={activeAssistantReviewId}
            onClose={() => {
              setActiveAssistantReviewId(undefined);
              setAssistantPanelOpen(true);
            }}
          />
        ) : null}
        {paletteOpen ? (
          <CommandPalette
            query={paletteQuery}
            onQuery={setPaletteQuery}
            onClose={() => setPaletteOpen(false)}
            commands={[
              {
                label: "Найти в документе",
                shortcut: "⌘/Ctrl F",
                icon: SearchCode,
                action: () => window.dispatchEvent(new CustomEvent("writers-studio-find")),
              },
              {
                label: "Поиск по проекту",
                shortcut: "⌘/Ctrl ⇧ F",
                icon: Search,
                action: () =>
                  void navigate({
                    to: "/projects/$projectId/search",
                    params: { projectId },
                  }),
              },
              {
                label: "Сохранить именованную версию",
                shortcut: "⌘/Ctrl ⇧ S",
                icon: Save,
                action: requestVersion,
              },
              {
                label: "Режим фокуса",
                shortcut: "⌘/Ctrl ⇧ Enter",
                icon: Focus,
                action: () => window.dispatchEvent(new CustomEvent("writers-studio-focus")),
              },
              { label: "Начать диктовку", icon: Mic, action: openPreflight },
              {
                label: "Открыть версии",
                icon: History,
                action: () => void navigate({ to: "/nodes/$nodeId/versions", params: { nodeId } }),
              },
              {
                label: "Открыть словарь",
                icon: SpellCheck2,
                action: () =>
                  void navigate({
                    to: "/projects/$projectId/dictionary",
                    params: { projectId },
                  }),
              },
              {
                label: "Экспортировать",
                icon: Download,
                action: () =>
                  void navigate({
                    to: "/projects/$projectId/export",
                    params: { projectId },
                  }),
              },
            ]}
          />
        ) : null}
      </div>
    </RequireSession>
  );
}

interface PaletteCommand {
  label: string;
  shortcut?: string;
  icon: typeof Search;
  action: () => void;
}

function CommandPalette({
  query,
  onQuery,
  onClose,
  commands,
}: {
  query: string;
  onQuery: (value: string) => void;
  onClose: () => void;
  commands: PaletteCommand[];
}) {
  const filtered = useMemo(
    () =>
      commands.filter((command) =>
        command.label.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase()),
      ),
    [commands, query],
  );
  return (
    <ModalDialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title="Палитра команд"
      description="Быстрый переход к основным действиям редактора."
      icon={Search}
      className="sm:top-[12vh] sm:w-[min(576px,calc(100vw-40px))] sm:translate-y-0"
    >
      <Input
        autoFocus
        aria-label="Найти команду"
        placeholder="Найти команду…"
        value={query}
        className="h-12 px-4"
        onChange={(event) => onQuery(event.target.value)}
      />
      <div className="mt-2 max-h-[42vh] overflow-y-auto rounded-[var(--radius-md)] border border-[var(--border-subtle)] p-2">
        {filtered.map((command) => (
          <Button
            key={command.label}
            type="button"
            variant="ghost"
            className="w-full justify-start"
            onClick={() => {
              onClose();
              command.action();
            }}
          >
            <command.icon className="size-4" aria-hidden="true" />
            {command.label}
            {command.shortcut ? (
              <kbd className="ml-auto text-[11px] text-[var(--ink-tertiary)]">
                {command.shortcut}
              </kbd>
            ) : null}
          </Button>
        ))}
        {filtered.length === 0 ? (
          <p className="p-5 text-center text-sm text-[var(--ink-tertiary)]">Команд не найдено</p>
        ) : null}
      </div>
    </ModalDialog>
  );
}
