import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  ArrowLeft,
  ArrowRight,
  BookOpenCheck,
  Brain,
  History,
  Pencil,
  Plus,
  RotateCcw,
  Save,
  Search,
  Settings2,
  Trash2,
} from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";

import {
  deactivateProjectMemory,
  deleteAssistantRunHistory,
  getAssistantGuide,
  listAssistantRuns,
  listProjectMemory,
  listProjectMemoryRevisions,
  type ProjectMemoryRecord,
  restoreAssistantGuide,
  restoreProjectMemoryRevision,
  saveAssistantGuide,
  saveProjectMemory,
  updateProjectMemory,
} from "~entities/assistant";
import { type DocumentNode, getProject } from "~entities/project";
import {
  type AssistantGuideDraft,
  clearAssistantGuideDraft,
  loadAssistantGuideDraft,
  saveAssistantGuideDraft,
} from "~shared/lib";
import {
  Banner,
  Button,
  Checkbox,
  Field,
  Input,
  Select,
  Spinner,
  useActionDialog,
  useToast,
} from "~shared/ui";
import { RequireSession } from "~widgets/require-session";

import {
  createEmptyMemoryDraft,
  filterProjectMemory,
  type MemoryKindFilter,
  type MemoryStatusFilter,
  memoryDraftFromRecord,
  memoryFieldsFromDraft,
  memoryKindLabel,
  memoryNodeTitle,
  memorySourceLabel,
  normalizeProjectMemoryRecord,
  toggleMemorySource,
} from "../model/memory";

export type AssistantContextView = "overview" | "guide" | "memory";

const assistantGuideTemplate = `# Голос и тон

- 

# Лицо, время и фокализация

- 

# Ритм, длина фраз и диалоги

- 

# Канон и приоритет источников

- Используй канонические написания из словаря проекта.

# Нежелательные слова, клише и приёмы

- 

# Положительные и отрицательные примеры

- Хорошо: 
- Не использовать: `;

