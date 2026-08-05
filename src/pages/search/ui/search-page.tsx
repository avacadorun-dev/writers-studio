import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowLeft, FileSearch, ReplaceAll, Search } from "lucide-react";
import { type FormEvent, useState } from "react";

import {
  applyProjectReplace,
  previewProjectReplace,
  searchProject,
  undoProjectReplace,
} from "~entities/project";
import { Banner, Button, Field, Input, Spinner, useToast } from "~shared/ui";
import { RequireSession } from "~widgets/require-session";

export function SearchPage({
  projectId,
  initialQuery = "",
  initialScope = "all",
}: {
  projectId: string;
  initialQuery?: string;
  initialScope?: string;
}) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { showToast } = useToast();
  const [query, setQuery] = useState(initialQuery);
  const [scope, setScope] = useState(initialScope);
  const [replaceSearch, setReplaceSearch] = useState(initialQuery);
  const [replacement, setReplacement] = useState("");
  const results = useQuery({
    queryKey: ["search", projectId, initialQuery, initialScope],
    queryFn: () => searchProject(projectId, initialQuery, initialScope),
    enabled: Boolean(initialQuery.trim()),
  });
  const preview = useMutation({
    mutationFn: () =>
      previewProjectReplace(projectId, {
        search: replaceSearch.trim(),
        replace: replacement,
        scope,
      }),
  });
  const apply = useMutation({
    mutationFn: () => {
      const current = preview.data?.preview;
      if (!current) {
        throw new Error("Сначала обновите предварительный просмотр");
      }
      return applyProjectReplace(projectId, {
        search: current.search,
        replace: current.replace,
        scope: current.scope,
        previewToken: current.token,
      });
    },
    onSuccess: async ({ result }) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["search", projectId] }),
      ]);
      preview.reset();
      showToast({
        title: `Заменено: ${result.replacements}`,
        description: `Созданы отдельные редакции в ${result.documents} документах.`,
        actionLabel: "Отменить",
        onAction: () => {
          void undoProjectReplace(projectId, result.operationId)
            .then(async () => {
              await Promise.all([
                queryClient.invalidateQueries({ queryKey: ["project", projectId] }),
                queryClient.invalidateQueries({ queryKey: ["search", projectId] }),
              ]);
              showToast({ title: "Массовая замена отменена" });
            })
            .catch((error: Error) => {
              showToast({ title: "Не удалось отменить", description: error.message });
            });
        },
      });
    },
  });
  const submit = (event: FormEvent) => {
    event.preventDefault();
    void navigate({
      to: "/projects/$projectId/search",
      params: { projectId },
      search: { q: query.trim() || undefined, scope },
    });
  };

  return (
    <RequireSession>
      <main className="mx-auto min-h-full w-full max-w-[936px] px-5 py-12 sm:py-[70px]">
        <Button asChild variant="ghost" size="icon" className="mb-4" aria-label="Назад в проект">
          <Link to="/projects/$projectId" params={{ projectId }}>
            <ArrowLeft className="size-4" aria-hidden="true" />
          </Link>
        </Button>
        <p className="text-[10px] font-semibold tracking-[0.16em] text-[var(--ink-tertiary)] uppercase">
          Поиск по проекту
        </p>
        <h1 className="font-display mt-4 text-[34px] leading-tight">
          Найдите фрагмент, не покидая рукопись
        </h1>
        <form onSubmit={submit} className="relative mt-5">
          <Search
            className="pointer-events-none absolute top-1/2 left-4 size-5 -translate-y-1/2 text-[var(--accent)]"
            aria-hidden="true"
          />
          <Input
            autoFocus
            className="h-[58px] rounded-[var(--radius-lg)] border-[var(--accent)] pr-28 pl-12 text-base"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Слово, фраза или имя"
            aria-label="Поисковый запрос"
          />
          <Button
            type="submit"
            variant="ghost"
            size="compact"
            className="absolute top-1/2 right-2 -translate-y-1/2"
          >
            Найти
          </Button>
        </form>
        <fieldset className="mt-4 flex flex-wrap gap-1">
          <legend className="sr-only">Область поиска</legend>
          {[
            ["all", "Везде"],
            ["manuscript", "Рукопись"],
            ["drafts", "Черновики"],
            ["notes", "Заметки"],
          ].map(([value, label]) => (
            <button
              key={value}
              type="button"
              className="min-h-9 rounded-[var(--radius-pill)] px-3 text-xs text-[var(--ink-secondary)] data-[active=true]:bg-[var(--surface-raised)] data-[active=true]:text-[var(--ink-primary)]"
              data-active={scope === value}
              onClick={() => {
                setScope(value);
                preview.reset();
                void navigate({
                  to: "/projects/$projectId/search",
                  params: { projectId },
                  search: { q: query.trim() || undefined, scope: value },
                });
              }}
            >
              {label}
            </button>
          ))}
        </fieldset>
        <details className="mt-6 rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-paper)]">
          <summary className="flex cursor-pointer list-none items-start gap-3 p-4">
            <span className="grid size-9 shrink-0 place-items-center rounded-full bg-[var(--accent-soft)] text-[var(--accent)]">
              <ReplaceAll className="size-4" aria-hidden="true" />
            </span>
            <div>
              <span className="block text-sm font-semibold">Заменить по проекту</span>
              <p className="mt-1 text-xs text-[var(--ink-tertiary)]">
                Сначала проверьте список изменений. Для каждого документа будет создана отдельная
                редакция с общим идентификатором операции.
              </p>
            </div>
          </summary>
          <div className="border-t border-[var(--border-subtle)] px-5 pb-5">
            <form
              className="mt-4 grid gap-3 sm:grid-cols-[1fr_1fr_auto]"
              onSubmit={(event) => {
                event.preventDefault();
                preview.mutate();
              }}
            >
              <Field label="Найти">
                <Input
                  required
                  maxLength={500}
                  value={replaceSearch}
                  onChange={(event) => {
                    setReplaceSearch(event.target.value);
                    preview.reset();
                  }}
                />
              </Field>
              <Field label="Заменить на">
                <Input
                  maxLength={5_000}
                  value={replacement}
                  onChange={(event) => {
                    setReplacement(event.target.value);
                    preview.reset();
                  }}
                />
              </Field>
              <Button
                className="self-end"
                type="submit"
                variant="secondary"
                disabled={!replaceSearch.trim() || preview.isPending}
              >
                {preview.isPending ? "Готовим…" : "Предпросмотр"}
              </Button>
            </form>
            {preview.isError ? (
              <Banner
                className="mt-4"
                variant="danger"
                title="Не удалось подготовить замену"
                body={preview.error.message}
              />
            ) : null}
            {apply.isError ? (
              <Banner
                className="mt-4"
                variant="danger"
                title="Замена не применена"
                body={apply.error.message}
              />
            ) : null}
            {preview.data ? (
              <div className="mt-5">
                <p className="text-sm font-semibold">
                  {preview.data.preview.totalCount} замен в {preview.data.preview.documents.length}{" "}
                  документах
                </p>
                <div className="mt-3 grid max-h-80 gap-2 overflow-y-auto">
                  {preview.data.preview.documents.map((document) => (
                    <article
                      key={document.nodeId}
                      className="rounded-[var(--radius-md)] bg-[var(--surface-subtle)] p-3"
                    >
                      <div className="flex items-center justify-between gap-3">
                        <h3 className="text-sm font-semibold">{document.title}</h3>
                        <span className="text-xs text-[var(--ink-tertiary)]">
                          {document.matchCount} совп.
                        </span>
                      </div>
                      <p className="mt-2 text-xs leading-5 text-[var(--ink-secondary)]">
                        <span className="line-through">{document.snippet}</span>
                        <span className="mt-1 block text-[var(--ink-primary)]">
                          → {document.replacement || "Пустой фрагмент"}
                        </span>
                      </p>
                    </article>
                  ))}
                  {preview.data.preview.documents.length === 0 ? (
                    <p className="text-sm text-[var(--ink-tertiary)]">
                      В выбранной области нет совпадений.
                    </p>
                  ) : null}
                </div>
                <Button
                  className="mt-4"
                  type="button"
                  disabled={preview.data.preview.totalCount === 0 || apply.isPending}
                  onClick={() => apply.mutate()}
                >
                  {apply.isPending ? "Применяем…" : "Применить показанные изменения"}
                </Button>
              </div>
            ) : null}
          </div>
        </details>
        {results.isFetching ? (
          <Spinner label="Ищем…" />
        ) : initialQuery && results.data?.results.length === 0 ? (
          <div className="mt-10 rounded-[var(--radius-lg)] border border-dashed border-[var(--border-strong)] p-10 text-center">
            <FileSearch className="mx-auto size-8 text-[var(--ink-tertiary)]" aria-hidden="true" />
            <h2 className="font-display mt-3 text-2xl">Ничего не найдено</h2>
            <p className="mt-1 text-sm text-[var(--ink-tertiary)]">
              Попробуйте другое написание или область поиска.
            </p>
          </div>
        ) : (
          <div className="mt-7 grid gap-3">
            {results.data?.results.map((result) => (
              <Link
                key={`${result.nodeId}-${result.revision}`}
                to="/projects/$projectId/studio/$nodeId"
                params={{ projectId, nodeId: result.nodeId }}
                search={{ highlight: initialQuery }}
                className="rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-4 hover:border-[var(--accent)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus)]"
              >
                <p className="text-xs text-[var(--ink-tertiary)]">{result.breadcrumb}</p>
                <p className="font-manuscript mt-2 text-lg leading-7">{result.snippet}</p>
                <p className="mt-2 text-xs text-[var(--accent)]">{result.matchCount} совпадений</p>
              </Link>
            ))}
          </div>
        )}
      </main>
    </RequireSession>
  );
}
