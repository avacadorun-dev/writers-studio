import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { GitCompareArrows, History, Plus, RotateCcw } from "lucide-react";
import { type FormEvent, useEffect, useMemo, useState } from "react";

import { createVersion, getNode, getProject, getVersions, restoreVersion } from "~entities/project";
import {
  Banner,
  Button,
  Field,
  Input,
  Select,
  Spinner,
  useActionDialog,
  useToast,
} from "~shared/ui";
import { ProjectUtilityShell } from "~widgets/project-utility-shell";

export function VersionsPage({
  nodeId,
  preparedVersionId,
  expectedCurrentRevision,
}: {
  nodeId: string;
  preparedVersionId?: string;
  expectedCurrentRevision?: number;
}) {
  const queryClient = useQueryClient();
  const { showToast } = useToast();
  const { requestConfirmation } = useActionDialog();
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [note, setNote] = useState("");
  const [leftRevision, setLeftRevision] = useState("");
  const [rightRevision, setRightRevision] = useState("");
  const node = useQuery({ queryKey: ["node", nodeId], queryFn: () => getNode(nodeId) });
  const workspace = useQuery({
    queryKey: ["project", node.data?.node.projectId],
    queryFn: () => getProject(node.data?.node.projectId ?? ""),
    enabled: Boolean(node.data?.node.projectId),
  });
  const history = useQuery({
    queryKey: ["versions", nodeId],
    queryFn: () => getVersions(nodeId),
  });
  const create = useMutation({
    mutationFn: () => createVersion(nodeId, { name: name.trim(), note: note.trim() }),
    onSuccess: () => {
      setCreating(false);
      setName("");
      setNote("");
      void queryClient.invalidateQueries({ queryKey: ["versions", nodeId] });
      showToast({ title: "Именованная версия создана" });
    },
  });
  const restore = useMutation({
    mutationFn: restoreVersion,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["versions", nodeId] }),
        queryClient.invalidateQueries({ queryKey: ["node", nodeId] }),
      ]);
      showToast({
        title: "Версия восстановлена как новая правка",
        description: "Предыдущая история не удалена.",
      });
    },
  });
  useEffect(() => {
    const revisions = history.data?.revisions ?? [];
    if (!rightRevision && revisions[0]) {
      setRightRevision(String(revisions[0].revision));
    }
    if (!leftRevision && revisions[1]) {
      setLeftRevision(String(revisions[1].revision));
    }
  }, [history.data?.revisions, leftRevision, rightRevision]);
  const comparison = useMemo(() => {
    const revisions = history.data?.revisions ?? [];
    const left = revisions.find((revision) => String(revision.revision) === leftRevision);
    const right = revisions.find((revision) => String(revision.revision) === rightRevision);
    return left && right ? compareWords(left.plainText, right.plainText) : [];
  }, [history.data?.revisions, leftRevision, rightRevision]);

  if (node.isPending || history.isPending || workspace.isPending) {
    return <Spinner label="Открываем историю…" />;
  }
  const document = node.data?.node;
  if (!document || !workspace.data) {
    return (
      <main className="grid min-h-full place-items-center p-6">
        <Banner variant="danger" title="История недоступна" body="Документ не найден." />
      </main>
    );
  }
  const preparedVersion = history.data?.versions.find(
    (version) => version.id === preparedVersionId,
  );
  const preparedRestoreIsStale =
    expectedCurrentRevision !== undefined && document.currentRevision !== expectedCurrentRevision;
  return (
    <ProjectUtilityShell
      project={workspace.data.project}
      nodes={workspace.data.nodes}
      activeNodeId={document.id}
    >
      <div className="mx-auto w-full max-w-[1280px] px-5 py-7 lg:px-8">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p className="text-[10px] font-semibold tracking-[0.14em] text-[var(--ink-tertiary)] uppercase">
              {document.title}
            </p>
            <h1 className="font-display mt-2 text-[34px] leading-tight">История версий</h1>
          </div>
          <Button type="button" onClick={() => setCreating(true)}>
            <Plus className="size-4" aria-hidden="true" />
            Сохранить версию
          </Button>
        </div>

        {creating ? (
          <form
            className="mt-5 grid gap-4 rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-5 sm:grid-cols-2"
            onSubmit={(event: FormEvent) => {
              event.preventDefault();
              create.mutate();
            }}
          >
            <Field label="Название версии">
              <Input
                required
                maxLength={160}
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
            </Field>
            <Field label="Заметка · необязательно">
              <Input value={note} onChange={(event) => setNote(event.target.value)} />
            </Field>
            <div className="flex justify-end gap-2 sm:col-span-2">
              <Button type="button" variant="ghost" onClick={() => setCreating(false)}>
                Отмена
              </Button>
              <Button type="submit" disabled={!name.trim() || create.isPending}>
                Сохранить снимок
              </Button>
            </div>
          </form>
        ) : null}

        {preparedVersionId ? (
          <Banner
            className="mt-5"
            variant={preparedRestoreIsStale || !preparedVersion ? "warning" : "info"}
            title={
              preparedRestoreIsStale
                ? "Документ изменился после поручения"
                : preparedVersion
                  ? `Ассистент выбрал «${preparedVersion.name}»`
                  : "Подготовленная версия больше недоступна"
            }
            body={
              preparedRestoreIsStale
                ? "Сначала сравните актуальный текст с выбранной версией. Старое подтверждение не применяется автоматически."
                : "Текущий текст не изменён. Выбранная версия выделена ниже; восстановление требует отдельного подтверждения."
            }
          />
        ) : null}

        <div className="mt-5 grid gap-4 lg:grid-cols-[390px_minmax(0,1fr)]">
          <aside className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-panel)] p-4">
            <div className="flex items-center justify-between gap-3">
              <h2 className="text-sm font-semibold">Сохранённые версии</h2>
              <span className="text-xs text-[var(--ink-tertiary)]">
                {history.data?.versions.length ?? 0}
              </span>
            </div>
            <div className="mt-3 grid gap-2">
              {history.data?.versions.map((version) => (
                <article
                  key={version.id}
                  aria-current={version.id === preparedVersionId ? "true" : undefined}
                  className={
                    version.id === preparedVersionId
                      ? "rounded-[var(--radius-md)] border border-[var(--accent)] bg-[var(--accent-soft)] p-3 ring-2 ring-[var(--accent)]/15"
                      : "rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-3"
                  }
                >
                  <div className="flex items-start gap-3">
                    <History className="mt-0.5 size-4 shrink-0 text-[var(--accent)]" aria-hidden />
                    <div className="min-w-0 flex-1">
                      <h3 className="text-sm font-semibold">{version.name}</h3>
                      <p className="mt-1 text-xs text-[var(--ink-tertiary)]">
                        Редакция {version.revision} ·{" "}
                        {new Intl.DateTimeFormat("ru", {
                          dateStyle: "medium",
                          timeStyle: "short",
                        }).format(new Date(version.createdAt))}
                      </p>
                      {version.note ? (
                        <p className="mt-2 text-xs text-[var(--ink-secondary)]">{version.note}</p>
                      ) : null}
                    </div>
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="compact"
                    className="mt-2 w-full"
                    disabled={
                      restore.isPending ||
                      (version.id === preparedVersionId && preparedRestoreIsStale)
                    }
                    onClick={() => {
                      if (version.id === preparedVersionId && preparedRestoreIsStale) {
                        showToast({
                          title: "Нужно заново проверить восстановление",
                          description: "Текущая редакция изменилась после поручения ассистенту.",
                        });
                        return;
                      }
                      void requestConfirmation({
                        title: `Восстановить «${version.name}»?`,
                        description:
                          "Текущая редакция останется в истории. Восстановленная версия станет новой текущей редакцией.",
                        icon: RotateCcw,
                        confirmLabel: "Восстановить",
                      }).then((confirmed) => {
                        if (confirmed) restore.mutate(version.id);
                      });
                    }}
                  >
                    <RotateCcw className="size-4" aria-hidden="true" />
                    Восстановить
                  </Button>
                </article>
              ))}
              {history.data?.versions.length === 0 ? (
                <div className="rounded-[var(--radius-md)] border border-dashed border-[var(--border-strong)] p-6 text-center">
                  <History
                    className="mx-auto size-6 text-[var(--ink-tertiary)]"
                    aria-hidden="true"
                  />
                  <p className="mt-2 text-sm font-medium">Пока нет именованных версий</p>
                </div>
              ) : null}
            </div>
            <Button asChild variant="ghost" className="justify-self-start">
              <Link
                to="/projects/$projectId/studio/$nodeId"
                params={{ projectId: document.projectId, nodeId: document.id }}
              >
                Открыть документ
              </Link>
            </Button>
          </aside>

          <section className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-5 sm:p-7">
            <div className="flex items-start gap-3">
              <span className="grid size-9 shrink-0 place-items-center rounded-full bg-[var(--accent-soft)] text-[var(--accent)]">
                <GitCompareArrows className="size-4" aria-hidden="true" />
              </span>
              <div>
                <h2 className="text-sm font-semibold">Сравнить редакции</h2>
                <p className="mt-1 text-xs text-[var(--ink-tertiary)]">
                  Сравнение работает только для чтения и не изменяет документ.
                </p>
              </div>
            </div>
            <div className="mt-4 grid gap-3 sm:grid-cols-2">
              <Field label="Более ранняя редакция">
                <Select
                  value={leftRevision}
                  onChange={(event) => setLeftRevision(event.target.value)}
                >
                  <option value="">Выберите редакцию</option>
                  {history.data?.revisions.map((revision) => (
                    <option key={revision.id} value={revision.revision}>
                      Редакция {revision.revision} · {revision.source}
                    </option>
                  ))}
                </Select>
              </Field>
              <Field label="Более поздняя редакция">
                <Select
                  value={rightRevision}
                  onChange={(event) => setRightRevision(event.target.value)}
                >
                  <option value="">Выберите редакцию</option>
                  {history.data?.revisions.map((revision) => (
                    <option key={revision.id} value={revision.revision}>
                      Редакция {revision.revision} · {revision.source}
                    </option>
                  ))}
                </Select>
              </Field>
            </div>
            <section
              aria-label="Сравнение редакций"
              className="font-manuscript mt-5 min-h-[520px] rounded-[var(--radius-md)] bg-[var(--surface-subtle)] p-6 text-lg leading-8"
            >
              {leftRevision && rightRevision ? (
                comparison.length === 0 ? (
                  <p className="font-ui text-sm text-[var(--ink-tertiary)]">
                    Текст редакций совпадает.
                  </p>
                ) : (
                  comparison.map((token) => (
                    <span
                      key={token.id}
                      className={
                        token.kind === "insert"
                          ? "rounded-sm bg-[var(--diff-insert)]"
                          : token.kind === "delete"
                            ? "bg-[var(--diff-delete)] line-through"
                            : ""
                      }
                    >
                      {token.text}
                    </span>
                  ))
                )
              ) : (
                <p className="font-ui text-sm text-[var(--ink-tertiary)]">
                  Для сравнения нужны как минимум две редакции.
                </p>
              )}
            </section>
          </section>
        </div>
      </div>
    </ProjectUtilityShell>
  );
}

