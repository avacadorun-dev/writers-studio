import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  AlertTriangle,
  BookMarked,
  Check,
  ExternalLink,
  MessageSquareText,
  RotateCcw,
  Save,
  Trash2,
  X,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import {
  type DocumentAnnotation,
  deleteDocumentAnnotation,
  updateDocumentAnnotation,
} from "~entities/project";
import { cn } from "~shared/lib";
import { Button, useActionDialog, useToast } from "~shared/ui";

interface ManuscriptAnnotationsPanelProps {
  projectId: string;
  nodeId: string;
  annotations: DocumentAnnotation[];
  selectedId?: string;
  onSelect: (annotationId: string) => void;
  onClose: () => void;
}

export function ManuscriptAnnotationsPanel({
  projectId,
  nodeId,
  annotations,
  selectedId,
  onSelect,
  onClose,
}: ManuscriptAnnotationsPanelProps) {
  const queryClient = useQueryClient();
  const { showToast } = useToast();
  const { requestConfirmation } = useActionDialog();
  const active = useMemo(
    () => annotations.filter((item) => item.status !== "resolved"),
    [annotations],
  );
  const resolved = useMemo(
    () => annotations.filter((item) => item.status === "resolved"),
    [annotations],
  );
  const selected = annotations.find((item) => item.id === selectedId) ?? active[0] ?? resolved[0];
  const [draft, setDraft] = useState(selected?.note ?? "");

  useEffect(() => setDraft(selected?.note ?? ""), [selected?.note]);

  const refresh = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["annotations", "node", nodeId] }),
      queryClient.invalidateQueries({ queryKey: ["annotations", "project", projectId] }),
    ]);
  };
  const update = useMutation({
    mutationFn: (input: Parameters<typeof updateDocumentAnnotation>[1]) =>
      updateDocumentAnnotation(selected?.id ?? "", input),
    onSuccess: async () => {
      await refresh();
      showToast({ title: "Заметка обновлена" });
    },
    onError: (error) =>
      showToast({ title: "Не удалось обновить заметку", description: error.message }),
  });
  const remove = useMutation({
    mutationFn: (annotationId: string) => deleteDocumentAnnotation(annotationId),
    onSuccess: async () => {
      await refresh();
      showToast({ title: "Пометка удалена" });
    },
  });

  return (
    <aside
      data-testid="annotations-docked-panel"
      className="fixed inset-x-0 bottom-0 z-40 flex max-h-[72dvh] min-h-0 flex-col border-t border-[var(--border-subtle)] bg-[var(--surface-panel)] shadow-[0_-12px_32px_var(--shadow-color)] lg:static lg:z-auto lg:h-full lg:max-h-none lg:w-[360px] lg:shrink-0 lg:border-t-0 lg:border-l lg:shadow-none"
    >
      <header
        data-testid="annotations-panel-header"
        className="flex h-14 shrink-0 items-center gap-3 border-b border-[var(--border-subtle)] px-4"
      >
        <MessageSquareText className="size-4 shrink-0 text-[var(--accent)]" aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-sm font-semibold">Заметки и ссылки</h2>
          <p className="truncate text-[10px] text-[var(--ink-tertiary)]">Поля рукописи</p>
        </div>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label="Закрыть поля"
          onClick={onClose}
        >
          <X className="size-4" aria-hidden="true" />
        </Button>
      </header>

      <div className="border-b border-[var(--border-subtle)] p-3">
        <Button asChild variant="secondary" className="w-full justify-between">
          <Link to="/projects/$projectId/annotations" params={{ projectId }}>
            Все правки проекта
            <ExternalLink className="size-4" aria-hidden="true" />
          </Link>
        </Button>
      </div>

      <div className="grid min-h-0 flex-1 grid-cols-[132px_minmax(0,1fr)] lg:grid-cols-1 lg:grid-rows-[minmax(140px,42%)_minmax(0,1fr)]">
        <div className="min-h-0 overflow-y-auto border-r border-[var(--border-subtle)] p-2 lg:border-r-0 lg:border-b">
          {active.length === 0 ? (
            <p className="p-3 text-xs leading-5 text-[var(--ink-secondary)]">
              Выделите текст, чтобы добавить заметку или связать его со словарём.
            </p>
          ) : (
            active.map((item) => (
              <AnnotationListItem
                key={item.id}
                item={item}
                selected={item.id === selected?.id}
                onSelect={onSelect}
              />
            ))
          )}
          {resolved.length > 0 ? (
            <details className="mt-2">
              <summary className="cursor-pointer px-2 py-2 text-[11px] text-[var(--ink-secondary)]">
                Исправлено · {resolved.length}
              </summary>
              {resolved.map((item) => (
                <AnnotationListItem
                  key={item.id}
                  item={item}
                  selected={item.id === selected?.id}
                  onSelect={onSelect}
                />
              ))}
            </details>
          ) : null}
        </div>

        <div className="min-h-0 overflow-y-auto p-4">
          {selected ? (
            <>
              <div className="flex items-start gap-3">
                <span
                  className={cn(
                    "mt-0.5 grid size-8 shrink-0 place-items-center rounded-full",
                    selected.kind === "note"
                      ? "bg-[var(--warning-soft)] text-[var(--warning)]"
                      : "bg-[var(--accent-soft)] text-[var(--accent)]",
                  )}
                >
                  {selected.kind === "note" ? (
                    <MessageSquareText className="size-4" aria-hidden="true" />
                  ) : (
                    <BookMarked className="size-4" aria-hidden="true" />
                  )}
                </span>
                <div className="min-w-0">
                  <p className="line-clamp-2 text-sm font-semibold">
                    «{selected.anchor.exactText}»
                  </p>
                  <p className="mt-1 text-[11px] text-[var(--ink-tertiary)]">
                    Редакция {selected.baseRevision}
                  </p>
                </div>
              </div>

              {selected.anchorStatus === "detached" || selected.status === "needs_attention" ? (
                <div className="mt-4 flex gap-2 rounded-[var(--radius-md)] bg-[var(--danger-soft)] p-3 text-xs leading-5 text-[var(--danger)]">
                  <AlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
                  Фрагмент или термин изменился. Пометка сохранена, но её нужно перепривязать.
                </div>
              ) : null}

              {selected.kind === "note" ? (
                <label className="mt-4 block text-xs font-semibold">
                  Заметка
                  <textarea
                    value={draft}
                    onChange={(event) => setDraft(event.target.value)}
                    rows={6}
                    maxLength={10000}
                    className="mt-2 w-full resize-y rounded-[var(--radius-md)] border border-[var(--border-strong)] bg-[var(--surface-paper)] px-3 py-2 text-sm font-normal leading-6 outline-none focus:border-[var(--focus)]"
                  />
                </label>
              ) : (
                <div className="mt-4 rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-3">
                  <p className="text-[10px] font-semibold tracking-wide text-[var(--ink-tertiary)] uppercase">
                    Термин словаря
                  </p>
                  <p className="mt-1 font-semibold">
                    {selected.dictionaryTerm?.canonical ?? "Удалённый термин"}
                  </p>
                  <p className="mt-1 text-xs text-[var(--ink-secondary)]">
                    {selected.dictionaryTerm?.id
                      ? selected.dictionaryTerm.active
                        ? "Активен в словаре"
                        : "Отключён в словаре"
                      : "Снимок термина сохранён вместе с проектом"}
                  </p>
                </div>
              )}

              <div className="mt-5 flex flex-wrap gap-2">
                {selected.kind === "note" && draft.trim() !== selected.note?.trim() ? (
                  <Button
                    type="button"
                    size="compact"
                    disabled={!draft.trim() || update.isPending}
                    onClick={() => update.mutate({ note: draft.trim() })}
                  >
                    <Save className="size-3.5" aria-hidden="true" />
                    Сохранить
                  </Button>
                ) : null}
                <Button
                  type="button"
                  variant="secondary"
                  size="compact"
                  disabled={update.isPending}
                  onClick={() =>
                    update.mutate({ status: selected.status === "resolved" ? "open" : "resolved" })
                  }
                >
                  {selected.status === "resolved" ? (
                    <RotateCcw className="size-3.5" aria-hidden="true" />
                  ) : (
                    <Check className="size-3.5" aria-hidden="true" />
                  )}
                  {selected.status === "resolved" ? "Вернуть" : "Исправлено"}
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="compact"
                  onClick={() => {
                    void requestConfirmation({
                      title: "Удалить пометку?",
                      description: "Текст рукописи и словарь не изменятся.",
                      confirmLabel: "Удалить",
                      tone: "danger",
                    }).then((confirmed) => {
                      if (confirmed) remove.mutate(selected.id);
                    });
                  }}
                >
                  <Trash2 className="size-3.5" aria-hidden="true" />
                  Удалить
                </Button>
              </div>
            </>
          ) : null}
        </div>
      </div>
    </aside>
  );
}

