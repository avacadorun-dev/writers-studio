import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  AlertTriangle,
  ArrowUpRight,
  BookMarked,
  CheckCircle2,
  MessageSquareText,
  Search,
} from "lucide-react";
import { useMemo, useState } from "react";

import { type DocumentAnnotation, getProject, listProjectAnnotations } from "~entities/project";
import { cn } from "~shared/lib";
import { Banner, Button, Input, Select, Spinner } from "~shared/ui";
import { ProjectUtilityShell } from "~widgets/project-utility-shell";

type StatusFilter = "active" | "needs_attention" | "resolved" | "all";

export function ProjectAnnotationsPage({ projectId }: { projectId: string }) {
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState<StatusFilter>("active");
  const [kind, setKind] = useState<DocumentAnnotation["kind"] | "all">("all");
  const [nodeId, setNodeId] = useState("all");
  const project = useQuery({
    queryKey: ["project", projectId],
    queryFn: () => getProject(projectId),
  });
  const annotations = useQuery({
    queryKey: ["annotations", "project", projectId],
    queryFn: () => listProjectAnnotations(projectId),
  });

  const items = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase();
    return (annotations.data?.annotations ?? []).filter((item) => {
      if (status === "active" && item.status === "resolved") return false;
      if (
        status === "needs_attention" &&
        item.anchorStatus !== "detached" &&
        item.status !== "needs_attention"
      )
        return false;
      if (
        status !== "active" &&
        status !== "all" &&
        status !== "needs_attention" &&
        item.status !== status
      )
        return false;
      if (kind !== "all" && item.kind !== kind) return false;
      if (nodeId !== "all" && item.nodeId !== nodeId) return false;
      return (
        !needle ||
        item.nodeTitle.toLocaleLowerCase().includes(needle) ||
        item.anchor.exactText.toLocaleLowerCase().includes(needle) ||
        item.note?.toLocaleLowerCase().includes(needle) ||
        item.dictionaryTerm?.canonical.toLocaleLowerCase().includes(needle)
      );
    });
  }, [annotations.data?.annotations, kind, nodeId, query, status]);

  if (project.isPending || !project.data) return <Spinner label="Собираем правки проекта…" />;

  const all = annotations.data?.annotations ?? [];
  const activeCount = all.filter((item) => item.status !== "resolved").length;
  const attentionCount = all.filter(
    (item) => item.anchorStatus === "detached" || item.status === "needs_attention",
  ).length;
  const noteNodes = project.data.nodes.filter((node) =>
    all.some((annotation) => annotation.nodeId === node.id),
  );

  return (
    <ProjectUtilityShell
      project={project.data.project}
      nodes={project.data.nodes}
      expandedTree
      contentClassName="px-5 py-7 lg:px-8"
    >
      <div className="mx-auto w-full max-w-[1096px]">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p className="text-[10px] font-semibold tracking-[0.14em] text-[var(--ink-tertiary)] uppercase">
              Весь проект · быстрые переходы
            </p>
            <h1 className="font-display mt-2 text-[34px] leading-tight">Правки в тексте</h1>
            <p className="mt-2 text-sm text-[var(--ink-secondary)]">
              {activeCount} открытых пометок
              {attentionCount ? ` · ${attentionCount} требуют внимания` : ""}
            </p>
          </div>
          <Button asChild variant="secondary">
            <Link
              to="/projects/$projectId/studio/$nodeId"
              params={{
                projectId,
                nodeId: project.data.nodes.find((node) => !node.trashedAt)?.id ?? "",
              }}
            >
              Открыть рукопись
            </Link>
          </Button>
        </div>

        {attentionCount > 0 ? (
          <Banner
            className="mt-5"
            variant="warning"
            title="Некоторые пометки нужно проверить"
            body="Текст или термин изменился. Мы сохранили исходную цитату и не переносили пометку наугад."
          />
        ) : null}

        <div className="mt-6 grid gap-2 md:grid-cols-[minmax(240px,1fr)_180px_180px]">
          <div className="relative">
            <Search
              className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-[var(--ink-tertiary)]"
              aria-hidden="true"
            />
            <Input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              className="pl-10"
              placeholder="Найти заметку, цитату или термин"
              aria-label="Поиск по правкам"
            />
          </div>
          <Select
            value={kind}
            onChange={(event) => setKind(event.target.value as DocumentAnnotation["kind"] | "all")}
            aria-label="Тип пометки"
          >
            <option value="all">Все типы</option>
            <option value="note">Заметки</option>
            <option value="dictionary_link">Ссылки на словарь</option>
          </Select>
          <Select
            value={nodeId}
            onChange={(event) => setNodeId(event.target.value)}
            aria-label="Документ"
          >
            <option value="all">Все документы</option>
            {noteNodes.map((node) => (
              <option key={node.id} value={node.id}>
                {node.title}
              </option>
            ))}
          </Select>
        </div>

        <fieldset className="mt-3 flex flex-wrap gap-2">
          <legend className="sr-only">Статус правок</legend>
          {(
            [
              ["active", `Открытые · ${activeCount}`],
              ["needs_attention", `Нужно проверить · ${attentionCount}`],
              ["resolved", `Исправлено · ${all.length - activeCount}`],
              ["all", `Все · ${all.length}`],
            ] as const
          ).map(([value, label]) => (
            <Button
              key={value}
              type="button"
              size="compact"
              variant={status === value ? "primary" : "secondary"}
              onClick={() => setStatus(value)}
            >
              {label}
            </Button>
          ))}
        </fieldset>

        {annotations.isError ? (
          <Banner
            className="mt-6"
            variant="danger"
            title="Не удалось загрузить правки"
            body="Проверьте соединение и обновите страницу."
          />
        ) : items.length === 0 ? (
          <div className="mt-8 rounded-[var(--radius-lg)] border border-dashed border-[var(--border-strong)] bg-[var(--surface-paper)] px-6 py-14 text-center">
            <MessageSquareText
              className="mx-auto size-7 text-[var(--ink-tertiary)]"
              aria-hidden="true"
            />
            <h2 className="font-display mt-3 text-xl">Подходящих правок нет</h2>
            <p className="mt-2 text-sm text-[var(--ink-secondary)]">
              Измените фильтры или добавьте заметку к выделенному тексту в рукописи.
            </p>
          </div>
        ) : (
          <div className="mt-6 space-y-3">
            {items.map((item) => (
              <AnnotationCard key={item.id} item={item} projectId={projectId} />
            ))}
          </div>
        )}
      </div>
    </ProjectUtilityShell>
  );
}

