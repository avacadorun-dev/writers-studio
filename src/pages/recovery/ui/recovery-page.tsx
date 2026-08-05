import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import {
  ArrowLeft,
  CloudUpload,
  Download,
  FileAudio,
  HardDrive,
  Play,
  RotateCcw,
  Trash2,
} from "lucide-react";

import { getRecoveryItems, retryDictation } from "~entities/dictation";
import { synchronizeLocalRecording } from "~features/dictation";
import {
  deleteLocalRecording,
  type LocalRecording,
  listAudioChunks,
  listLocalRecordings,
} from "~shared/lib";
import { Banner, Button, Spinner, useActionDialog, useToast } from "~shared/ui";
import { RequireSession } from "~widgets/require-session";
import { WorkspaceHeader } from "~widgets/workspace-header";

export function RecoveryPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { showToast } = useToast();
  const { requestInput } = useActionDialog();
  const local = useQuery({
    queryKey: ["recovery-local"],
    queryFn: listLocalRecordings,
    networkMode: "always",
    staleTime: 0,
    refetchOnMount: "always",
  });
  const remote = useQuery({ queryKey: ["recovery-remote"], queryFn: getRecoveryItems });
  const sync = useMutation({
    mutationFn: synchronizeLocalRecording,
    onSuccess: (sessionId) => {
      void queryClient.invalidateQueries({ queryKey: ["recovery-local"] });
      void queryClient.invalidateQueries({ queryKey: ["recovery-remote"] });
      void navigate({
        to: "/dictations/$sessionId/finalizing",
        params: { sessionId },
      });
    },
  });
  const retry = useMutation({
    mutationFn: retryDictation,
    onSuccess: (_response, sessionId) =>
      void navigate({
        to: "/dictations/$sessionId/finalizing",
        params: { sessionId },
      }),
  });

  return (
    <RequireSession>
      <div className="min-h-full">
        <WorkspaceHeader />
        <main className="mx-auto w-full max-w-[1280px] px-5 py-10 lg:px-0">
          <Button
            asChild
            variant="ghost"
            size="icon"
            className="mb-4"
            aria-label="Назад к проектам"
          >
            <Link to="/projects">
              <ArrowLeft className="size-4" aria-hidden="true" />
            </Link>
          </Button>
          <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
            <div>
              <p className="text-[10px] font-semibold tracking-[0.14em] text-[var(--ink-tertiary)] uppercase">
                Сохранённая работа на устройствах
              </p>
              <h1 className="font-display mt-2 text-[38px] leading-tight">Центр восстановления</h1>
              <p className="mt-2 text-sm text-[var(--ink-secondary)]">
                Незавершённые записи и данные, которые ещё не синхронизированы.
              </p>
            </div>
            <Button
              type="button"
              variant="secondary"
              disabled={local.isFetching || remote.isFetching}
              onClick={() => void Promise.all([local.refetch(), remote.refetch()])}
            >
              <RotateCcw className="size-4" aria-hidden="true" />
              Проверить снова
            </Button>
          </div>
          <Banner
            className="mt-6"
            variant="info"
            title="Эти записи не являются кэшем"
            body="Они останутся здесь, пока вы их не восстановите или не удалите сами."
          />
          <div className="mt-6 grid gap-6 lg:grid-cols-2">
            <section>
              <h2 className="flex items-center gap-2 text-sm font-semibold">
                <HardDrive className="size-4 text-[var(--accent)]" aria-hidden="true" />
                На этом устройстве
              </h2>
              {local.isPending ? (
                <Spinner label="Проверяем локальное хранилище…" />
              ) : (
                <div className="mt-3 grid gap-3">
                  {local.data?.map((recording) => (
                    <RecoveryCard
                      key={recording.localId}
                      recording={recording}
                      busy={sync.isPending}
                      onSync={() => sync.mutate(recording.localId)}
                      onListen={() =>
                        void playLocalRecording(recording).catch(() =>
                          showToast({ title: "Локальное аудио пока недоступно" }),
                        )
                      }
                      onDownload={() => void downloadLocalRecording(recording)}
                      onDelete={async () => {
                        const confirmation = await requestInput({
                          title: "Удалить запись с этого устройства?",
                          description:
                            "Эта копия существует только локально. После удаления восстановить аудио и расшифровку будет невозможно.",
                          label: "Подтверждение",
                          requiredValue: "УДАЛИТЬ",
                          icon: HardDrive,
                          tone: "danger",
                          confirmLabel: "Удалить безвозвратно",
                        });
                        if (confirmation !== "УДАЛИТЬ") {
                          return;
                        }
                        await deleteLocalRecording(recording.localId);
                        await queryClient.invalidateQueries({ queryKey: ["recovery-local"] });
                        showToast({
                          title: "Локальная запись удалена",
                          description:
                            "Это действие нельзя отменить. Серверная копия, если она была, не затронута.",
                        });
                      }}
                    />
                  ))}
                  {local.data?.length === 0 ? (
                    <EmptyRecovery text="Несинхронизированных локальных записей нет." />
                  ) : null}
                </div>
              )}
            </section>
            <section>
              <h2 className="flex items-center gap-2 text-sm font-semibold">
                <CloudUpload className="size-4 text-[var(--accent)]" aria-hidden="true" />
                На сервере
              </h2>
              <div className="mt-3 grid gap-3">
                {remote.data?.items.map((item) => (
                  <article
                    key={item.sessionId}
                    className="flex flex-col gap-3 rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-4 sm:flex-row sm:items-center"
                  >
                    <span className="grid size-10 shrink-0 place-items-center rounded-full bg-[var(--surface-subtle)]">
                      <FileAudio className="size-4" aria-hidden="true" />
                    </span>
                    <div className="min-w-0 flex-1">
                      <h3 className="text-sm font-medium">{item.projectTitle}</h3>
                      <p className="text-xs text-[var(--ink-tertiary)]">
                        {item.destinationTitle ?? "Без места вставки"} ·{" "}
                        {formatDuration(item.durationMs)} · {item.status}
                      </p>
                    </div>
                    {item.status === "review_ready" ? (
                      <Button asChild variant="secondary" size="compact">
                        <Link
                          to="/dictations/$sessionId/review"
                          params={{ sessionId: item.sessionId }}
                        >
                          Открыть черновик
                        </Link>
                      </Button>
                    ) : ["finalization_failed", "missing_chunks"].includes(item.status) ? (
                      <Button
                        type="button"
                        variant="secondary"
                        size="compact"
                        disabled={retry.isPending}
                        onClick={() => retry.mutate(item.sessionId)}
                      >
                        <RotateCcw className="size-4" aria-hidden="true" />
                        Повторить
                      </Button>
                    ) : (
                      <Button asChild variant="secondary" size="compact">
                        <Link
                          to="/dictations/$sessionId/finalizing"
                          params={{ sessionId: item.sessionId }}
                        >
                          Проверить
                        </Link>
                      </Button>
                    )}
                  </article>
                ))}
                {!remote.isPending && remote.data?.items.length === 0 ? (
                  <EmptyRecovery text="Серверных записей для восстановления нет." />
                ) : null}
              </div>
            </section>
          </div>
          {sync.error ? (
            <Banner
              className="mt-5"
              variant="danger"
              title="Синхронизация не завершена"
              body={`${sync.error.message}. Локальная копия сохранена.`}
            />
          ) : null}
        </main>
      </div>
    </RequireSession>
  );
}