export function AssistantContextPage({
  projectId,
  view = "overview",
}: {
  projectId: string;
  view?: AssistantContextView;
}) {
  const queryClient = useQueryClient();
  const { showToast } = useToast();
  const { requestConfirmation } = useActionDialog();
  const project = useQuery({
    queryKey: ["project", projectId],
    queryFn: () => getProject(projectId),
  });
  const guide = useQuery({
    queryKey: ["assistant-guide", projectId],
    queryFn: () => getAssistantGuide(projectId),
  });
  const memory = useQuery({
    queryKey: ["project-memory", projectId],
    queryFn: () => listProjectMemory(projectId, true),
  });
  const runs = useQuery({
    queryKey: ["assistant-runs", projectId],
    queryFn: () => listAssistantRuns(projectId),
  });
  const [guideContent, setGuideContent] = useState("");
  const [guideDraftRecovered, setGuideDraftRecovered] = useState(false);
  const [staleGuideDraft, setStaleGuideDraft] = useState<AssistantGuideDraft>();
  const [memoryDraft, setMemoryDraft] = useState(createEmptyMemoryDraft);
  const [editingMemory, setEditingMemory] = useState<ProjectMemoryRecord>();
  const [memoryQuery, setMemoryQuery] = useState("");
  const [memoryStatusFilter, setMemoryStatusFilter] = useState<MemoryStatusFilter>("all");
  const [memoryKindFilter, setMemoryKindFilter] = useState<MemoryKindFilter>("all");
  const [expandedMemoryId, setExpandedMemoryId] = useState<string>();
  useEffect(() => {
    const remoteContent = guide.data?.guide?.content ?? "";
    const remoteRevision = guide.data?.guide?.revision ?? 0;
    const draft = loadAssistantGuideDraft(projectId);
    if (!draft || draft.content === remoteContent) {
      if (draft?.content === remoteContent) clearAssistantGuideDraft(projectId);
      setGuideContent(remoteContent);
      setGuideDraftRecovered(false);
      setStaleGuideDraft(undefined);
      return;
    }
    if (draft.baseRevision === remoteRevision) {
      setGuideContent(draft.content);
      setGuideDraftRecovered(true);
      setStaleGuideDraft(undefined);
      return;
    }
    setGuideContent(remoteContent);
    setGuideDraftRecovered(false);
    setStaleGuideDraft(draft);
  }, [guide.data?.guide?.content, guide.data?.guide?.revision, projectId]);
  const updateGuideDraft = (content: string) => {
    const remoteContent = guide.data?.guide?.content ?? "";
    setGuideContent(content);
    setStaleGuideDraft(undefined);
    if (content === remoteContent) {
      clearAssistantGuideDraft(projectId);
      setGuideDraftRecovered(false);
      return;
    }
    try {
      saveAssistantGuideDraft(projectId, guide.data?.guide?.revision ?? 0, content);
      setGuideDraftRecovered(true);
    } catch {
      setGuideDraftRecovered(false);
    }
  };
  const discardGuideDraft = () => {
    clearAssistantGuideDraft(projectId);
    setGuideContent(guide.data?.guide?.content ?? "");
    setGuideDraftRecovered(false);
    setStaleGuideDraft(undefined);
  };
  const applyGuideTemplate = async () => {
    if (guideContent.trim() && guideContent !== (guide.data?.guide?.content ?? "")) {
      const confirmed = await requestConfirmation({
        title: "Заменить несохранённый черновик шаблоном?",
        description:
          "Текущий локальный черновик будет заменён. На сервере ничего не изменится, пока вы не сохраните revision.",
        confirmLabel: "Вставить шаблон",
        tone: "default",
        icon: BookOpenCheck,
      });
      if (!confirmed) return;
    }
    updateGuideDraft(assistantGuideTemplate);
  };
  const saveGuide = useMutation({
    mutationFn: () => saveAssistantGuide(projectId, guideContent, guide.data?.guide?.revision ?? 0),
    onSuccess: async () => {
      clearAssistantGuideDraft(projectId);
      setGuideDraftRecovered(false);
      setStaleGuideDraft(undefined);
      await queryClient.invalidateQueries({ queryKey: ["assistant-guide", projectId] });
      showToast({ title: "Руководство сохранено" });
    },
    onError: (error) =>
      showToast({ title: "Руководство не сохранено", description: error.message }),
  });
  const restoreGuide = useMutation({
    mutationFn: (revision: number) => restoreAssistantGuide(projectId, revision),
    onSuccess: () => {
      clearAssistantGuideDraft(projectId);
      setGuideDraftRecovered(false);
      setStaleGuideDraft(undefined);
      return queryClient.invalidateQueries({ queryKey: ["assistant-guide", projectId] });
    },
  });
  const resetMemoryEditor = () => {
    setMemoryDraft(createEmptyMemoryDraft());
    setEditingMemory(undefined);
  };
  const saveMemory = useMutation({
    mutationFn: () => {
      const fields = memoryFieldsFromDraft(memoryDraft);
      return editingMemory
        ? updateProjectMemory(projectId, { ...editingMemory, ...fields })
        : saveProjectMemory(projectId, fields);
    },
    onSuccess: async () => {
      const wasEditing = Boolean(editingMemory);
      resetMemoryEditor();
      await queryClient.invalidateQueries({ queryKey: ["project-memory", projectId] });
      showToast({
        title: wasEditing ? "Запись памяти обновлена" : "Факт добавлен в память проекта",
      });
    },
    onError: (error) => showToast({ title: "Память не сохранена", description: error.message }),
  });
  const deactivate = useMutation({
    mutationFn: (memoryId: string) => deactivateProjectMemory(projectId, memoryId),
    onSuccess: async (_, memoryId) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project-memory", projectId] }),
        queryClient.invalidateQueries({
          queryKey: ["project-memory-history", projectId, memoryId],
        }),
      ]);
    },
    onError: (error) =>
      showToast({ title: "Запись не деактивирована", description: error.message }),
  });
  const reactivate = useMutation({
    mutationFn: (record: ProjectMemoryRecord) =>
      updateProjectMemory(projectId, { ...record, active: true }),
    onSuccess: async (_, record) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project-memory", projectId] }),
        queryClient.invalidateQueries({
          queryKey: ["project-memory-history", projectId, record.id],
        }),
      ]);
    },
    onError: (error) => showToast({ title: "Запись не активирована", description: error.message }),
  });
  const restoreMemory = useMutation({
    mutationFn: ({ memoryId, revision }: { memoryId: string; revision: number }) =>
      restoreProjectMemoryRevision(projectId, memoryId, revision),
    onSuccess: async (_, variables) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project-memory", projectId] }),
        queryClient.invalidateQueries({
          queryKey: ["project-memory-history", projectId, variables.memoryId],
        }),
      ]);
      showToast({ title: `Версия r${variables.revision} восстановлена новой revision` });
    },
    onError: (error) =>
      showToast({ title: "Версия памяти не восстановлена", description: error.message }),
  });
  const deleteHistory = useMutation({
    mutationFn: (runId: string) => deleteAssistantRunHistory(runId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["assistant-runs", projectId] });
      showToast({
        title: "История сессии удалена",
        description: "Уже применённые изменения проекта сохранены.",
      });
    },
    onError: (error) => showToast({ title: "История не удалена", description: error.message }),
  });

  if (project.isPending || guide.isPending || memory.isPending || runs.isPending) {
    return <Spinner label="Открываем контекст ассистента…" />;
  }
  if (project.isError || !project.data) {
    return <Banner variant="danger" title="Проект недоступен" body="Обновите страницу." />;
  }
  const memoryRecords = (memory.data?.records ?? []).map(normalizeProjectMemoryRecord);
  const filteredMemoryRecords = filterProjectMemory(
    memoryRecords,
    memoryQuery,
    memoryStatusFilter,
    memoryKindFilter,
  );
  const projectNodes = project.data.nodes;

  return (
    <RequireSession>
      <div className="min-h-full bg-[var(--surface-app)]">
        <header className="flex h-14 items-center gap-3 border-b border-[var(--border-subtle)] bg-[var(--surface-panel)] px-4">
          <Button asChild variant="ghost" size="icon" aria-label="Назад в проект">
            <Link to="/projects/$projectId" params={{ projectId }}>
              <ArrowLeft className="size-4" aria-hidden="true" />
            </Link>
          </Button>
          <Brain className="size-5 text-[var(--accent)]" aria-hidden="true" />
          <p className="truncate text-sm font-semibold">
            {view === "guide"
              ? "Руководство ассистента"
              : view === "memory"
                ? "Память проекта"
                : "Контекст ассистента"}
            <span className="hidden text-[var(--ink-tertiary)] sm:inline">
              {" "}
              · {project.data.project.title}
            </span>
          </p>
          <Button asChild variant="secondary" size="compact" className="ml-auto">
            <Link to="/settings/assistant-providers">
              <Settings2 className="size-4" aria-hidden="true" />
              Провайдеры
            </Link>
          </Button>
        </header>
        <main className="mx-auto grid w-full max-w-[1180px] gap-6 px-4 py-8 lg:grid-cols-2 lg:px-8">
          <nav
            aria-label="Контекст ассистента"
            className="flex gap-2 overflow-x-auto rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-panel)] p-1 lg:col-span-2"
          >
            <AssistantContextLink
              active={view === "overview"}
              to="/projects/$projectId/assistant-context"
              projectId={projectId}
            >
              Обзор и история
            </AssistantContextLink>
            <AssistantContextLink
              active={view === "guide"}
              to="/projects/$projectId/assistant-guide"
              projectId={projectId}
            >
              ASSISTANT.md
            </AssistantContextLink>
            <AssistantContextLink
              active={view === "memory"}
              to="/projects/$projectId/memory"
              projectId={projectId}
            >
              Память проекта
            </AssistantContextLink>
          </nav>

          {view === "overview" ? (
            <>
              <ContextDestinationCard
                icon={BookOpenCheck}
                eyebrow={`Руководство · r${guide.data?.guide?.revision ?? 0}`}
                title="ASSISTANT.md"
                body="Отдельный проектный файл со стилем, голосом, запретами и правилами работы ассистента."
                action="Открыть руководство"
                to="/projects/$projectId/assistant-guide"
                projectId={projectId}
              />
              <ContextDestinationCard
                icon={Brain}
                eyebrow={`${memoryRecords.filter((record) => record.active).length} активных записей`}
                title="Память проекта"
                body="Канон, решения, правила мира и ограничения — отдельно от словаря и документов."
                action="Открыть память"
                to="/projects/$projectId/memory"
                projectId={projectId}
              />
            </>
          ) : null}

          {view === "guide" ? (
            <section className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-5 sm:p-7 lg:col-span-2">
              <div className="flex items-start gap-3">
                <BookOpenCheck className="mt-1 size-5 text-[var(--accent)]" aria-hidden="true" />
                <div>
                  <h1 className="font-display text-3xl">ASSISTANT.md</h1>
                  <p className="mt-2 text-sm leading-6 text-[var(--ink-secondary)]">
                    Стиль, тон, запреты и договорённости автора. Активная revision фиксируется при
                    подтверждении каждого поручения: сохранение здесь повлияет на следующий turn, но
                    не изменит уже выполняющийся.
                  </p>
                </div>
              </div>
              <label htmlFor="assistant-guide" className="mt-5 block text-sm font-medium">
                Руководство проекта · r{guide.data?.guide?.revision ?? 0}
              </label>
              <div className="mt-3 flex flex-wrap gap-2">
                <Button
                  type="button"
                  variant="secondary"
                  size="compact"
                  onClick={() => void applyGuideTemplate()}
                >
                  <BookOpenCheck className="size-3.5" aria-hidden="true" />
                  Вставить структурированный шаблон
                </Button>
                {guideContent !== (guide.data?.guide?.content ?? "") ? (
                  <Button type="button" variant="ghost" size="compact" onClick={discardGuideDraft}>
                    Отменить локальный черновик
                  </Button>
                ) : null}
              </div>
              {guideDraftRecovered ? (
                <Banner
                  className="mt-3"
                  variant="info"
                  title="Несохранённый черновик защищён локально"
                  body="Он восстановится после перезагрузки этого браузера. Ассистент увидит его только после «Сохранить revision»."
                />
              ) : null}
              {staleGuideDraft ? (
                <Banner
                  className="mt-3"
                  variant="warning"
                  title={`Есть черновик для r${staleGuideDraft.baseRevision}`}
                  body={`На сервере уже активна r${guide.data?.guide?.revision ?? 0}. Откройте старый черновик только если хотите вручную перенести его поверх новой revision.`}
                  action={
                    <div className="flex flex-wrap gap-2">
                      <Button
                        type="button"
                        variant="secondary"
                        size="compact"
                        onClick={() => updateGuideDraft(staleGuideDraft.content)}
                      >
                        Открыть черновик
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="compact"
                        onClick={discardGuideDraft}
                      >
                        Отбросить
                      </Button>
                    </div>
                  }
                />
              ) : null}
              <textarea
                id="assistant-guide"
                value={guideContent}
                onChange={(event) => updateGuideDraft(event.target.value)}
                maxLength={24_000}
                rows={18}
                placeholder={`# Голос и стиль\n\n- Пиши сдержанно, без лишних объяснений.\n- Используй канонические имена из словаря.\n- Не меняй финальную реплику без прямого поручения.`}
                className="font-mono mt-2 w-full resize-y rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-raised)] p-4 text-[13px] leading-6 outline-none focus:border-[var(--accent)]"
              />
              <div className="mt-3 flex items-center justify-between">
                <span className="text-[11px] text-[var(--ink-tertiary)]">
                  {guideContent.length.toLocaleString("ru")} / 24 000
                </span>
                <Button
                  onClick={() => saveGuide.mutate()}
                  disabled={
                    saveGuide.isPending || guideContent === (guide.data?.guide?.content ?? "")
                  }
                >
                  <Save className="size-4" aria-hidden="true" />
                  Сохранить revision
                </Button>
              </div>
              {(guide.data?.guide?.history?.length ?? 0) > 0 ? (
                <details className="mt-6 border-t border-[var(--border-subtle)] pt-4">
                  <summary className="flex cursor-pointer items-center gap-2 text-sm">
                    <History className="size-4" aria-hidden="true" />
                    История руководства
                  </summary>
                  <div className="mt-3 space-y-2">
                    {guide.data?.guide?.history?.map((revision) => (
                      <div
                        key={revision.id}
                        className="flex items-center gap-3 rounded-[var(--radius-md)] bg-[var(--surface-subtle)] px-3 py-2 text-xs"
                      >
                        <span>r{revision.revision}</span>
                        <span className="text-[var(--ink-tertiary)]">{revision.source}</span>
                        <Button
                          variant="ghost"
                          size="compact"
                          className="ml-auto"
                          disabled={revision.revision === guide.data?.guide?.revision}
                          onClick={() => restoreGuide.mutate(revision.revision)}
                        >
                          <RotateCcw className="size-3.5" aria-hidden="true" />
                          Вернуть
                        </Button>
                      </div>
                    ))}
                  </div>
                </details>
              ) : null}
            </section>
          ) : null}

          {view === "memory" ? (
            <section className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-5 sm:p-7 lg:col-span-2">
              <div className="flex flex-col gap-4 sm:flex-row sm:items-start">
                <Brain
                  className="mt-1 hidden size-5 shrink-0 text-[var(--accent)] sm:block"
                  aria-hidden="true"
                />
                <div className="min-w-0 flex-1">
                  <h2 className="font-display text-3xl">Память проекта</h2>
                  <p className="mt-2 text-sm leading-6 text-[var(--ink-secondary)]">
                    Канон, решения, правила мира и ограничения. Записи, предложенные ассистентом,
                    всегда проходят отдельную группу review.
                  </p>
                </div>
              </div>
              <form
                className="mt-5 rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-raised)] p-4"
                onSubmit={(event) => {
                  event.preventDefault();
                  saveMemory.mutate();
                }}
              >
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <h3 className="text-sm font-semibold">
                    {editingMemory
                      ? `Изменить запись · текущая r${editingMemory.revision}`
                      : "Добавить факт или решение"}
                  </h3>
                  {editingMemory ? (
                    <Button
                      type="button"
                      variant="ghost"
                      size="compact"
                      onClick={resetMemoryEditor}
                    >
                      Отмена
                    </Button>
                  ) : null}
                </div>
                <div className="mt-4 grid gap-3 sm:grid-cols-[180px_minmax(0,1fr)]">
                  <Field label="Тип">
                    <Select
                      value={memoryDraft.kind}
                      onChange={(event) =>
                        setMemoryDraft((current) => ({
                          ...current,
                          kind: event.target.value as ProjectMemoryRecord["kind"],
                        }))
                      }
                    >
                      <option value="fact">Факт</option>
                      <option value="character">Персонаж</option>
                      <option value="world_rule">Правило мира</option>
                      <option value="plot">Сюжет</option>
                      <option value="style">Стиль</option>
                      <option value="constraint">Ограничение</option>
                      <option value="decision">Решение</option>
                      <option value="other">Другое</option>
                    </Select>
                  </Field>
                  <Field label="Что нужно помнить">
                    <Input
                      value={memoryDraft.statement}
                      onChange={(event) =>
                        setMemoryDraft((current) => ({ ...current, statement: event.target.value }))
                      }
                      maxLength={4_000}
                      placeholder="Элина никогда не называет отца по имени."
                    />
                  </Field>
                  <Field label="Область действия">
                    <Select
                      value={memoryDraft.scope}
                      onChange={(event) =>
                        setMemoryDraft((current) => ({
                          ...current,
                          scope: event.target.value as ProjectMemoryRecord["scope"],
                          scopeNodeId: event.target.value === "project" ? "" : current.scopeNodeId,
                        }))
                      }
                    >
                      <option value="project">Весь проект</option>
                      <option value="node">Конкретный файл</option>
                    </Select>
                  </Field>
                  {memoryDraft.scope === "node" ? (
                    <Field label="Файл области">
                      <Select
                        required
                        value={memoryDraft.scopeNodeId}
                        onChange={(event) =>
                          setMemoryDraft((current) => ({
                            ...current,
                            scopeNodeId: event.target.value,
                          }))
                        }
                      >
                        <option value="">Выберите файл</option>
                        {projectNodes.map((node) => (
                          <option key={node.id} value={node.id}>
                            {node.title}
                          </option>
                        ))}
                      </Select>
                    </Field>
                  ) : (
                    <Field label="Теги" hint="Через запятую; по ним работает поиск.">
                      <Input
                        value={memoryDraft.tagsText}
                        onChange={(event) =>
                          setMemoryDraft((current) => ({
                            ...current,
                            tagsText: event.target.value,
                          }))
                        }
                        maxLength={1_000}
                        placeholder="Элина, канон, вода"
                      />
                    </Field>
                  )}
                  {memoryDraft.scope === "node" ? (
                    <Field
                      label="Теги"
                      hint="Через запятую; область ограничивает применение, теги помогают найти запись."
                      className="sm:col-span-2"
                    >
                      <Input
                        value={memoryDraft.tagsText}
                        onChange={(event) =>
                          setMemoryDraft((current) => ({
                            ...current,
                            tagsText: event.target.value,
                          }))
                        }
                        maxLength={1_000}
                        placeholder="Элина, канон, вода"
                      />
                    </Field>
                  ) : null}
                </div>
                <fieldset className="mt-4 rounded-[var(--radius-sm)] border border-[var(--border-subtle)] p-3">
                  <legend className="px-1 text-xs font-medium">Файлы-источники</legend>
                  <p className="mb-3 text-[11px] leading-5 text-[var(--ink-tertiary)]">
                    Источник объясняет, откуда взялся факт. Он не меняет область действия записи.
                  </p>
                  <div className="grid max-h-36 gap-2 overflow-y-auto sm:grid-cols-2">
                    {projectNodes.map((node) => {
                      const checked = memoryDraft.sourceReferences.some(
                        (reference) => reference.id === node.id,
                      );
                      const checkboxId = `memory-source-${node.id}`;
                      return (
                        <label
                          key={node.id}
                          htmlFor={checkboxId}
                          className="flex items-center gap-2 text-xs"
                        >
                          <Checkbox
                            id={checkboxId}
                            checked={checked}
                            onCheckedChange={() =>
                              setMemoryDraft((current) => ({
                                ...current,
                                sourceReferences: toggleMemorySource(
                                  current.sourceReferences,
                                  node,
                                ),
                              }))
                            }
                          />
                          <span className="truncate">{node.title}</span>
                        </label>
                      );
                    })}
                  </div>
                </fieldset>
                <label
                  className="mt-3 flex items-center gap-3 text-xs"
                  htmlFor="assistant-memory-always-relevant"
                >
                  <Checkbox
                    id="assistant-memory-always-relevant"
                    checked={memoryDraft.alwaysRelevant}
                    onCheckedChange={(checked) =>
                      setMemoryDraft((current) => ({
                        ...current,
                        alwaysRelevant: checked === true,
                      }))
                    }
                  />
                  Всегда включать в ограниченный prompt-контекст
                </label>
                <p className="mt-2 text-[11px] leading-5 text-[var(--ink-tertiary)]">
                  Используйте редко: такая запись расходует prompt-контекст в каждом поручении.
                </p>
                <div className="mt-4 flex flex-wrap gap-2">
                  <Button
                    type="submit"
                    variant="secondary"
                    disabled={
                      !memoryDraft.statement.trim() ||
                      (memoryDraft.scope === "node" && !memoryDraft.scopeNodeId) ||
                      saveMemory.isPending
                    }
                  >
                    {editingMemory ? (
                      <Save className="size-4" aria-hidden="true" />
                    ) : (
                      <Plus className="size-4" aria-hidden="true" />
                    )}
                    {editingMemory ? "Сохранить новую revision" : "Добавить в память"}
                  </Button>
                </div>
              </form>
              <div className="mt-5 grid gap-2 sm:grid-cols-[minmax(0,1fr)_170px_180px]">
                <div className="relative">
                  <Search
                    className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-[var(--ink-tertiary)]"
                    aria-hidden="true"
                  />
                  <Input
                    className="pl-10"
                    value={memoryQuery}
                    onChange={(event) => setMemoryQuery(event.target.value)}
                    placeholder="Факт, тег или источник"
                    aria-label="Поиск по памяти проекта"
                  />
                </div>
                <Select
                  value={memoryStatusFilter}
                  onChange={(event) =>
                    setMemoryStatusFilter(event.target.value as MemoryStatusFilter)
                  }
                  aria-label="Фильтр по состоянию памяти"
                >
                  <option value="all">Все состояния</option>
                  <option value="active">Активные</option>
                  <option value="inactive">Не используются</option>
                  <option value="always">Всегда в контексте</option>
                </Select>
                <Select
                  value={memoryKindFilter}
                  onChange={(event) => setMemoryKindFilter(event.target.value as MemoryKindFilter)}
                  aria-label="Фильтр по типу памяти"
                >
                  <option value="all">Все типы</option>
                  <option value="fact">Факты</option>
                  <option value="character">Персонажи</option>
                  <option value="world_rule">Правила мира</option>
                  <option value="plot">Сюжет</option>
                  <option value="style">Стиль</option>
                  <option value="constraint">Ограничения</option>
                  <option value="decision">Решения</option>
                  <option value="other">Другое</option>
                </Select>
              </div>
              <p className="mt-3 text-[11px] text-[var(--ink-tertiary)]" aria-live="polite">
                Показано {filteredMemoryRecords.length} из {memoryRecords.length}
              </p>
              <div className="mt-5 space-y-3">
                {filteredMemoryRecords.length ? (
                  filteredMemoryRecords.map((record) => (
                    <article
                      key={record.id}
                      className="rounded-[var(--radius-md)] border border-[var(--border-subtle)] p-4"
                    >
                      <div className="flex flex-wrap items-center gap-2 text-[9px] font-semibold tracking-wider text-[var(--accent)] uppercase">
                        {memoryKindLabel(record.kind)} · r{record.revision}
                        {!record.active ? (
                          <span className="rounded-full bg-[var(--surface-subtle)] px-2 py-1 text-[var(--ink-secondary)]">
                            не используется
                          </span>
                        ) : null}
                        {record.alwaysRelevant ? (
                          <span
                            className="rounded-full bg-[var(--accent-soft)] px-2 py-1 text-[var(--accent-ink)]"
                            title="Эта запись входит в ограниченный prompt-контекст каждого поручения"
                          >
                            всегда
                          </span>
                        ) : null}
                      </div>
                      <p className="mt-2 text-sm leading-6">{record.statement}</p>
                      <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-[10px] text-[var(--ink-tertiary)]">
                        <span>
                          Область:{" "}
                          {record.scope === "project"
                            ? "весь проект"
                            : record.scopeNodeId && (
                                <MemoryNodeLink
                                  projectId={projectId}
                                  nodeId={record.scopeNodeId}
                                  nodes={projectNodes}
                                  fallback="файл недоступен"
                                />
                              )}
                        </span>
                        <span>Источник: {memorySourceLabel(record.source)}</span>
                        <span>{new Date(record.updatedAt).toLocaleString("ru")}</span>
                      </div>
                      {record.tags.length ? (
                        <div className="mt-3 flex flex-wrap items-center gap-1.5">
                          <span className="text-[10px] text-[var(--ink-tertiary)]">Теги:</span>
                          {record.tags.map((tag) => (
                            <span
                              key={tag}
                              className="rounded-full bg-[var(--surface-subtle)] px-2 py-1 text-[10px] text-[var(--ink-secondary)]"
                            >
                              {tag}
                            </span>
                          ))}
                        </div>
                      ) : null}
                      {record.sourceReferences.length ? (
                        <div className="mt-3 text-[11px] leading-5 text-[var(--ink-secondary)]">
                          <p>Файлы-источники:</p>
                          <ul className="mt-1 space-y-1">
                            {record.sourceReferences.map((reference) => (
                              <li
                                key={`${reference.kind}:${reference.id}:${reference.start ?? ""}:${reference.end ?? ""}`}
                              >
                                <MemoryNodeLink
                                  projectId={projectId}
                                  nodeId={reference.id}
                                  nodes={projectNodes}
                                  fallback={reference.label || reference.id}
                                />
                                {reference.quote ? (
                                  <q className="ml-2 text-[var(--ink-tertiary)]">
                                    {reference.quote}
                                  </q>
                                ) : null}
                              </li>
                            ))}
                          </ul>
                        </div>
                      ) : null}
                      <div className="mt-3 flex flex-wrap gap-2">
                        <Button
                          variant="ghost"
                          size="compact"
                          onClick={() => {
                            setEditingMemory(record);
                            setMemoryDraft(memoryDraftFromRecord(record));
                            window.scrollTo({ top: 0, behavior: "smooth" });
                          }}
                        >
                          <Pencil className="size-3.5" aria-hidden="true" />
                          Изменить
                        </Button>
                        {record.active ? (
                          <Button
                            variant="ghost"
                            size="compact"
                            disabled={deactivate.isPending}
                            onClick={() => deactivate.mutate(record.id)}
                          >
                            Не использовать
                          </Button>
                        ) : (
                          <Button
                            variant="ghost"
                            size="compact"
                            disabled={reactivate.isPending}
                            onClick={() => reactivate.mutate(record)}
                          >
                            Использовать снова
                          </Button>
                        )}
                        <Button
                          variant="ghost"
                          size="compact"
                          aria-expanded={expandedMemoryId === record.id}
                          aria-controls={`memory-history-${record.id}`}
                          onClick={() =>
                            setExpandedMemoryId((current) =>
                              current === record.id ? undefined : record.id,
                            )
                          }
                        >
                          <History className="size-3.5" aria-hidden="true" />
                          История
                        </Button>
                      </div>
                      {expandedMemoryId === record.id ? (
                        <MemoryHistoryPanel
                          projectId={projectId}
                          record={record}
                          restoring={restoreMemory.isPending}
                          onRestore={(revision) =>
                            restoreMemory.mutate({ memoryId: record.id, revision })
                          }
                        />
                      ) : null}
                    </article>
                  ))
                ) : memoryRecords.length ? (
                  <Banner
                    variant="info"
                    title="Ничего не найдено"
                    body="Измените запрос или сбросьте фильтры памяти."
                    action={
                      <Button
                        type="button"
                        variant="secondary"
                        size="compact"
                        onClick={() => {
                          setMemoryQuery("");
                          setMemoryStatusFilter("all");
                          setMemoryKindFilter("all");
                        }}
                      >
                        Сбросить фильтры
                      </Button>
                    }
                  />
                ) : (
                  <Banner
                    variant="info"
                    title="Память пока пуста"
                    body="Добавьте первый факт вручную или скажите ассистенту: «занеси это в память»."
                  />
                )}
              </div>
            </section>
          ) : null}
          {view === "overview" ? (
            <section className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-5 sm:p-7 lg:col-span-2">
              <div className="flex items-start gap-3">
                <History className="mt-1 size-5 text-[var(--accent)]" aria-hidden="true" />
                <div>
                  <h2 className="font-display text-3xl">История ассистента</h2>
                  <p className="mt-2 text-sm leading-6 text-[var(--ink-secondary)]">
                    Сессии, поручения и техническая история хранятся отдельно от проекта. Удаление
                    записи не откатывает уже применённые правки, словарь или память.
                  </p>
                </div>
              </div>
              <div className="mt-5 space-y-3">
                {runs.data?.runs.length ? (
                  runs.data.runs.map((run) => (
                    <article
                      key={run.id}
                      className="flex flex-col gap-3 rounded-[var(--radius-md)] border border-[var(--border-subtle)] p-4 sm:flex-row sm:items-center"
                    >
                      <div className="min-w-0 flex-1">
                        <p className="text-[10px] font-semibold tracking-[0.12em] text-[var(--accent)] uppercase">
                          {assistantRunStatusLabel(run.status)} ·{" "}
                          {run.inputMode === "voice"
                            ? "голосом"
                            : run.inputMode === "mcp"
                              ? `внешний агент · ${run.agent?.clientName ?? "MCP"}`
                              : "чат"}
                        </p>
                        <p className="mt-1 truncate text-sm font-semibold">
                          {run.latestTurn?.instruction || "Сессия без подтверждённого поручения"}
                        </p>
                        <p className="mt-1 text-[11px] text-[var(--ink-tertiary)]">
                          {new Date(run.updatedAt).toLocaleString("ru")} ·{" "}
                          {run.origin === "remote_mcp"
                            ? `Внешний агент · ${run.agent?.clientName ?? run.provider.name}`
                            : `${run.provider.name} · ${run.provider.model}`}
                        </p>
                      </div>
                      <div className="flex gap-2">
                        <Button asChild variant="secondary" size="compact">
                          <Link
                            to="/projects/$projectId/assistant/$runId"
                            params={{ projectId, runId: run.id }}
                          >
                            Открыть
                          </Link>
                        </Button>
                        <Button
                          variant="ghost"
                          size="compact"
                          disabled={run.status === "working" || deleteHistory.isPending}
                          onClick={async () => {
                            const confirmed = await requestConfirmation({
                              title: "Удалить историю этой сессии?",
                              description:
                                "Поручения, ответы и неприменённые предложения исчезнут. Уже применённые изменения проекта останутся.",
                              confirmLabel: "Удалить историю",
                              tone: "danger",
                              icon: Trash2,
                            });
                            if (confirmed) deleteHistory.mutate(run.id);
                          }}
                        >
                          <Trash2 className="size-3.5" aria-hidden="true" />
                          Удалить историю
                        </Button>
                      </div>
                    </article>
                  ))
                ) : (
                  <Banner
                    variant="info"
                    title="Истории пока нет"
                    body="Новая сессия появится здесь после запуска ассистента."
                  />
                )}
              </div>
            </section>
          ) : null}
        </main>
      </div>
    </RequireSession>
  );
}

