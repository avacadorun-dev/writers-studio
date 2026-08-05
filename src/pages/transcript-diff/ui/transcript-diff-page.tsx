import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { Check, Headphones, RotateCcw, ShieldCheck } from "lucide-react";
import { useMemo, useRef, useState } from "react";

import { getDictation, getDictationAudio, saveTranscriptDraft } from "~entities/dictation";
import { getProject, type TiptapNode } from "~entities/project";
import { createClientId } from "~shared/lib";
import { Banner, Button, Checkbox, Spinner, useToast } from "~shared/ui";
import { ProjectUtilityShell } from "~widgets/project-utility-shell";

interface DiffToken {
  id: string;
  kind: "same" | "insert" | "delete";
  text: string;
  group?: number;
}

export function TranscriptDiffPage({ sessionId }: { sessionId: string }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { showToast } = useToast();
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const [acceptedGroups, setAcceptedGroups] = useState<Set<number>>(new Set());
  const session = useQuery({
    queryKey: ["dictation", sessionId],
    queryFn: () => getDictation(sessionId),
  });
  const audio = useQuery({
    queryKey: ["dictation-audio", sessionId],
    queryFn: () => getDictationAudio(sessionId),
    enabled: session.data?.session.status === "review_ready",
  });
  const project = useQuery({
    queryKey: ["project", session.data?.session.projectId],
    queryFn: () => getProject(session.data?.session.projectId ?? ""),
    enabled: Boolean(session.data?.session.projectId),
  });
  const draft = session.data?.session.draft;
  const revised = session.data?.session.suggestionArtifact ?? session.data?.session.batchArtifact;
  const diff = useMemo(
    () => wordDiff(draft?.plainText ?? "", revised?.text ?? draft?.plainText ?? ""),
    [draft?.plainText, revised?.text],
  );
  const groups = useMemo(
    () => Array.from(new Set(diff.flatMap((token) => token.group ?? []))),
    [diff],
  );
  const apply = useMutation({
    mutationFn: () => {
      const text = diff
        .filter((token) => {
          if (token.kind === "same") {
            return true;
          }
          const accepted = token.group !== undefined && acceptedGroups.has(token.group);
          return token.kind === "insert" ? accepted : !accepted;
        })
        .map((token) => token.text)
        .join("")
        .replace(/\s+([.,!?;:])/gu, "$1")
        .trim();
      const content: TiptapNode = {
        type: "doc",
        content: text.split(/\n{2,}/u).map((paragraph, index) => ({
          type: "paragraph",
          attrs: {
            blockId: createClientId(),
            sourceRefs: revised?.segments[index]?.id ? [revised.segments[index].id] : [],
          },
          content: paragraph ? [{ type: "text", text: paragraph }] : [],
        })),
      };
      return saveTranscriptDraft(sessionId, content, "final_batch_apply");
    },
    onSuccess: async () => {
      const previousContent = draft?.content;
      await queryClient.invalidateQueries({ queryKey: ["dictation", sessionId] });
      showToast({
        title: "Выбранные изменения применены",
        description: "Исходная расшифровка сохранена отдельно.",
        actionLabel: previousContent ? "Отменить" : undefined,
        onAction: previousContent
          ? () => {
              void saveTranscriptDraft(sessionId, previousContent, "final_batch_undo").then(
                async () => {
                  await queryClient.invalidateQueries({ queryKey: ["dictation", sessionId] });
                  showToast({ title: "Применение отменено" });
                },
              );
            }
          : undefined,
      });
      await navigate({
        to: "/dictations/$sessionId/review",
        params: { sessionId },
      });
    },
  });

  if (session.isPending) {
    return <Spinner label="Сравниваем варианты…" />;
  }
  if (!draft) {
    return (
      <main className="grid min-h-full place-items-center p-5">
        <Banner
          variant="warning"
          title="Нет черновика для сравнения"
          body="Дождитесь обработки записи."
        />
      </main>
    );
  }
  if (project.isPending) {
    return <Spinner label="Открываем проект…" />;
  }
  if (!project.data || !session.data?.session) {
    return (
      <main className="grid min-h-full place-items-center p-5">
        <Banner variant="danger" title="Проект недоступен" body="Сравнение сохранено." />
      </main>
    );
  }

  return (
    <ProjectUtilityShell
      project={project.data.project}
      nodes={project.data.nodes}
      activeNodeId={session.data.session.destinationNodeId}
    >
      <main className="mx-auto min-h-full w-full max-w-[1200px] px-4 py-7">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p className="text-[10px] font-semibold tracking-[0.14em] text-[var(--ink-tertiary)] uppercase">
              Сравнение расшифровок
            </p>
            <h1 className="font-display mt-2 text-[34px] leading-tight">
              Что уточнила финальная расшифровка
            </h1>
          </div>
          <Button type="button" variant="secondary" onClick={() => window.history.back()}>
            Закрыть
          </Button>
        </div>
        <Banner
          className="mt-5"
          variant="info"
          title="Ваш текущий текст не изменится, пока вы не примените правки"
          body="Можно принять только выбранные группы."
        />
        <div className="mt-5 grid gap-5 lg:grid-cols-[minmax(0,1fr)_320px]">
          <section className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-5 sm:p-8">
            <p className="text-xs font-semibold tracking-[0.14em] text-[var(--ink-tertiary)] uppercase">
              Уточнённая версия
            </p>
            <div className="font-manuscript mt-5 text-xl leading-[1.9]">
              {diff.map((token) => {
                const accepted = token.group === undefined || acceptedGroups.has(token.group);
                return (
                  <span
                    key={token.id}
                    className={
                      token.kind === "insert"
                        ? accepted
                          ? "rounded-sm bg-[var(--diff-insert)]"
                          : "text-[var(--ink-tertiary)] underline decoration-dotted"
                        : token.kind === "delete"
                          ? accepted
                            ? "text-[var(--ink-tertiary)] line-through"
                            : "rounded-sm bg-[var(--diff-delete)] line-through"
                          : ""
                    }
                  >
                    {token.text}
                  </span>
                );
              })}
            </div>
          </section>
          <aside className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-5">
            <h2 className="text-sm font-semibold">{groups.length} групп изменений</h2>
            <p className="mt-1 text-xs leading-5 text-[var(--ink-tertiary)]">
              Выберите только те уточнения, которые хотите внести в редактируемый черновик.
            </p>
            <div className="mt-4 grid gap-2">
              {groups.map((group, index) => {
                const checked = acceptedGroups.has(group);
                const range = audioRangeForGroup(diff, group, revised?.segments ?? []);
                return (
                  <div
                    key={group}
                    className="rounded-[var(--radius-sm)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-3 text-sm"
                  >
                    <label
                      htmlFor={`diff-group-${index}`}
                      className="flex min-h-7 cursor-pointer items-center gap-3"
                    >
                      <Checkbox
                        id={`diff-group-${index}`}
                        checked={checked}
                        onCheckedChange={() =>
                          setAcceptedGroups((current) => {
                            const next = new Set(current);
                            if (next.has(group)) {
                              next.delete(group);
                            } else {
                              next.add(group);
                            }
                            return next;
                          })
                        }
                      />
                      Изменение {index + 1}
                    </label>
                    {range && audio.data ? (
                      <button
                        type="button"
                        className="mt-2 inline-flex min-h-8 items-center gap-2 rounded px-1 text-xs text-[var(--accent)] underline-offset-4 hover:underline"
                        onClick={() => {
                          if (audioRef.current) {
                            playAudioRange(audioRef.current, range.startMs, range.endMs);
                          }
                        }}
                      >
                        <Headphones className="size-3.5" aria-hidden="true" />
                        Аудио · {formatClock(range.startMs)}–{formatClock(range.endMs)}
                      </button>
                    ) : null}
                  </div>
                );
              })}
            </div>
            {audio.data ? (
              <audio ref={audioRef} className="sr-only" preload="metadata" src={audio.data.url}>
                <track kind="captions" />
              </audio>
            ) : null}
            <div className="mt-5 grid gap-2">
              <Button
                type="button"
                disabled={acceptedGroups.size === 0 || apply.isPending}
                onClick={() => apply.mutate()}
              >
                <Check className="size-4" aria-hidden="true" />
                {apply.isPending ? "Применяем…" : "Применить выбранное"}
              </Button>
              <Button type="button" variant="ghost" onClick={() => setAcceptedGroups(new Set())}>
                <RotateCcw className="size-4" aria-hidden="true" />
                Сбросить выбор
              </Button>
            </div>
            <div className="mt-6 flex items-start gap-2 border-t border-[var(--border-subtle)] pt-4 text-xs leading-5 text-[var(--ink-tertiary)]">
              <ShieldCheck className="mt-0.5 size-4 shrink-0 text-[var(--accent)]" aria-hidden />
              Исходная и текущая версии останутся в истории.
            </div>
          </aside>
        </div>
      </main>
    </ProjectUtilityShell>
  );
}