interface ComparisonToken {
  id: string;
  kind: "same" | "insert" | "delete";
  text: string;
}

function compareWords(before: string, after: string): ComparisonToken[] {
  const left = tokenize(before);
  const right = tokenize(after);
  const table = Array.from({ length: left.length + 1 }, () => new Uint32Array(right.length + 1));
  for (let i = left.length - 1; i >= 0; i -= 1) {
    for (let j = right.length - 1; j >= 0; j -= 1) {
      table[i][j] =
        left[i] === right[j] ? table[i + 1][j + 1] + 1 : Math.max(table[i + 1][j], table[i][j + 1]);
    }
  }
  const result: ComparisonToken[] = [];
  let i = 0;
  let j = 0;
  let id = 0;
  while (i < left.length || j < right.length) {
    if (i < left.length && j < right.length && left[i] === right[j]) {
      result.push({ id: `same-${id++}`, kind: "same", text: left[i] });
      i += 1;
      j += 1;
    } else if (j < right.length && (i === left.length || table[i][j + 1] >= table[i + 1][j])) {
      result.push({ id: `insert-${id++}`, kind: "insert", text: right[j] });
      j += 1;
    } else {
      result.push({ id: `delete-${id++}`, kind: "delete", text: left[i] });
      i += 1;
    }
  }
  return result;
}

function tokenize(text: string): string[] {
  return text.match(/\s+|[\p{L}\p{N}’'-]+|[^\s\p{L}\p{N}]/gu) ?? [];
}