type AssistantContextRoute =
  | "/projects/$projectId/assistant-context"
  | "/projects/$projectId/assistant-guide"
  | "/projects/$projectId/memory";

function AssistantContextLink({
  active,
  to,
  projectId,
  children,
}: {
  active: boolean;
  to: AssistantContextRoute;
  projectId: string;
  children: ReactNode;
}) {
  return (
    <Link
      to={to}
      params={{ projectId }}
      aria-current={active ? "page" : undefined}
      className={`inline-flex min-h-10 shrink-0 items-center rounded-[var(--radius-sm)] px-4 text-sm font-medium ${
        active
          ? "bg-[var(--surface-paper)] text-[var(--accent-ink)] shadow-sm"
          : "text-[var(--ink-secondary)] hover:text-[var(--ink-primary)]"
      }`}
    >
      {children}
    </Link>
  );
}

function ContextDestinationCard({
  icon: Icon,
  eyebrow,
  title,
  body,
  action,
  to,
  projectId,
}: {
  icon: typeof Brain;
  eyebrow: string;
  title: string;
  body: string;
  action: string;
  to: AssistantContextRoute;
  projectId: string;
}) {
  return (
    <section className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-5 sm:p-7">
      <span className="grid size-11 place-items-center rounded-full bg-[var(--accent-soft)] text-[var(--accent)]">
        <Icon className="size-5" aria-hidden="true" />
      </span>
      <p className="mt-5 text-[10px] font-semibold tracking-[0.12em] text-[var(--accent)] uppercase">
        {eyebrow}
      </p>
      <h1 className="font-display mt-2 text-3xl">{title}</h1>
      <p className="mt-2 text-sm leading-6 text-[var(--ink-secondary)]">{body}</p>
      <Button asChild variant="secondary" className="mt-6">
        <Link to={to} params={{ projectId }}>
          {action}
          <ArrowRight className="size-4" aria-hidden="true" />
        </Link>
      </Button>
    </section>
  );
}

