import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import {
  ArrowLeft,
  Check,
  FileText,
  Link2,
  MessageCircle,
  Network,
  RotateCcw,
  ShieldCheck,
  X,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import {
  type AssistantChangeOperation,
  applyAssistantChangeSet,
  createAssistantTurn,
  getAssistantChangeSet,
  getAssistantRun,
  listAssistantTurns,
  refreshAssistantChangedDocuments,
  rejectAssistantChangeSet,
  undoAssistantChangeSet,
} from "~entities/assistant";
import { getProject } from "~entities/project";
import { AssistantMessage, Banner, Button, Checkbox, Spinner, useToast } from "~shared/ui";
import { RequireSession } from "~widgets/require-session";

import {
  annotationFixes,
  canvasGhostPreview,
  changeGroupLabel,
  exactTextEdits,
  operationLabel,
  resolvedTarget,
  reviewPrimaryAction,
  reviewStatusLabel,
  selectedChangeGroupDependents,
  snapshotText,
  withRequiredChangeGroups,
} from "../model/assistant-review-presentation";

interface AssistantReviewPageProps {
  projectId: string;
  runId: string;
  changeSetId: string;
  embedded?: boolean;
  onClose?: () => void;
}

export function AssistantReviewPage({
  projectId,
  runId,
  changeSetId,
  embedded = false,
  onClose,
}: AssistantReviewPageProps) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { showToast } = useToast();
  const changeSet = useQuery({
    queryKey: ["assistant-change-set", runId, changeSetId],
    queryFn: () => getAssistantChangeSet(runId, changeSetId),
  });
  const run = useQuery({
    queryKey: ["assistant-run", runId],
    queryFn: () => getAssistantRun(runId),
  });
  const project = useQuery({
    queryKey: ["project", projectId],
    queryFn: () => getProject(projectId),
  });
  const turns = useQuery({
    queryKey: ["assistant-turns", runId],
    queryFn: () => listAssistantTurns(runId),
  });
  const returnToSession = () => {
    if (onClose) {
      onClose();
      return;
    }
    void navigate({
      to: "/projects/$projectId/assistant/$runId",
      params: { projectId, runId },
    });
  };
  const groups = useMemo(
    () =>
      Array.from(
        new Set(changeSet.data?.changeSet.operations.map((operation) => operation.groupKey) ?? []),
      ),
    [changeSet.data?.changeSet.operations],
  );
  const [selectedGroups, setSelectedGroups] = useState<Set<string>>(new Set());
  const [annotationResolutions, setAnnotationResolutions] = useState<Record<string, boolean>>({});
  const [requestingChanges, setRequestingChanges] = useState(false);
  const [changeRequest, setChangeRequest] = useState("");
  const changeRequestRef = useRef<HTMLTextAreaElement>(null);
  const dialogRef = useRef<HTMLElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const returnFocusRef = useRef<HTMLElement | null>(null);
  useEffect(() => {
    if (groups.length > 0 && selectedGroups.size === 0) {
      setSelectedGroups(new Set(groups));
    }
  }, [groups, selectedGroups.size]);
  useEffect(() => {
    const defaults = Object.fromEntries(
      (changeSet.data?.changeSet.operations ?? []).flatMap((operation) =>
        annotationFixes(operation).map((fix) => [fix.annotationId, fix.resolve]),
      ),
    );
    setAnnotationResolutions(defaults);
  }, [changeSet.data?.changeSet.operations]);
  useEffect(() => {
    if (requestingChanges) {
      changeRequestRef.current?.focus();
    }
  }, [requestingChanges]);
  useEffect(() => {
    if (!embedded) return;
    returnFocusRef.current = document.activeElement as HTMLElement | null;
    window.requestAnimationFrame(() => closeButtonRef.current?.focus());
    const maintainDialogFocus = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose?.();
        return;
      }
      if (event.key !== "Tab" || !dialogRef.current) return;
      const controls = Array.from(
        dialogRef.current.querySelectorAll<HTMLElement>(
          'button:not([disabled]), input:not([disabled]), textarea:not([disabled]), select:not([disabled]), a[href], [tabindex]:not([tabindex="-1"])',
        ),
      ).filter((element) => element.offsetParent !== null);
      if (controls.length === 0) return;
      const first = controls[0];
      const last = controls.at(-1);
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last?.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first?.focus();
      }
    };
    window.addEventListener("keydown", maintainDialogFocus);
    return () => {
      window.removeEventListener("keydown", maintainDialogFocus);
      window.requestAnimationFrame(() => returnFocusRef.current?.focus());
    };
  }, [embedded, onClose]);
  const apply = useMutation({
    mutationFn: () => {
      const selectedAnnotationIds = new Set(
        (changeSet.data?.changeSet.operations ?? [])
          .filter((operation) => selectedGroups.has(operation.groupKey))
          .flatMap((operation) => annotationFixes(operation).map((fix) => fix.annotationId)),
      );
      return applyAssistantChangeSet(
        runId,
        changeSetId,
        [...selectedGroups],
        Object.fromEntries(
          Object.entries(annotationResolutions).filter(([annotationId]) =>
            selectedAnnotationIds.has(annotationId),
          ),
        ),
      );
    },
    onSuccess: async ({ changeSet: appliedChangeSet }) => {
      await refreshAssistantChangedDocuments(queryClient, projectId, appliedChangeSet);
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: ["assistant-change-set", runId, changeSetId],
        }),
        queryClient.invalidateQueries({ queryKey: ["assistant-run", runId] }),
      ]);
      showToast({
        title:
          selectedGroups.size === groups.length
            ? "Изменения применены"
            : "Выбранные группы применены",
        description: "Весь набор можно отменить одной кнопкой.",
      });
    },
    onError: (error) => showToast({ title: "Изменения не применены", description: error.message }),
  });
  const reject = useMutation({
    mutationFn: () => rejectAssistantChangeSet(runId, changeSetId),
    onSuccess: returnToSession,
  });
  const requestChanges = useMutation({
    mutationFn: async (feedback: string) => {
      await rejectAssistantChangeSet(runId, changeSetId);
      return createAssistantTurn(runId, {
        instruction:
          "Исправь предыдущее предложение по замечаниям автора. " +
          `Замечания: ${feedback.trim()}. ` +
          "Подготовь новый полный change set через инструменты; ничего не применяй автоматически.",
        inputMode: "chat",
        requireConfirmation: false,
        execute: true,
      });
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["assistant-run", runId] }),
        queryClient.invalidateQueries({ queryKey: ["assistant-turns", runId] }),
      ]);
      showToast({ title: "Замечания отправлены", description: "Ассистент готовит новую версию." });
      returnToSession();
    },
    onError: (error) =>
      showToast({ title: "Не удалось запросить исправление", description: error.message }),
  });
  const undo = useMutation({
    mutationFn: () => undoAssistantChangeSet(runId, changeSetId),
    onSuccess: async ({ changeSet: undoneChangeSet }) => {
      await refreshAssistantChangedDocuments(queryClient, projectId, undoneChangeSet);
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: ["assistant-change-set", runId, changeSetId],
        }),
      ]);
      showToast({ title: "Набор изменений отменён" });
    },
  });

  if (changeSet.isPending || run.isPending || project.isPending || turns.isPending) {
    return <Spinner label="Готовим точный diff…" />;
  }
  if (changeSet.isError || !changeSet.data || run.isError || project.isError || turns.isError) {
    return (
      <Banner
        variant="danger"
        title="Предложение недоступно"
        body="Откройте сессию и повторите поручение."
      />
    );
  }
  const value = changeSet.data.changeSet;
  const staged = value.state === "staged";
  const applied = value.state === "applied";
  const primaryAction = reviewPrimaryAction(value.state, requestingChanges);

  if (embedded) {
    return (
      <RequireSession>
        <div className="fixed inset-0 z-50 grid place-items-center p-4">
          <button
            type="button"
            aria-label="Закрыть проверку изменения"
            className="absolute inset-0 cursor-default bg-[var(--ink-primary)]/25"
            onClick={returnToSession}
          />
          <section
            ref={dialogRef}
            role="dialog"
            aria-modal="true"
            aria-labelledby="assistant-review-title"
            data-testid="assistant-review-overlay"
            className="relative z-10 flex h-[min(824px,calc(100vh-32px))] w-[min(1120px,calc(100vw-32px))] flex-col overflow-hidden rounded-[16px] border border-[var(--border-strong)] bg-[var(--surface-raised)] shadow-[0_18px_48px_var(--shadow-color)]"
          >
            <header className="flex h-16 shrink-0 items-center gap-3 border-b border-[var(--border-subtle)] px-5 sm:px-6">
              <div className="min-w-0 flex-1">
                <p className="text-[9px] font-semibold tracking-[0.14em] text-[var(--accent)] uppercase">
                  {reviewStatusLabel(value.state)} · выбрано {selectedGroups.size} из{" "}
                  {groups.length}
                </p>
                <h1 id="assistant-review-title" className="font-display mt-0.5 truncate text-xl">
                  {value.title}
                </h1>
              </div>
              <span className="rounded-full bg-[var(--surface-subtle)] px-3 py-1 text-[11px]">
                {reviewStatusLabel(value.state)}
              </span>
              <Button
                ref={closeButtonRef}
                type="button"
                variant="ghost"
                size="icon"
                aria-label="Закрыть проверку"
                onClick={returnToSession}
              >
                <X className="size-4" aria-hidden="true" />
              </Button>
            </header>

            <main className="grid min-h-0 flex-1 overflow-y-auto p-4 sm:p-6 lg:grid-cols-[minmax(0,1fr)_300px] lg:gap-5 lg:overflow-hidden">
              <section className="min-w-0 lg:overflow-y-auto lg:pr-1" aria-label="Точный diff">
                <div className="mb-4 flex items-start gap-3 rounded-[var(--radius-md)] bg-[var(--info-soft)] p-3">
                  <ShieldCheck
                    className="mt-0.5 size-4 shrink-0 text-[var(--info)]"
                    aria-hidden="true"
                  />
                  <div>
                    <p className="text-[14px] font-semibold">
                      {selectedGroups.size} из {groups.length} групп · {value.operations.length}{" "}
                      точных изменений
                    </p>
                    <p className="mt-0.5 text-[11px] text-[var(--ink-secondary)]">
                      Рукопись, autosave и история пока не изменены
                    </p>
                  </div>
                </div>
                <div className="space-y-4">
                  {value.operations.map((operation) => (
                    <OperationCard
                      key={operation.id}
                      operation={operation}
                      excluded={!selectedGroups.has(operation.groupKey)}
                      editable={staged}
                      annotationResolutions={annotationResolutions}
                      onAnnotationResolutionChange={(annotationId, resolve) =>
                        setAnnotationResolutions((current) => ({
                          ...current,
                          [annotationId]: resolve,
                        }))
                      }
                    />
                  ))}
                </div>
              </section>

              <aside className="mt-5 space-y-3 border-t border-[var(--border-subtle)] pt-5 lg:mt-0 lg:overflow-y-auto lg:border-t-0 lg:pt-0">
                <div className="rounded-[var(--radius-md)] bg-[var(--accent-soft)] p-4">
                  <p className="text-[9px] font-semibold tracking-[0.14em] text-[var(--accent)] uppercase">
                    Поручение
                  </p>
                  <p className="mt-2 whitespace-pre-wrap text-[14px] leading-[1.45]">
                    {turns.data?.turns.at(-1)?.instruction ?? value.summary}
                  </p>
                </div>
                <div className="rounded-[var(--radius-md)] bg-[var(--surface-subtle)] p-4">
                  <p className="text-[9px] font-semibold tracking-[0.14em] text-[var(--ink-tertiary)] uppercase">
                    Группы изменений
                  </p>
                  <div className="mt-3 space-y-2">
                    {groups.map((group, groupIndex) => {
                      const selected = selectedGroups.has(group);
                      const requiredBy = selectedChangeGroupDependents(
                        value.operations,
                        group,
                        selectedGroups,
                      );
                      const controlId = `assistant-modal-change-group-${groupIndex}`;
                      return (
                        <label
                          key={group}
                          htmlFor={controlId}
                          className={`flex items-start gap-2 rounded-[var(--radius-sm)] border p-2 text-[12px] ${
                            selected
                              ? "border-transparent bg-[var(--surface-paper)]"
                              : "border-dashed border-[var(--border-strong)] bg-transparent"
                          }`}
                        >
                          <Checkbox
                            id={controlId}
                            checked={selected}
                            disabled={!staged || (selected && requiredBy.length > 0)}
                            onCheckedChange={(checked) =>
                              setSelectedGroups((current) => {
                                if (checked) {
                                  return withRequiredChangeGroups(value.operations, [
                                    ...current,
                                    group,
                                  ]);
                                }
                                const next = new Set(current);
                                next.delete(group);
                                return next;
                              })
                            }
                          />
                          <span>
                            <span className="block">
                              {changeGroupLabel(value.operations, group)}
                            </span>
                            {!selected ? (
                              <span className="mt-0.5 block text-[10px] text-[var(--ink-tertiary)]">
                                Исключена из применения
                              </span>
                            ) : requiredBy.length > 0 ? (
                              <span className="mt-0.5 block text-[10px] text-[var(--ink-tertiary)]">
                                Обязательна для:{" "}
                                {requiredBy
                                  .map((item) => changeGroupLabel(value.operations, item))
                                  .join(", ")}
                              </span>
                            ) : null}
                          </span>
                        </label>
                      );
                    })}
                  </div>
                </div>
                <div className="rounded-[var(--radius-md)] bg-[var(--info-soft)] p-4">
                  <p className="text-[13px] font-semibold">Можно применить без смены экрана</p>
                  <p className="mt-1 text-[11px] leading-5 text-[var(--ink-secondary)]">
                    После применения весь набор останется отменяемым в Studio.
                  </p>
                </div>
                <details className="rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-3">
                  <summary className="cursor-pointer text-[12px] font-semibold">
                    Полный контекст сессии
                  </summary>
                  <div className="mt-3 space-y-3">
                    {(turns.data?.turns ?? []).map((turn) => (
                      <div key={turn.id} className="space-y-2">
                        <AssistantMessage>{turn.instruction}</AssistantMessage>
                        {turn.assistantText ? (
                          <AssistantMessage>{turn.assistantText}</AssistantMessage>
                        ) : null}
                      </div>
                    ))}
                  </div>
                </details>
                <details className="rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-3">
                  <summary className="cursor-pointer text-[12px] font-semibold">
                    Трассировка и JSON
                  </summary>
                  <div className="mt-3">
                    <AssistantMessage>
                      {JSON.stringify(
                        value.operations.map((operation) => ({
                          tool: operation.toolName,
                          arguments: operation.arguments,
                          trace: operation.trace,
                          proposedSnapshot: operation.proposedSnapshot,
                        })),
                      )}
                    </AssistantMessage>
                  </div>
                </details>
              </aside>
            </main>

            <footer className="shrink-0 border-t border-[var(--border-subtle)] bg-[var(--surface-panel)] px-4 py-3 sm:px-6">
              {requestingChanges && staged ? (
                <form
                  className="flex flex-col gap-2 sm:flex-row"
                  onSubmit={(event) => {
                    event.preventDefault();
                    if (changeRequest.trim()) requestChanges.mutate(changeRequest);
                  }}
                >
                  <label htmlFor="assistant-modal-change-request" className="sr-only">
                    Что нужно исправить в предложении?
                  </label>
                  <textarea
                    id="assistant-modal-change-request"
                    ref={changeRequestRef}
                    value={changeRequest}
                    onChange={(event) => setChangeRequest(event.target.value)}
                    rows={2}
                    maxLength={8_000}
                    placeholder="Что нужно исправить?"
                    className="min-h-11 flex-1 resize-none rounded-[var(--radius-sm)] border border-[var(--border-strong)] bg-[var(--surface-raised)] px-3 py-2 text-[13px] outline-none focus:border-[var(--focus)]"
                  />
                  <div className="flex shrink-0 items-end gap-2">
                    <Button
                      type="button"
                      variant="ghost"
                      onClick={() => setRequestingChanges(false)}
                      disabled={requestChanges.isPending}
                    >
                      Отмена
                    </Button>
                    <Button
                      type="submit"
                      disabled={!changeRequest.trim() || requestChanges.isPending}
                    >
                      {requestChanges.isPending ? "Отправляем…" : primaryAction.label}
                    </Button>
                  </div>
                </form>
              ) : (
                <div className="flex flex-wrap items-center justify-end gap-2">
                  {staged ? (
                    <>
                      <Button
                        variant="ghost"
                        onClick={() => reject.mutate()}
                        disabled={reject.isPending}
                      >
                        <X className="size-4" aria-hidden="true" />
                        Отклонить
                      </Button>
                      <Button
                        variant="secondary"
                        onClick={() => setRequestingChanges(true)}
                        disabled={requestChanges.isPending}
                      >
                        <MessageCircle className="size-4" aria-hidden="true" />
                        Попросить исправить
                      </Button>
                      <Button
                        onClick={() => apply.mutate()}
                        disabled={selectedGroups.size === 0 || apply.isPending}
                      >
                        <Check className="size-4" aria-hidden="true" />
                        {apply.isPending ? "Применяем…" : primaryAction.label}
                      </Button>
                    </>
                  ) : applied ? (
                    <Button onClick={() => undo.mutate()} disabled={undo.isPending}>
                      <RotateCcw className="size-4" aria-hidden="true" />
                      {primaryAction.label}
                    </Button>
                  ) : (
                    <Button onClick={returnToSession}>{primaryAction.label}</Button>
                  )}
                </div>
              )}
            </footer>
          </section>
        </div>
      </RequireSession>
    );
  }

  return (
    <RequireSession>
      <div
        className={`${
          embedded ? "fixed inset-0 z-50" : "min-h-full"
        } flex flex-col overflow-hidden bg-[var(--surface-app)]`}
        data-testid={embedded ? "assistant-review-overlay" : undefined}
      >
        <header className="flex h-14 items-center border-b border-[var(--border-subtle)] bg-[var(--surface-panel)] px-4">
          {embedded ? (
            <Button
              variant="ghost"
              size="icon"
              aria-label="Назад к сессии"
              onClick={returnToSession}
            >
              <ArrowLeft className="size-4" aria-hidden="true" />
            </Button>
          ) : (
            <Button asChild variant="ghost" size="icon" aria-label="Назад к сессии">
              <Link to="/projects/$projectId/assistant/$runId" params={{ projectId, runId }}>
                <ArrowLeft className="size-4" aria-hidden="true" />
              </Link>
            </Button>
          )}
          <p className="ml-2 min-w-0 truncate text-sm font-semibold">{value.title}</p>
          <span className="ml-3 hidden text-[11px] text-[var(--ink-tertiary)] sm:inline">
            Выбрано {selectedGroups.size} из {groups.length}
          </span>
          <span className="ml-auto rounded-full bg-[var(--surface-subtle)] px-3 py-1 text-[11px]">
            {reviewStatusLabel(value.state)}
          </span>
        </header>
        <main className="grid min-h-0 flex-1 overflow-y-auto xl:grid-cols-[280px_minmax(0,1fr)_360px] xl:overflow-hidden">
          <aside className="border-r border-[var(--border-subtle)] bg-[var(--surface-panel)] p-5">
            <p className="text-[10px] font-semibold tracking-[0.14em] text-[var(--accent)] uppercase">
              Набор изменений
            </p>
            <h1 className="font-display mt-4 text-3xl">{value.title}</h1>
            <p className="mt-2 text-sm leading-6 text-[var(--ink-secondary)]">{value.summary}</p>
            <Banner
              className="mt-5"
              variant={staged ? "info" : applied ? "success" : "warning"}
              title={
                staged
                  ? "Сначала проверьте каждую группу"
                  : applied
                    ? "Изменения записаны"
                    : "Набор не активен"
              }
              body={
                staged
                  ? "Независимые группы можно исключить. Для связанных групп интерфейс сохранит обязательные условия."
                  : applied
                    ? "Применённые операции связаны одним operation ID и отменяются вместе."
                    : "Канонический проект соответствует состоянию, указанному выше."
              }
            />
            <h2 className="mt-6 text-[10px] font-semibold tracking-[0.14em] text-[var(--ink-tertiary)] uppercase">
              Группы изменений
            </h2>
            <div className="mt-2 space-y-2">
              {groups.map((group, groupIndex) => {
                const count = value.operations.filter(
                  (operation) => operation.groupKey === group,
                ).length;
                const selected = selectedGroups.has(group);
                const requiredBy = selectedChangeGroupDependents(
                  value.operations,
                  group,
                  selectedGroups,
                );
                const controlId = `assistant-change-group-${groupIndex}`;
                const label = changeGroupLabel(value.operations, group);
                return (
                  <label
                    key={group}
                    htmlFor={controlId}
                    className={`flex cursor-pointer items-center gap-3 rounded-[var(--radius-md)] border p-3 ${
                      selected
                        ? "border-[var(--border-subtle)] bg-[var(--surface-paper)]"
                        : "border-dashed border-[var(--border-strong)] bg-transparent"
                    }`}
                  >
                    <Checkbox
                      id={controlId}
                      checked={selected}
                      disabled={!staged || (selected && requiredBy.length > 0)}
                      onCheckedChange={(checked) =>
                        setSelectedGroups((current) => {
                          if (checked) {
                            return withRequiredChangeGroups(value.operations, [...current, group]);
                          }
                          const next = new Set(current);
                          next.delete(group);
                          return next;
                        })
                      }
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block text-sm">{label}</span>
                      {!selected ? (
                        <span className="mt-1 block text-[10px] text-[var(--ink-tertiary)]">
                          Исключена из применения
                        </span>
                      ) : requiredBy.length > 0 ? (
                        <span className="mt-1 block text-[10px] text-[var(--ink-tertiary)]">
                          Требуется для:{" "}
                          {requiredBy
                            .map((dependent) => changeGroupLabel(value.operations, dependent))
                            .join(", ")}
                        </span>
                      ) : null}
                    </span>
                    <span className="grid size-7 place-items-center rounded-full bg-[var(--surface-subtle)] text-[11px]">
                      {count}
                    </span>
                  </label>
                );
              })}
            </div>
            <div className="mt-6 rounded-[var(--radius-md)] bg-[var(--surface-subtle)] p-3 text-[11px] leading-5 text-[var(--ink-secondary)]">
              <p className="flex items-center gap-2 font-semibold text-[var(--ink-primary)]">
                <Link2 className="size-3.5" aria-hidden="true" />
                Связь поручения и результата
              </p>
              <p className="mt-2">
                Каждая карточка показывает действие, точную цель и защищённые диапазоны. Связанные
                группы включают необходимые условия автоматически.
              </p>
            </div>
          </aside>
          <section className="min-w-0 p-4 pb-28 sm:p-8 sm:pb-28 xl:overflow-y-auto">
            <div className="mx-auto max-w-[920px]">
              <p className="text-[10px] font-semibold tracking-[0.14em] text-[var(--accent)] uppercase">
                {project.data?.project.structure.rootLabel} · предложенная редактура
              </p>
              <h2 className="font-display mt-3 text-3xl">
                Проверьте цель, ограничения и результат
              </h2>
              <div className="mt-6 space-y-5">
                {value.operations.map((operation) => (
                  <OperationCard
                    key={operation.id}
                    operation={operation}
                    excluded={!selectedGroups.has(operation.groupKey)}
                    editable={staged}
                    annotationResolutions={annotationResolutions}
                    onAnnotationResolutionChange={(annotationId, resolve) =>
                      setAnnotationResolutions((current) => ({
                        ...current,
                        [annotationId]: resolve,
                      }))
                    }
                  />
                ))}
              </div>
            </div>
          </section>
          <aside className="border-t border-[var(--border-subtle)] bg-[var(--surface-panel)] p-5 pb-28 xl:overflow-y-auto xl:border-t-0 xl:border-l">
            <p className="text-[10px] font-semibold tracking-[0.14em] text-[var(--accent)] uppercase">
              Дополнительные материалы
            </p>
            <h2 className="font-display mt-3 text-2xl">Поручение и ход работы</h2>
            <details className="mt-5 rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-3">
              <summary className="cursor-pointer text-sm font-semibold">
                Полный контекст сессии
              </summary>
              <ol className="mt-4 space-y-4" aria-label="Стенограмма сессии ассистента">
                {(turns.data?.turns ?? []).map((turn) => (
                  <li key={turn.id} className="space-y-2">
                    <AssistantMessage compact>{turn.instruction}</AssistantMessage>
                    {turn.assistantText ? (
                      <AssistantMessage compact>{turn.assistantText}</AssistantMessage>
                    ) : null}
                  </li>
                ))}
              </ol>
            </details>
            <details className="mt-3 rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-3">
              <summary className="cursor-pointer text-sm font-semibold">
                Трассировка и сырые данные
              </summary>
              <div className="mt-4 space-y-3">
                {value.operations.map((operation) => {
                  const target = resolvedTarget(operation);
                  return (
                    <div key={operation.id} className="text-[11px] text-[var(--ink-secondary)]">
                      <p className="font-semibold text-[var(--ink-primary)]">
                        {operationLabel(operation.toolName)}
                        {target ? ` · ${target.ranges.length} диапазонов` : ""}
                      </p>
                      <AssistantMessage compact>
                        {JSON.stringify({
                          arguments: operation.arguments,
                          trace: operation.trace,
                          proposedSnapshot: operation.proposedSnapshot,
                        })}
                      </AssistantMessage>
                    </div>
                  );
                })}
              </div>
            </details>
          </aside>
        </main>
        <footer className="fixed right-0 bottom-0 left-0 z-[60] border-t border-[var(--border-subtle)] bg-[var(--surface-panel)]/95 px-4 py-3 backdrop-blur xl:right-[360px] xl:left-[280px]">
          {staged ? (
            <div className="ml-auto max-w-[920px]">
              {requestingChanges ? (
                <form
                  className="mb-3 rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-3"
                  onSubmit={(event) => {
                    event.preventDefault();
                    if (changeRequest.trim()) requestChanges.mutate(changeRequest);
                  }}
                >
                  <label htmlFor="assistant-change-request" className="text-sm font-semibold">
                    Что нужно исправить в предложении?
                  </label>
                  <textarea
                    id="assistant-change-request"
                    ref={changeRequestRef}
                    value={changeRequest}
                    onChange={(event) => setChangeRequest(event.target.value)}
                    rows={2}
                    maxLength={8_000}
                    placeholder="Например: сократи первый абзац и не меняй последнюю реплику"
                    className="mt-2 w-full resize-none rounded-[var(--radius-sm)] border border-[var(--border-strong)] bg-[var(--surface-raised)] px-3 py-2 text-sm leading-5 outline-none focus:border-[var(--focus)] focus:outline-2 focus:outline-offset-1 focus:outline-[var(--focus)]"
                  />
                  <div className="mt-2 flex justify-end gap-2">
                    <Button
                      type="button"
                      variant="ghost"
                      onClick={() => setRequestingChanges(false)}
                      disabled={requestChanges.isPending}
                    >
                      Отмена
                    </Button>
                    <Button
                      type="submit"
                      disabled={!changeRequest.trim() || requestChanges.isPending}
                    >
                      {requestChanges.isPending ? "Отправляем…" : primaryAction.label}
                    </Button>
                  </div>
                </form>
              ) : null}
              {!requestingChanges ? (
                <div className="flex flex-wrap items-center justify-end gap-2">
                  <Button
                    variant="ghost"
                    onClick={() => reject.mutate()}
                    disabled={reject.isPending}
                  >
                    <X className="size-4" aria-hidden="true" />
                    Отклонить
                  </Button>
                  <Button
                    variant="secondary"
                    onClick={() => setRequestingChanges(true)}
                    disabled={reject.isPending || requestChanges.isPending}
                  >
                    <MessageCircle className="size-4" aria-hidden="true" />
                    Попросить исправить
                  </Button>
                  <Button
                    onClick={() => apply.mutate()}
                    disabled={selectedGroups.size === 0 || apply.isPending}
                  >
                    <Check className="size-4" aria-hidden="true" />
                    {apply.isPending ? "Применяем…" : primaryAction.label}
                  </Button>
                </div>
              ) : null}
            </div>
          ) : applied ? (
            <div className="flex justify-end">
              <Button onClick={() => undo.mutate()} disabled={undo.isPending}>
                <RotateCcw className="size-4" aria-hidden="true" />
                {primaryAction.label}
              </Button>
            </div>
          ) : (
            <div className="flex justify-end">
              {embedded ? (
                <Button onClick={returnToSession}>{primaryAction.label}</Button>
              ) : (
                <Button asChild>
                  <Link to="/projects/$projectId/assistant/$runId" params={{ projectId, runId }}>
                    {primaryAction.label}
                  </Link>
                </Button>
              )}
            </div>
          )}
        </footer>
      </div>
    </RequireSession>
  );
}

