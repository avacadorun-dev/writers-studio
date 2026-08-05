import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { Check, FileAudio, LoaderCircle, RotateCcw, ShieldCheck } from "lucide-react";
import { useEffect } from "react";

import { getDictation, retryDictation } from "~entities/dictation";
import { getProject } from "~entities/project";
import { Banner, Button, Spinner } from "~shared/ui";
import { ProjectUtilityShell } from "~widgets/project-utility-shell";
import { RequireSession } from "~widgets/require-session";

export function FinalizingPage({ sessionId }: { sessionId: string }) {
  const navigate = useNavigate();
  const session = useQuery({
    queryKey: ["dictation", sessionId],
    queryFn: () => getDictation(sessionId),
    refetchInterval: (query) => (query.state.data?.session.status === "finalizing" ? 1_500 : false),
  });
  const retry = useMutation({
    mutationFn: () => retryDictation(sessionId),
    onSuccess: () => void session.refetch(),
  });
  const project = useQuery({
    queryKey: ["project", session.data?.session.projectId],
    queryFn: () => getProject(session.data?.session.projectId ?? ""),
    enabled: Boolean(session.data?.session.projectId),
  });

  useEffect(() => {
    if (session.data?.session.status === "review_ready") {
      void navigate({
        to: "/dictations/$sessionId/review",
        params: { sessionId },
        replace: true,
      });
    }
  }, [navigate, session.data?.session.status, sessionId]);

  if (session.isPending) {
    return <Spinner label="Проверяем запись…" />;
  }
  const data = session.data?.session;
  if (!data || session.isError) {
    return (
      <main className="grid min-h-full place-items-center p-5">
        <Banner
          variant="danger"
          title="Не удалось открыть запись"
          body="Запись не удалена. Проверьте раздел восстановления."
          action={
            <Button asChild variant="secondary" size="compact">
              <Link to="/recovery">Восстановление</Link>
            </Button>
          }
        />
      </main>
    );
  }
  const failed = ["finalization_failed", "missing_chunks"].includes(data.status);
  const content = (
    <main className="flex min-h-full justify-center px-5 pt-16 pb-10">
      <div className="w-full max-w-[560px]">
        <div className="text-center">
          <p className="text-[10px] font-semibold tracking-[0.14em] text-[var(--accent)] uppercase">
            Сессия сохранена · {formatDuration(data.durationMs)}
          </p>
          <h1 className="font-display mt-3 text-[38px] leading-tight">
            {failed ? "Аудио сохранено" : "Запись уже в проекте"}
          </h1>
          <p className="mt-3 text-sm leading-6 text-[var(--ink-secondary)]">
            {failed
              ? "Исходная запись не потеряна — обработку можно безопасно повторить."
              : "Готовим точную расшифровку. Это не мешает продолжить работу с рукописью."}
          </p>
        </div>
        <section className="mt-7 rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-6 text-left sm:p-7">
          <div className="flex items-center gap-3">
            <span className="grid size-10 place-items-center rounded-full bg-[var(--accent-soft)] text-[var(--accent)]">
              {failed ? (
                <FileAudio className="size-5" aria-hidden="true" />
              ) : (
                <LoaderCircle className="size-5 animate-spin" aria-hidden="true" />
              )}
            </span>
            <h2 className="font-display text-2xl">
              {failed ? "Нужна повторная обработка" : "Готовим точную расшифровку"}
            </h2>
          </div>
          <p className="mt-4 text-sm leading-6 text-[var(--ink-secondary)]">
            {failed
              ? "Расшифровка не завершилась, но исходная запись осталась неизменной."
              : "Обработка завершится в фоне. Исходное аудио и живой текст уже сохранены."}
          </p>
          <ol className="mt-5 grid gap-3 text-sm">
            <ProgressRow done label="Сохраняем запись" />
            <ProgressRow done={!failed} label="Проверяем аудио" />
            <ProgressRow done={!failed} active={!failed} label="Готовим точную расшифровку" />
            <ProgressRow label="Готово" />
          </ol>
          <div className="mt-7 flex flex-wrap justify-end gap-2">
            {data.draft ? (
              <Button asChild>
                <Link to="/dictations/$sessionId/review" params={{ sessionId }}>
                  Проверить живой черновик
                </Link>
              </Button>
            ) : null}
            {failed ? (
              <Button type="button" disabled={retry.isPending} onClick={() => retry.mutate()}>
                <RotateCcw className="size-4" aria-hidden="true" />
                {retry.isPending ? "Запускаем…" : "Повторить обработку"}
              </Button>
            ) : null}
            <Button asChild variant="ghost">
              <Link to="/projects/$projectId" params={{ projectId: data.projectId }}>
                Вернуться в проект
              </Link>
            </Button>
          </div>
        </section>
        <div className="mt-5 flex items-center justify-center gap-2 text-xs text-[var(--ink-tertiary)]">
          <ShieldCheck className="size-4" aria-hidden="true" />
          Сообщим, когда расшифровка будет готова
        </div>
      </div>
    </main>
  );

  return project.data ? (
    <ProjectUtilityShell
      project={project.data.project}
      nodes={project.data.nodes}
      activeNodeId={data.destinationNodeId}
    >
      {content}
    </ProjectUtilityShell>
  ) : (
    <RequireSession>{content}</RequireSession>
  );
}

function ProgressRow({
  done = false,
  active = false,
  label,
}: {
  done?: boolean;
  active?: boolean;
  label: string;
}) {
  return (
    <li className="flex items-center gap-3">
      <span
        className={
          done
            ? "grid size-6 place-items-center rounded-full bg-[var(--success-soft)] text-[var(--success)]"
            : "grid size-6 place-items-center rounded-full border border-[var(--border-strong)] text-[var(--ink-tertiary)]"
        }
      >
        {active ? (
          <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
        ) : done ? (
          <Check className="size-3.5" aria-hidden="true" />
        ) : (
          <span className="size-1.5 rounded-full bg-current" />
        )}
      </span>
      {label}
    </li>
  );
}

function formatDuration(milliseconds: number): string {
  const seconds = Math.floor(milliseconds / 1_000);
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`;
}