function AnnotationCard({ item, projectId }: { item: DocumentAnnotation; projectId: string }) {
  const Icon =
    item.anchorStatus === "detached" || item.status === "needs_attention"
      ? AlertTriangle
      : item.status === "resolved"
        ? CheckCircle2
        : item.kind === "note"
          ? MessageSquareText
          : BookMarked;
  return (
    <article className="group grid gap-4 rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-4 transition-colors hover:border-[var(--border-strong)] sm:grid-cols-[36px_minmax(0,1fr)_auto] sm:items-start">
      <span
        className={cn(
          "grid size-9 place-items-center rounded-full",
          item.anchorStatus === "detached" || item.status === "needs_attention"
            ? "bg-[var(--danger-soft)] text-[var(--danger)]"
            : item.kind === "note"
              ? "bg-[var(--warning-soft)] text-[var(--warning)]"
              : "bg-[var(--accent-soft)] text-[var(--accent)]",
        )}
      >
        <Icon className="size-4" aria-hidden="true" />
      </span>
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-[var(--ink-tertiary)]">
          <span className="font-semibold text-[var(--ink-secondary)]">{item.nodeTitle}</span>
          <span aria-hidden="true">·</span>
          <span>{item.kind === "note" ? "Заметка" : "Словарь"}</span>
          {item.anchorStatus === "detached" || item.status === "needs_attention" ? (
            <span className="font-semibold text-[var(--danger)]">Нужно проверить</span>
          ) : null}
        </div>
        <blockquote className="font-manuscript mt-2 border-l-2 border-[var(--border-strong)] pl-3 text-sm leading-6">
          «{item.anchor.exactText}»
        </blockquote>
        <p className="mt-2 text-sm leading-6 text-[var(--ink-secondary)]">
          {item.kind === "note"
            ? item.note
            : `Связано с термином «${item.dictionaryTerm?.canonical ?? "удалённый термин"}»`}
        </p>
      </div>
      <Button asChild variant="secondary" size="compact" className="sm:self-center">
        <Link
          to="/projects/$projectId/studio/$nodeId"
          params={{ projectId, nodeId: item.nodeId }}
          search={{ annotationId: item.id }}
        >
          Перейти
          <ArrowUpRight className="size-3.5" aria-hidden="true" />
        </Link>
      </Button>
    </article>
  );
}