function OperationCard({
  operation,
  excluded,
  editable,
  annotationResolutions,
  onAnnotationResolutionChange,
}: {
  operation: AssistantChangeOperation;
  excluded: boolean;
  editable: boolean;
  annotationResolutions: Record<string, boolean>;
  onAnnotationResolutionChange: (annotationId: string, resolve: boolean) => void;
}) {
  const target = resolvedTarget(operation);
  const before = snapshotText(operation.beforeSnapshot?.content ?? operation.beforeSnapshot);
  const after = snapshotText(operation.proposedSnapshot);
  const edits = exactTextEdits(operation);
  const noteFixes = annotationFixes(operation);
  const canvasPreview = canvasGhostPreview(operation.beforeSnapshot, operation.proposedSnapshot);
  const targetTitle = operation.beforeSnapshot?.title;
  return (
    <article
      className={`rounded-[var(--radius-lg)] border bg-[var(--surface-paper)] p-5 ${
        excluded
          ? "border-dashed border-[var(--border-strong)] opacity-60"
          : "border-[var(--border-subtle)]"
      }`}
    >
      <div className="flex flex-wrap items-center gap-2">
        <span className="grid size-9 place-items-center rounded bg-[var(--accent-soft)] text-[var(--accent)]">
          {canvasPreview ? (
            <Network className="size-4" aria-hidden="true" />
          ) : (
            <FileText className="size-4" aria-hidden="true" />
          )}
        </span>
        <div className="min-w-0 flex-1">
          <h3 className="font-semibold">{changeGroupLabel([operation], operation.groupKey)}</h3>
          <p className="mt-1 text-[11px] text-[var(--ink-tertiary)]">
            {operationLabel(operation.toolName)}
            {targetTitle ? ` · ${targetTitle}` : ""} · исходная версия{" "}
            {operation.baseRevision ?? "—"}
          </p>
        </div>
        {excluded ? (
          <span className="rounded-full bg-[var(--surface-subtle)] px-3 py-1 text-[10px]">
            Не применять
          </span>
        ) : null}
      </div>
      {target ? (
        <div className="mt-4 rounded-[var(--radius-md)] bg-[var(--info-soft)] p-3 text-[11px] leading-5">
          <p className="flex items-center gap-2 font-semibold">
            <ShieldCheck className="size-4 text-[var(--info)]" aria-hidden="true" />
            Точная цель: {target.sourcePhrase}
          </p>
          <p className="mt-1 text-[var(--ink-secondary)]">
            {target.ranges
              .map((range) => `${range.reason} · символы ${range.start}–${range.end}`)
              .join("; ")}
          </p>
          {target.protectedRanges.length > 0 ? (
            <div className="mt-2 flex flex-wrap gap-2">
              {target.protectedRanges.map((range) => (
                <span
                  key={`${range.blockId}:${range.start}`}
                  className="rounded-full border border-[var(--warning)]/30 bg-[var(--warning-soft)] px-2 py-1 text-[10px] text-[var(--warning)]"
                >
                  Защищено · {range.reason}
                </span>
              ))}
            </div>
          ) : null}
        </div>
      ) : null}
      {canvasPreview ? <CanvasGhostOverlay preview={canvasPreview} /> : null}
      {noteFixes.length > 0 ? (
        <div className="mt-4 space-y-2">
          {noteFixes.map((fix) => (
            <div
              key={fix.annotationId}
              className="rounded-[var(--radius-md)] border border-[var(--warning)]/25 bg-[var(--warning-soft)] p-3 text-[11px] leading-5"
            >
              <p className="font-semibold">Заметка: {fix.note}</p>
              <p className="mt-1 text-[var(--ink-secondary)]">
                Якорь будет заменён: «{fix.originalText}» → «{fix.replacement}»
              </p>
              <div className="mt-2 flex items-center gap-2 font-semibold text-[var(--ink-secondary)]">
                <Checkbox
                  aria-labelledby={`annotation-resolution-${fix.annotationId}`}
                  checked={annotationResolutions[fix.annotationId] ?? fix.resolve}
                  disabled={!editable || excluded}
                  onCheckedChange={(checked) =>
                    onAnnotationResolutionChange(fix.annotationId, Boolean(checked))
                  }
                />
                <span id={`annotation-resolution-${fix.annotationId}`}>
                  После применения отметить исправленной
                </span>
              </div>
            </div>
          ))}
        </div>
      ) : null}
      {edits.length > 0 ? (
        <div className="mt-4 space-y-4">
          {edits.map((edit, index) => (
            <div
              key={`${edit.blockId}:${edit.start}:${edit.end}`}
              className="overflow-hidden rounded-[var(--radius-md)] border border-[var(--border-subtle)]"
            >
              {edits.length > 1 ? (
                <p className="bg-[var(--surface-subtle)] px-4 py-2 text-[10px] font-semibold tracking-wider text-[var(--ink-tertiary)] uppercase">
                  Точная правка {index + 1}
                </p>
              ) : null}
              <div className="grid sm:grid-cols-2">
                <div className="bg-[var(--diff-delete)] px-4 py-3">
                  <p className="text-[9px] font-semibold tracking-wider text-[var(--danger)] uppercase">
                    Было
                  </p>
                  <p className="font-manuscript mt-2 whitespace-pre-wrap text-[15px] leading-6">
                    {edit.before || "Пустой диапазон"}
                  </p>
                </div>
                <div className="bg-[var(--diff-insert)] px-4 py-3">
                  <p className="text-[9px] font-semibold tracking-wider text-[var(--success)] uppercase">
                    Предложено
                  </p>
                  <p className="font-manuscript mt-2 whitespace-pre-wrap text-[15px] leading-6">
                    {edit.after || "Удалить этот фрагмент"}
                  </p>
                </div>
              </div>
            </div>
          ))}
        </div>
      ) : before || after ? (
        <div className="mt-4 space-y-3">
          {before && before !== after ? (
            <div className="rounded-[var(--radius-md)] bg-[var(--diff-delete)] px-4 py-3">
              <p className="text-[9px] font-semibold tracking-wider text-[var(--danger)] uppercase">
                Было
              </p>
              <p className="font-manuscript mt-2 whitespace-pre-wrap text-[15px] leading-6">
                {before}
              </p>
            </div>
          ) : null}
          <div className="rounded-[var(--radius-md)] bg-[var(--diff-insert)] px-4 py-3">
            <p className="text-[9px] font-semibold tracking-wider text-[var(--success)] uppercase">
              Предложено
            </p>
            <p className="font-manuscript mt-2 whitespace-pre-wrap text-[15px] leading-6">
              {after || JSON.stringify(operation.proposedSnapshot, null, 2)}
            </p>
          </div>
        </div>
      ) : null}
    </article>
  );
}