function AnnotationListItem({
  item,
  selected,
  onSelect,
}: {
  item: DocumentAnnotation;
  selected: boolean;
  onSelect: (id: string) => void;
}) {
  return (
    <button
      type="button"
      className={cn(
        "mb-1 w-full rounded-[var(--radius-sm)] px-2 py-2 text-left text-xs",
        selected ? "bg-[var(--surface-selected)]" : "hover:bg-[var(--surface-subtle)]",
      )}
      onClick={() => onSelect(item.id)}
    >
      <span className="flex items-center gap-1.5 font-semibold">
        {item.anchorStatus === "detached" || item.status === "needs_attention" ? (
          <AlertTriangle className="size-3 text-[var(--danger)]" aria-hidden="true" />
        ) : item.kind === "note" ? (
          <MessageSquareText className="size-3 text-[var(--warning)]" aria-hidden="true" />
        ) : (
          <BookMarked className="size-3 text-[var(--accent)]" aria-hidden="true" />
        )}
        <span className="truncate">{item.anchor.exactText}</span>
      </span>
      <span className="mt-1 block line-clamp-2 text-[11px] leading-4 text-[var(--ink-secondary)]">
        {item.kind === "note" ? item.note : (item.dictionaryTerm?.canonical ?? "Удалённый термин")}
      </span>
    </button>
  );
}