function RecoveryCard({
  recording,
  busy,
  onSync,
  onListen,
  onDownload,
  onDelete,
}: {
  recording: LocalRecording;
  busy: boolean;
  onSync: () => void;
  onListen: () => void;
  onDownload: () => void;
  onDelete: () => void;
}) {
  return (
    <article className="rounded-[var(--radius-md)] border border-[var(--warning)] bg-[var(--warning-soft)] p-4">
      <div className="flex items-start gap-3">
        <FileAudio className="mt-0.5 size-5 shrink-0 text-[var(--warning)]" aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-semibold">{recording.projectTitle}</h3>
          <p className="mt-1 text-xs text-[var(--ink-secondary)]">
            {recording.destinationTitle ?? "Голосовая сессия"} ·{" "}
            {formatDuration(recording.durationMs)} · {recording.status}
          </p>
        </div>
      </div>
      <div className="mt-4 flex flex-wrap gap-2">
        {recording.status !== "uploaded" ? (
          <Button
            type="button"
            size="compact"
            disabled={busy || !navigator.onLine}
            onClick={onSync}
          >
            <CloudUpload className="size-4" aria-hidden="true" />
            Синхронизировать
          </Button>
        ) : null}
        <Button type="button" variant="secondary" size="compact" onClick={onListen}>
          <Play className="size-4" aria-hidden="true" />
          Прослушать
        </Button>
        <Button type="button" variant="secondary" size="compact" onClick={onDownload}>
          <Download className="size-4" aria-hidden="true" />
          Скачать аудио
        </Button>
        <Button type="button" variant="ghost" size="compact" onClick={onDelete}>
          <Trash2 className="size-4" aria-hidden="true" />
          Удалить с устройства
        </Button>
      </div>
    </article>
  );
}

function EmptyRecovery({ text }: { text: string }) {
  return (
    <div className="rounded-[var(--radius-md)] border border-dashed border-[var(--border-strong)] p-7 text-center text-sm text-[var(--ink-tertiary)]">
      {text}
    </div>
  );
}

async function downloadLocalRecording(recording: LocalRecording): Promise<void> {
  const blob = await localRecordingBlob(recording);
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = `writers-studio-${recording.localId}.${recording.format}`;
  anchor.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 1_000);
}

async function playLocalRecording(recording: LocalRecording): Promise<void> {
  const blob = await localRecordingBlob(recording);
  if (blob.size === 0) {
    throw new Error("empty recording");
  }
  const url = URL.createObjectURL(blob);
  const audio = new Audio(url);
  audio.addEventListener("ended", () => URL.revokeObjectURL(url), { once: true });
  audio.addEventListener("error", () => URL.revokeObjectURL(url), { once: true });
  await audio.play();
}

async function localRecordingBlob(recording: LocalRecording): Promise<Blob> {
  const chunks = await listAudioChunks(recording.localId);
  return new Blob(
    chunks.sort((left, right) => left.sequence - right.sequence).map((chunk) => chunk.blob),
    { type: recording.format === "ogg" ? "audio/ogg" : "audio/webm" },
  );
}

function formatDuration(milliseconds: number): string {
  const seconds = Math.floor(milliseconds / 1_000);
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`;
}