function CanvasGhostOverlay({
  preview,
}: {
  preview: NonNullable<ReturnType<typeof canvasGhostPreview>>;
}) {
  const changed = preview.nodes.filter((node) => node.state !== "unchanged");
  const visible = changed.length > 0 ? changed : preview.nodes.slice(0, 40);
  const minX = Math.min(...visible.map((node) => node.x), 0);
  const minY = Math.min(...visible.map((node) => node.y), 0);
  const maxX = Math.max(...visible.map((node) => node.x + node.width), 320);
  const maxY = Math.max(...visible.map((node) => node.y + node.height), 180);
  const width = Math.max(maxX - minX, 1);
  const height = Math.max(maxY - minY, 1);
  const scale = Math.min(720 / width, 260 / height, 1);
  return (
    <div className="mt-4 overflow-hidden rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-app)]">
      <div className="flex flex-wrap gap-3 border-b border-[var(--border-subtle)] bg-[var(--surface-subtle)] px-3 py-2 text-[10px] text-[var(--ink-secondary)]">
        <strong className="text-[var(--ink-primary)]">Ghost preview</strong>
        <span>{preview.changedNodes} карточек изменится</span>
        <span>+{preview.addedEdges} связей</span>
        <span>−{preview.deletedEdges} связей</span>
      </div>
      <div className="relative h-[280px] overflow-hidden">
        {visible.map((node) => {
          const colors = {
            unchanged: "var(--border-strong)",
            added: "var(--success)",
            moved: "var(--info)",
            updated: "var(--warning)",
            deleted: "var(--danger)",
          };
          return (
            <div
              key={`${node.state}:${node.id}`}
              className="absolute overflow-hidden rounded border-2 bg-[var(--surface-raised)] px-2 py-1 text-[9px] shadow-sm"
              style={{
                left: 12 + (node.x - minX) * scale,
                top: 12 + (node.y - minY) * scale,
                width: Math.max(44, node.width * scale),
                height: Math.max(28, node.height * scale),
                borderColor: colors[node.state],
                opacity: node.state === "deleted" ? 0.5 : 1,
                borderStyle: node.state === "deleted" ? "dashed" : "solid",
              }}
              title={`${node.state}: ${node.label}`}
            >
              <span className="font-semibold uppercase" style={{ color: colors[node.state] }}>
                {node.state}
              </span>
              <span className="mt-1 block truncate">{node.label}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