function MemoryHistoryPanel({
  projectId,
  record,
  restoring,
  onRestore,
}: {
  projectId: string;
  record: ProjectMemoryRecord;
  restoring: boolean;
  onRestore: (revision: number) => void;
}) {
  const history = useQuery({
    queryKey: ["project-memory-history", projectId, record.id],
    queryFn: () => listProjectMemoryRevisions(projectId, record.id),
  });
  return (
    <section
      id={`memory-history-${record.id}`}
      aria-label={`История записи «${record.statement}»`}
      className="mt-4 border-t border-[var(--border-subtle)] pt-4"
    >
      <h4 className="text-xs font-semibold">История без перезаписи прошлых версий</h4>
      {history.isPending ? <Spinner label="Открываем историю записи…" /> : null}
      {history.isError ? (
        <Banner
          className="mt-3"
          variant="danger"
          title="История недоступна"
          body={history.error.message}
        />
      ) : null}
      <div className="mt-3 space-y-2">
        {history.data?.revisions.map((revision) => (
          <div
            key={revision.id}
            className="rounded-[var(--radius-sm)] bg-[var(--surface-subtle)] px-3 py-2"
          >
            <div className="flex flex-wrap items-center gap-2 text-[10px] text-[var(--ink-tertiary)]">
              <span className="font-semibold text-[var(--ink-primary)]">r{revision.revision}</span>
              <span>{memorySourceLabel(revision.source)}</span>
              <span>{new Date(revision.createdAt).toLocaleString("ru")}</span>
              <Button
                type="button"
                variant="ghost"
                size="compact"
                className="ml-auto"
                disabled={revision.revision === record.revision || restoring}
                onClick={() => onRestore(revision.revision)}
              >
                <RotateCcw className="size-3.5" aria-hidden="true" />
                Восстановить r{revision.revision}
              </Button>
            </div>
            <p className="mt-1 text-xs leading-5">{revision.snapshot.statement}</p>
          </div>
        ))}
      </div>
    </section>
  );
}

function MemoryNodeLink({
  projectId,
  nodeId,
  nodes,
  fallback,
}: {
  projectId: string;
  nodeId: string;
  nodes: DocumentNode[];
  fallback: string;
}) {
  const title = memoryNodeTitle(nodes, nodeId);
  if (!title) return <span>{fallback}</span>;
  return (
    <Link
      to="/projects/$projectId/studio/$nodeId"
      params={{ projectId, nodeId }}
      className="underline decoration-[var(--border-strong)] underline-offset-2 hover:text-[var(--accent)]"
    >
      {title}
    </Link>
  );
}

function assistantRunStatusLabel(status: string): string {
  const labels: Record<string, string> = {
    active: "Можно продолжить",
    working: "Выполняется",
    awaiting_review: "Нужно проверить",
    completed: "Завершено",
    cancelled: "Остановлено",
    failed: "Требует внимания",
  };
  return labels[status] ?? "Сессия";
}