function wordDiff(before: string, after: string): DiffToken[] {
  const left = tokenize(before);
  const right = tokenize(after);
  const rows = left.length + 1;
  const columns = right.length + 1;
  const table = Array.from({ length: rows }, () => new Uint32Array(columns));
  for (let i = left.length - 1; i >= 0; i -= 1) {
    for (let j = right.length - 1; j >= 0; j -= 1) {
      table[i][j] =
        left[i] === right[j] ? table[i + 1][j + 1] + 1 : Math.max(table[i + 1][j], table[i][j + 1]);
    }
  }
  const output: DiffToken[] = [];
  let i = 0;
  let j = 0;
  let group = 0;
  let changing = false;
  let serial = 0;
  while (i < left.length || j < right.length) {
    if (i < left.length && j < right.length && left[i] === right[j]) {
      output.push({ id: `same-${serial++}`, kind: "same", text: left[i] });
      i += 1;
      j += 1;
      changing = false;
    } else {
      if (!changing) {
        group += 1;
        changing = true;
      }
      if (j < right.length && (i === left.length || table[i][j + 1] >= table[i + 1][j])) {
        output.push({ id: `insert-${serial++}`, kind: "insert", text: right[j], group });
        j += 1;
      } else {
        output.push({ id: `delete-${serial++}`, kind: "delete", text: left[i], group });
        i += 1;
      }
    }
  }
  return output;
}

function tokenize(text: string): string[] {
  return text.match(/\s+|[\p{L}\p{N}’'-]+|[^\s\p{L}\p{N}]/gu) ?? [];
}

function audioRangeForGroup(
  tokens: DiffToken[],
  group: number,
  segments: Array<{ text: string; startMs: number; endMs: number }>,
): { startMs: number; endMs: number } | undefined {
  const changedWords = tokens
    .filter((token) => token.group === group)
    .map((token) => token.text.trim().toLocaleLowerCase())
    .filter(Boolean);
  const matching = segments.find((segment) =>
    changedWords.some((word) => segment.text.toLocaleLowerCase().includes(word)),
  );
  return matching ? { startMs: matching.startMs, endMs: matching.endMs } : undefined;
}

function formatClock(milliseconds: number): string {
  const seconds = Math.floor(milliseconds / 1_000);
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`;
}

function playAudioRange(audio: HTMLAudioElement, startMs: number, endMs: number): void {
  audio.currentTime = startMs / 1_000;
  audio.ontimeupdate = () => {
    if (audio.currentTime * 1_000 >= endMs) {
      audio.pause();
      audio.ontimeupdate = null;
    }
  };
  void audio.play();
}
