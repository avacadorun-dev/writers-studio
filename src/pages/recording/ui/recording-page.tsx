import { Link, useNavigate } from "@tanstack/react-router";
import {
  ArrowDown,
  ArrowLeft,
  Check,
  ChevronDown,
  ChevronUp,
  CircleStop,
  Command,
  CornerDownRight,
  Flag,
  Headphones,
  MessageSquareQuote,
  Mic,
  Pause,
  Pencil,
  Pilcrow,
  Play,
  RotateCcw,
  SeparatorHorizontal,
  Undo2,
  Wifi,
  WifiOff,
  X,
} from "lucide-react";
import { useEffect, useRef, useState } from "react";

import {
  groupLiveTranscriptSegments,
  isNavigationCommand,
  type QuickDictationCommandKind,
  resolveVoiceEditTarget,
  useRecorder,
  type VoiceEditTarget,
} from "~features/dictation";
import { Banner, BrandMark, Button, useActionDialog, useToast } from "~shared/ui";

export function RecordingPage({ projectId, localId }: { projectId: string; localId: string }) {
  const navigate = useNavigate();
  const { showToast } = useToast();
  const { requestInput } = useActionDialog();
  const { snapshot, controller } = useRecorder();
  const [finishing, setFinishing] = useState(false);
  const [followLive, setFollowLive] = useState(true);
  const [commandsExpanded, setCommandsExpanded] = useState(true);
  const [announcedText, setAnnouncedText] = useState("");
  const [voiceEditTarget, setVoiceEditTarget] = useState<VoiceEditTarget | null>(null);
  const [voiceEditText, setVoiceEditText] = useState("");
  const transcriptRef = useRef<HTMLDivElement | null>(null);
  const voiceEditRef = useRef<HTMLTextAreaElement | null>(null);
  const handledNavigationRef = useRef("");
  const transcriptUpdateKey = `${snapshot.stableSegments.length}:${snapshot.partial}`;
  const transcriptRuns = groupLiveTranscriptSegments(snapshot.stableSegments);
  const partialJoinsProse = transcriptRuns.at(-1)?.kind === "prose";
  const hasUndoableCommand = snapshot.stableSegments.some(
    (segment) => segment.command?.executed && segment.command.kind !== "undo_command",
  );

  const executeQuickCommand = (kind: QuickDictationCommandKind) => {
    if (!controller.executeCommand(kind)) {
      showToast({ title: "Пока нечего отменять" });
    }
  };

  useEffect(() => {
    if (snapshot.localId !== localId || !snapshot.projectId) {
      void navigate({ to: "/recovery", replace: true });
    }
  }, [localId, navigate, snapshot.localId, snapshot.projectId]);

  useEffect(() => {
    if (followLive && transcriptUpdateKey) {
      transcriptRef.current?.scrollTo({
        top: transcriptRef.current.scrollHeight,
        behavior: "smooth",
      });
    }
  }, [followLive, transcriptUpdateKey]);

  useEffect(() => {
    const latest = snapshot.stableSegments.at(-1);
    if (!latest) {
      return;
    }
    const announcement =
      latest.command?.executed && latest.command.kind !== "undo_command"
        ? `Выполнена команда: ${commandLabel(latest.command.kind)}`
        : latest.command?.kind === "undo_command" && latest.command.executed
          ? "Предыдущая команда отменена"
          : latest.text;
    const timer = window.setTimeout(() => setAnnouncedText(announcement), 500);
    return () => window.clearTimeout(timer);
  }, [snapshot.stableSegments]);

  useEffect(() => {
    const navigationSegment = snapshot.stableSegments.findLast(
      (segment) => segment.command && isNavigationCommand(segment.command),
    );
    if (!navigationSegment?.command) {
      return;
    }
    if (!navigationSegment.command.executed) {
      if (handledNavigationRef.current === navigationSegment.id) {
        handledNavigationRef.current = "";
      }
      return;
    }
    if (handledNavigationRef.current === navigationSegment.id) {
      return;
    }
    handledNavigationRef.current = navigationSegment.id;

    if (navigationSegment.command.kind === "navigate_end") {
      setVoiceEditTarget(null);
      setFollowLive(true);
      window.requestAnimationFrame(() => {
        transcriptRef.current?.scrollTo({
          top: transcriptRef.current.scrollHeight,
          behavior: "smooth",
        });
      });
      return;
    }

    const target = resolveVoiceEditTarget(snapshot.stableSegments, navigationSegment.command);
    if (!target) {
      controller.setCommandExecution(navigationSegment.id, false);
      showToast({
        title: "Не нашли это место",
        description: "Назовите номер абзаца или несколько точных слов из текста.",
      });
      return;
    }

    void controller.pause().then(() => {
      setVoiceEditTarget(target);
      setVoiceEditText(target.text);
      setFollowLive(false);
    });
  }, [controller, showToast, snapshot.stableSegments]);

  useEffect(() => {
    if (!voiceEditTarget) {
      return;
    }
    const frame = window.requestAnimationFrame(() => {
      voiceEditRef.current?.focus();
      voiceEditRef.current?.setSelectionRange(
        voiceEditTarget.selectionStart,
        voiceEditTarget.selectionEnd,
      );
      voiceEditRef.current?.scrollIntoView({ block: "center", behavior: "smooth" });
    });
    return () => window.cancelAnimationFrame(frame);
  }, [voiceEditTarget]);

  const finish = async () => {
    setFinishing(true);
    const result = await controller.stop();
    if (result.sessionId) {
      await navigate({
        to: "/dictations/$sessionId/finalizing",
        params: { sessionId: result.sessionId },
      });
    } else {
      await navigate({ to: "/recovery" });
    }
  };

  const paused = snapshot.status === "paused";
  const degraded =
    snapshot.status === "recording_degraded" ||
    snapshot.connection === "offline" ||
    snapshot.connection === "degraded";

  const completeVoiceEdit = async (resume: boolean) => {
    if (!voiceEditTarget) {
      return;
    }
    if (voiceEditText.trim() !== voiceEditTarget.text.trim()) {
      controller.replaceSegments(voiceEditTarget.segmentIds, voiceEditText);
      showToast({
        title: voiceEditText.trim() ? "Исправление сохранено" : "Фрагмент удалён",
      });
    }
    setVoiceEditTarget(null);
    if (!resume) {
      return;
    }
    setFollowLive(true);
    await controller.resume();
    window.requestAnimationFrame(() => {
      transcriptRef.current?.scrollTo({
        top: transcriptRef.current.scrollHeight,
        behavior: "smooth",
      });
    });
  };

  return (
    <main className="flex h-full min-h-0 flex-col bg-[var(--surface-app)]">
      <header className="flex h-14 items-center gap-2 border-b border-[var(--border-subtle)] bg-[var(--surface-panel)] px-2 sm:px-[18px]">
        <Button asChild variant="ghost" size="icon" aria-label="Вернуться в проект">
          <Link to="/projects/$projectId" params={{ projectId }}>
            <ArrowLeft className="size-4" aria-hidden="true" />
          </Link>
        </Button>
        <BrandMark className="hidden size-7 shrink-0 sm:block" aria-hidden="true" />
        <div className="flex min-w-0 items-center gap-2">
          <p className="truncate text-sm font-medium">{snapshot.projectTitle ?? "Запись"}</p>
          <span className="text-[var(--ink-tertiary)]" aria-hidden="true">
            ›
          </span>
          <p className="hidden truncate text-xs text-[var(--ink-tertiary)] sm:block">
            {snapshot.destinationTitle ?? "Голосовая сессия"}
          </p>
          <span className="hidden rounded-[var(--radius-pill)] bg-[var(--info-soft)] px-3 py-1 text-[11px] text-[var(--info)] md:inline-flex">
            На этом устройстве
          </span>
        </div>
        <div className="ml-auto flex min-h-9 items-center gap-2 rounded-[var(--radius-pill)] bg-[var(--record-surface)] px-4 text-sm font-semibold text-[var(--ink-on-solid)]">
          <span className="size-2 rounded-full bg-current" aria-hidden="true" />
          <span className="sr-only">
            Общая длительность сессии {formatDuration(snapshot.totalDurationMs)}
          </span>
          <span aria-hidden="true">
            {paused ? "Пауза" : "Запись"} · {formatDuration(snapshot.totalDurationMs)}
          </span>
        </div>
      </header>
      <p className="sr-only" role="status" aria-live="polite" aria-atomic="true">
        {paused
          ? "Запись приостановлена"
          : degraded
            ? "Локальная запись продолжается, живая расшифровка недоступна"
            : "Идёт запись и живая расшифровка"}
      </p>

      {degraded ? (
        <Banner
          className="m-3 mb-0"
          variant="warning"
          title={
            snapshot.connection === "offline" ? "Нет соединения" : "Живая расшифровка недоступна"
          }
          body="Запись продолжается и сохраняется на этом устройстве. Завершите её обычной кнопкой."
        />
      ) : null}

      <section
        className="mx-auto flex min-h-0 w-full max-w-[1200px] flex-1 flex-col gap-2 px-3 py-3 sm:gap-3 sm:px-6 sm:py-4"
        aria-label="Живая расшифровка"
      >
        <div className="flex min-h-8 items-center gap-3 text-xs">
          <span className="truncate font-medium">
            {snapshot.destinationTitle ?? "Продолжение рукописи"}
          </span>
          <span className="ml-auto inline-flex shrink-0 items-center gap-1.5 text-[var(--ink-tertiary)]">
            <span
              className={`size-1.5 rounded-full ${
                paused
                  ? "bg-[var(--warning)]"
                  : degraded
                    ? "bg-[var(--warning)]"
                    : "bg-[var(--success)]"
              }`}
              aria-hidden="true"
            />
            {paused ? "На паузе" : degraded ? "Только локальная запись" : "Онлайн · сохраняется"}
          </span>
        </div>
        <div
          ref={transcriptRef}
          onScroll={(event) => {
            const target = event.currentTarget;
            setFollowLive(target.scrollHeight - target.scrollTop - target.clientHeight < 80);
          }}
          className="relative min-h-0 flex-1 overflow-y-auto rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] px-5 py-7 sm:px-12 sm:py-9"
        >
          {snapshot.stableSegments.length === 0 && !snapshot.partial ? (
            <div className="grid h-full place-items-center text-center">
              <div>
                <Mic className="mx-auto size-9 text-[var(--record)]" aria-hidden="true" />
                <h1 className="font-display mt-4 text-3xl">Говорите — мы слушаем</h1>
                <p className="mt-2 text-sm text-[var(--ink-tertiary)]">
                  Стабильные фрагменты появятся здесь. Аудио сохраняется независимо от текста.
                </p>
              </div>
            </div>
          ) : (
            <div className="font-manuscript mx-auto max-w-[68ch] text-xl leading-[1.75] sm:text-2xl">
              {transcriptRuns.map((run) =>
                run.kind === "command" ? (
                  <div
                    key={run.id}
                    className="font-ui my-5 rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-panel)] px-3 py-2 text-sm"
                    role="status"
                    data-command-kind={run.segment.command.kind}
                    data-command-executed={run.segment.command.executed}
                  >
                    {run.segment.command.executed ? (
                      <div className="flex items-center gap-2">
                        <Flag className="size-4 shrink-0 text-[var(--accent)]" aria-hidden="true" />
                        <span className="font-medium text-[var(--accent)]">
                          {run.segment.command.kind === "marker"
                            ? `Метка · ${formatDuration(run.segment.startMs)}`
                            : run.segment.command.kind === "undo_command"
                              ? "Предыдущая команда отменена"
                              : `Выполнено: ${commandLabel(run.segment.command.kind)}`}
                        </span>
                        {run.segment.command.kind !== "marker" ? (
                          <span className="ml-auto font-mono text-[11px] text-[var(--ink-tertiary)]">
                            {formatDuration(run.segment.startMs)}
                          </span>
                        ) : (
                          <span className="ml-auto" />
                        )}
                        {run.segment.command.kind !== "undo_command" &&
                        !isNavigationCommand(run.segment.command) ? (
                          <Button
                            type="button"
                            variant="ghost"
                            size="compact"
                            onClick={() => controller.setCommandExecution(run.segment.id, false)}
                          >
                            <RotateCcw className="size-3.5" aria-hidden="true" />
                            Отменить
                          </Button>
                        ) : null}
                      </div>
                    ) : (
                      <div>
                        <p className="font-manuscript text-xl">{run.segment.text}</p>
                        <div className="mt-2 flex items-center gap-2 text-xs text-[var(--warning)]">
                          Команда не выполнена
                          <Button
                            type="button"
                            variant="ghost"
                            size="compact"
                            onClick={() => controller.setCommandExecution(run.segment.id, true)}
                          >
                            Выполнить
                          </Button>
                        </div>
                      </div>
                    )}
                  </div>
                ) : voiceEditTarget?.runId === run.id ? (
                  <div
                    key={run.id}
                    className="font-ui mb-5 rounded-[var(--radius-md)] border-2 border-[var(--accent)] bg-[var(--accent-soft)] p-3 shadow-[0_10px_30px_var(--shadow-color)]"
                    data-voice-editing="active"
                  >
                    <div className="mb-2 flex items-center gap-2 text-sm">
                      <Pencil className="size-4 text-[var(--accent)]" aria-hidden="true" />
                      <span className="font-semibold">Правка: {voiceEditTarget.label}</span>
                      <span className="ml-auto text-xs text-[var(--ink-tertiary)]">
                        запись на паузе
                      </span>
                    </div>
                    <textarea
                      ref={voiceEditRef}
                      aria-label={`Правка абзаца ${voiceEditTarget.paragraphNumber}`}
                      value={voiceEditText}
                      onChange={(event) => setVoiceEditText(event.target.value)}
                      className="font-manuscript min-h-40 w-full resize-y rounded-[var(--radius-sm)] border border-[var(--border-strong)] bg-[var(--surface-paper)] px-4 py-3 text-xl leading-[1.75] text-[var(--ink-primary)] outline-none focus:border-[var(--accent)] focus:ring-2 focus:ring-[var(--accent-soft)] sm:text-2xl"
                    />
                    <div className="mt-3 flex flex-wrap justify-end gap-2">
                      <Button
                        type="button"
                        variant="ghost"
                        size="compact"
                        onClick={() => setVoiceEditTarget(null)}
                      >
                        <X className="size-3.5" aria-hidden="true" />
                        Отменить
                      </Button>
                      <Button
                        type="button"
                        variant="secondary"
                        size="compact"
                        onClick={() => void completeVoiceEdit(false)}
                      >
                        <Check className="size-3.5" aria-hidden="true" />
                        Сохранить
                      </Button>
                      <Button
                        type="button"
                        size="compact"
                        onClick={() => void completeVoiceEdit(true)}
                      >
                        <CornerDownRight className="size-3.5" aria-hidden="true" />К концу и
                        продолжить
                      </Button>
                    </div>
                  </div>
                ) : (
                  <p key={run.id} className="mb-5" data-transcript-run="prose">
                    {run.segments.map((segment, index) => (
                      <span key={segment.id}>
                        {index > 0 ? " " : null}
                        {segment.text}
                      </span>
                    ))}
                    {snapshot.partial && run === transcriptRuns.at(-1) ? (
                      <span
                        className="ml-1 border-l-2 border-dotted border-[var(--accent)] pl-2 text-[var(--ink-tertiary)]"
                        data-transcript-state="partial"
                      >
                        <span className="sr-only">Предварительный текст: </span>
                        {snapshot.partial}
                        <span className="ml-1 inline-block h-6 w-px bg-[var(--accent)] align-middle" />
                      </span>
                    ) : null}
                  </p>
                ),
              )}
              {snapshot.partial && !partialJoinsProse ? (
                <p
                  className="mb-5 border-l-2 border-dotted border-[var(--accent)] pl-3 text-[var(--ink-tertiary)]"
                  data-transcript-state="partial"
                >
                  <span className="sr-only">Предварительный текст: </span>
                  <span
                    className="font-ui mr-2 inline-flex rounded-full border border-[var(--border-strong)] px-2 py-0.5 align-middle text-[10px] font-semibold tracking-wide uppercase"
                    aria-hidden="true"
                  >
                    Черновик
                  </span>
                  {snapshot.partial}
                  <span className="ml-1 inline-block h-6 w-px bg-[var(--accent)] align-middle" />
                </p>
              ) : null}
            </div>
          )}
          {!followLive ? (
            <Button
              type="button"
              variant="secondary"
              size="compact"
              className="sticky bottom-2 left-1/2 -translate-x-1/2 shadow-[0_8px_20px_var(--shadow-color)]"
              onClick={() => setFollowLive(true)}
            >
              <ArrowDown className="size-4" aria-hidden="true" />К текущему тексту
            </Button>
          ) : null}
        </div>
      </section>

      <div className="sr-only" aria-live="polite" aria-atomic="true">
        {announcedText}
      </div>
      <footer className="px-3 pb-[max(8px,env(safe-area-inset-bottom))]">
        <div className="mx-auto max-w-[1200px] rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-panel)] px-3 py-2.5 sm:px-4">
          <div
            className="mb-2 border-b border-[var(--border-subtle)] pb-2"
            data-testid="dictation-command-panel"
          >
            <button
              type="button"
              className="flex min-h-9 w-full items-center gap-2 rounded-[var(--radius-sm)] px-1 text-left text-xs font-semibold focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus)]"
              aria-expanded={commandsExpanded}
              onClick={() => setCommandsExpanded((current) => !current)}
            >
              <Command className="size-4 text-[var(--accent)]" aria-hidden="true" />
              Команды
              <span className="font-normal text-[var(--ink-tertiary)]">
                голосом с префиксом «{snapshot.commandPrefix}» или кнопкой
              </span>
              {commandsExpanded ? (
                <ChevronUp className="ml-auto size-4" aria-hidden="true" />
              ) : (
                <ChevronDown className="ml-auto size-4" aria-hidden="true" />
              )}
            </button>
            {commandsExpanded ? (
              <div className="mt-1 grid gap-2">
                <fieldset className="flex min-w-0 gap-2 overflow-x-auto border-0 p-0 pb-1">
                  <legend className="sr-only">Кнопки команд диктовки</legend>
                  <Button
                    type="button"
                    variant="secondary"
                    size="compact"
                    className="shrink-0"
                    onClick={() => executeQuickCommand("new_paragraph")}
                  >
                    <Pilcrow className="size-3.5" aria-hidden="true" />
                    Новый абзац
                  </Button>
                  <Button
                    type="button"
                    variant="secondary"
                    size="compact"
                    className="shrink-0"
                    onClick={() => executeQuickCommand("dialogue_start")}
                  >
                    <MessageSquareQuote className="size-3.5" aria-hidden="true" />
                    Начать диалог
                  </Button>
                  <Button
                    type="button"
                    variant="secondary"
                    size="compact"
                    className="shrink-0"
                    onClick={() => executeQuickCommand("scene_break")}
                  >
                    <SeparatorHorizontal className="size-3.5" aria-hidden="true" />
                    Разделитель сцены
                  </Button>
                  <Button
                    type="button"
                    variant="secondary"
                    size="compact"
                    className="shrink-0"
                    onClick={() => controller.addMarker()}
                  >
                    <Flag className="size-3.5" aria-hidden="true" />
                    Метка
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="compact"
                    className="shrink-0"
                    disabled={!hasUndoableCommand}
                    onClick={() => executeQuickCommand("undo_command")}
                  >
                    <Undo2 className="size-3.5" aria-hidden="true" />
                    Отменить команду
                  </Button>
                </fieldset>
                <p className="text-[11px] leading-4 text-[var(--ink-tertiary)]">
                  {snapshot.commandsEnabled ? (
                    <>
                      Голосом: «{snapshot.commandPrefix}, новый абзац» · «{snapshot.commandPrefix},
                      разделитель сцены». Навигация: «{snapshot.commandPrefix}, вернись к первому
                      абзацу» · «{snapshot.commandPrefix}, вернись к словам…» · «
                      {snapshot.commandPrefix}, вернись в конец».
                    </>
                  ) : (
                    "Голосовое распознавание команд отключено для этой записи; кнопки остаются доступны."
                  )}
                </p>
              </div>
            ) : null}
          </div>
          {paused ? (
            <div className="mx-auto mb-3 flex flex-wrap justify-center gap-2">
              <Button
                type="button"
                variant="ghost"
                size="compact"
                onClick={() => {
                  const latest = snapshot.stableSegments.findLast((segment) => !segment.command);
                  if (!latest) {
                    showToast({ title: "Текст ещё не появился" });
                    return;
                  }
                  void requestInput({
                    title: "Исправить последний фрагмент",
                    description: "Изменение сохранится в живом черновике этой записи.",
                    label: "Текст фрагмента",
                    defaultValue: latest.text,
                    icon: Pencil,
                    confirmLabel: "Сохранить",
                  }).then((text) => {
                    if (text?.trim()) {
                      controller.editSegment(latest.id, text.trim());
                      showToast({ title: "Исправление сохранено в живой расшифровке" });
                    }
                  });
                }}
              >
                <Pencil className="size-3.5" aria-hidden="true" />
                Исправить текст
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="compact"
                onClick={() =>
                  void controller.playRecent(10).catch(() => {
                    showToast({ title: "Последние секунды аудио ещё не готовы" });
                  })
                }
              >
                <Headphones className="size-3.5" aria-hidden="true" />
                Прослушать последние 10 секунд
              </Button>
            </div>
          ) : null}
          <div className="mx-auto flex items-center justify-between gap-2">
            <div className="hidden items-center gap-2 text-xs text-[var(--ink-tertiary)] sm:flex">
              {degraded ? (
                <WifiOff className="size-4" aria-hidden="true" />
              ) : (
                <Wifi className="size-4 text-[var(--success)]" aria-hidden="true" />
              )}
              {snapshot.uploadedChunks}/{snapshot.totalChunks} фрагментов на сервере
            </div>
            <Button
              type="button"
              variant="secondary"
              aria-pressed={paused}
              onClick={() => void (paused ? controller.resume() : controller.pause())}
            >
              {paused ? (
                <Play className="size-4" aria-hidden="true" />
              ) : (
                <Pause className="size-4" aria-hidden="true" />
              )}
              {paused ? "Продолжить" : "Пауза"}
            </Button>
            <Button
              type="button"
              variant="record"
              disabled={finishing}
              onClick={() => void finish()}
            >
              <CircleStop className="size-4" aria-hidden="true" />
              {finishing ? "Сохраняем…" : "Завершить"}
            </Button>
          </div>
          <p className="mt-2 text-center text-[11px] text-[var(--ink-tertiary)]">
            {paused
              ? voiceEditTarget
                ? "Исправьте выделенный текст и вернитесь к концу"
                : "Запись приостановлена"
              : `${formatDuration(snapshot.recordedDurationMs)} записанного аудио`}
          </p>
        </div>
      </footer>
      <Link to="/projects/$projectId" params={{ projectId }} className="sr-only focus:not-sr-only">
        Вернуться без завершения
      </Link>
    </main>
  );
}

function formatDuration(milliseconds: number): string {
  const total = Math.max(0, Math.floor(milliseconds / 1_000));
  const minutes = Math.floor(total / 60);
  const seconds = total % 60;
  return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
}

function commandLabel(
  kind:
    | "new_paragraph"
    | "dialogue_start"
    | "scene_break"
    | "marker"
    | "undo_command"
    | "navigate_paragraph"
    | "navigate_sentence"
    | "navigate_text"
    | "navigate_end",
): string {
  return {
    new_paragraph: "новый абзац",
    dialogue_start: "начало диалога",
    scene_break: "разделитель сцены",
    marker: "метка",
    undo_command: "отменить команду",
    navigate_paragraph: "переход к абзацу",
    navigate_sentence: "переход к предложению",
    navigate_text: "переход к словам",
    navigate_end: "переход в конец",
  }[kind];
}
