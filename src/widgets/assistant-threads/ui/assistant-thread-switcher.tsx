import { useQuery } from "@tanstack/react-query";
import { Bot, MessageSquarePlus, MessagesSquare, Plus, X } from "lucide-react";

import { type AssistantRun, listAssistantRuns } from "~entities/assistant";
import type { DocumentNode } from "~entities/project";
import { cn } from "~shared/lib";
import { Banner, Button, Spinner } from "~shared/ui";
import {
  type AssistantThreadBucket,
  assistantThreadBucket,
  assistantThreadCountLabel,
  assistantThreadStatus,
  assistantThreadTitle,
  assistantThreadUpdatedLabel,
  groupAssistantThreads,
} from "../model/assistant-threads";

interface AssistantThreadSwitcherProps {
  projectId: string;
  activeRunId?: string;
  nodes: DocumentNode[];
  onSelect: (runId: string) => void;
  onNew: () => void;
  onClose: () => void;
}

const sectionLabels: Record<AssistantThreadBucket, string> = {
  attention: "Нужно внимание",
  working: "В работе",
  recent: "Недавние",
};

export function AssistantThreadSwitcher({
  projectId,
  activeRunId,
  nodes,
  onSelect,
  onNew,
  onClose,
}: AssistantThreadSwitcherProps) {
  const runs = useQuery({
    queryKey: ["assistant-runs", projectId],
    queryFn: () => listAssistantRuns(projectId),
    refetchInterval: ({ state }) =>
      state.data?.runs.some((run) => run.status === "working") ? 1_200 : 10_000,
  });
  const groups = groupAssistantThreads(runs.data?.runs ?? []);
  const nodeTitles = new Map(nodes.map((node) => [node.id, node.title]));

  return (
    <aside
      aria-label="Чаты ассистента"
      data-testid="assistant-thread-switcher"
      className="flex h-full min-h-0 w-full flex-col border-l border-[var(--border-subtle)] bg-[var(--surface-panel)]"
    >
      <header className="flex h-14 shrink-0 items-center gap-3 border-b border-[var(--border-subtle)] px-3">
        <span className="grid size-8 shrink-0 place-items-center rounded-full bg-[var(--accent-soft)] text-[var(--accent)]">
          <MessagesSquare className="size-4" aria-hidden="true" />
        </span>
        <div className="min-w-0 flex-1">
          <p className="text-[9px] font-semibold tracking-[0.12em] text-[var(--ink-tertiary)] uppercase">
            Ассистент · чаты проекта
          </p>
          <p className="truncate text-sm font-semibold">Все поручения</p>
        </div>
        <Button type="button" variant="ghost" size="icon" onClick={onNew} aria-label="Новый чат">
          <Plus className="size-4" aria-hidden="true" />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          onClick={onClose}
          aria-label="Скрыть панель"
        >
          <X className="size-4" aria-hidden="true" />
        </Button>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto px-3 py-3">
        <Button type="button" className="w-full" onClick={onNew} data-testid="assistant-new-thread">
          <MessageSquarePlus className="size-4" aria-hidden="true" />
          Новый чат
        </Button>

        {runs.isPending ? (
          <div className="py-12">
            <Spinner label="Загружаем чаты…" />
          </div>
        ) : runs.isError ? (
          <Banner
            variant="danger"
            title="Чаты недоступны"
            body="Обновите список или продолжите работу с открытой рукописью."
          />
        ) : runs.data.runs.length === 0 ? (
          <div className="mt-4 rounded-[var(--radius-md)] border border-dashed border-[var(--border-strong)] bg-[var(--surface-paper)] p-5 text-center">
            <Bot className="mx-auto size-5 text-[var(--accent)]" aria-hidden="true" />
            <p className="mt-3 text-sm font-semibold">Пока нет поручений</p>
            <p className="mt-1 text-[11px] leading-5 text-[var(--ink-secondary)]">
              Начните чат — рукопись и контекст проекта останутся общими.
            </p>
          </div>
        ) : (
          <div className="mt-4 space-y-5">
            {(["attention", "working", "recent"] as const).map((bucket) =>
              groups[bucket].length > 0 ? (
                <section key={bucket} aria-labelledby={`assistant-${bucket}-label`}>
                  <h3
                    id={`assistant-${bucket}-label`}
                    className={cn(
                      "mb-2 px-1 text-[10px] font-semibold tracking-[0.1em] uppercase",
                      bucket === "attention"
                        ? "text-[var(--warning)]"
                        : "text-[var(--ink-tertiary)]",
                    )}
                  >
                    {sectionLabels[bucket]}
                  </h3>
                  <div className="space-y-2">
                    {groups[bucket].map((run) => (
                      <AssistantThreadRow
                        key={run.id}
                        run={run}
                        active={run.id === activeRunId}
                        targetTitle={
                          run.targetNodeId ? nodeTitles.get(run.targetNodeId) : undefined
                        }
                        onSelect={() => onSelect(run.id)}
                      />
                    ))}
                  </div>
                </section>
              ) : null,
            )}
          </div>
        )}
      </div>

      <footer className="shrink-0 border-t border-[var(--border-subtle)] px-4 py-3 text-center text-[10px] leading-4 text-[var(--ink-tertiary)]">
        {assistantThreadCountLabel(runs.data?.runs.length ?? 0)} · рукопись общая, выполнение
        независимое
      </footer>
    </aside>
  );
}

function AssistantThreadRow({
  run,
  active,
  targetTitle,
  onSelect,
}: {
  run: AssistantRun;
  active: boolean;
  targetTitle?: string;
  onSelect: () => void;
}) {
  const bucket = assistantThreadBucket(run);
  const status = assistantThreadStatus(run);
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-current={active ? "true" : undefined}
      data-testid={`assistant-thread-${run.id}`}
      className={cn(
        "flex min-h-16 w-full items-center gap-3 rounded-[var(--radius-md)] border px-3 py-2 text-left transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus)]",
        active
          ? "border-[var(--accent)] bg-[var(--accent-soft)]"
          : bucket === "attention"
            ? "border-[var(--warning)]/45 bg-[var(--warning-soft)]"
            : "border-[var(--border-subtle)] bg-[var(--surface-paper)] hover:border-[var(--border-strong)]",
      )}
    >
      <span
        className={cn(
          "size-2 shrink-0 rounded-full",
          bucket === "attention"
            ? "bg-[var(--warning)]"
            : bucket === "working" || active
              ? "bg-[var(--accent)]"
              : "bg-[var(--ink-tertiary)]",
        )}
        aria-hidden="true"
      />
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[12px] font-semibold">
          {assistantThreadTitle(run)}
        </span>
        <span className="mt-0.5 block truncate text-[10px] text-[var(--ink-tertiary)]">
          {targetTitle ?? "Весь проект"} · {assistantThreadUpdatedLabel(run.updatedAt)}
        </span>
      </span>
      <span
        className={cn(
          "shrink-0 rounded-full px-2 py-1 text-[8px] font-bold tracking-[0.06em]",
          bucket === "attention"
            ? "bg-[var(--warning-soft)] text-[var(--warning)]"
            : bucket === "working" || active
              ? "bg-[var(--accent-soft)] text-[var(--accent)]"
              : "bg-[var(--surface-subtle)] text-[var(--ink-secondary)]",
        )}
      >
        {status}
      </span>
    </button>
  );
}
