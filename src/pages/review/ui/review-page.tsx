import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { EditorContent, useEditor } from "@tiptap/react";
import { GitCompareArrows, ListPlus, RefreshCw, Save, Search, Send } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import {
  getDictation,
  getDictationAudio,
  retranscribeDictation,
  saveTranscriptDraft,
  transferTranscript,
} from "~entities/dictation";
import { createNode, getNode, getProject, saveRevision, type TiptapNode } from "~entities/project";
import { editorExtensions, FormattingToolbar, highlightSourceNode } from "~features/editor";
import { loadWriterPreferences } from "~shared/lib";
import {
  Banner,
  Button,
  Field,
  ModalDialog,
  type SaveState,
  Select,
  Spinner,
  useActionDialog,
  useToast,
} from "~shared/ui";
import { ProjectUtilityShell } from "~widgets/project-utility-shell";

import { mergeTranscriptAtEnd } from "../model/transcript-transfer";

export function ReviewPage({ sessionId }: { sessionId: string }) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { showToast } = useToast();
  const { requestInput } = useActionDialog();
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const saveTimer = useRef<number | undefined>(undefined);
  const [saveState, setSaveState] = useState<SaveState>("saved");
  const [transferOpen, setTransferOpen] = useState(false);
  const [destination, setDestination] = useState("");
  const [mode, setMode] = useState<"append" | "replace" | "new_scene">("append");
  const [retranscriptionBaseline, setRetranscriptionBaseline] = useState<string | null>(null);
  const [writerPreferences] = useState(loadWriterPreferences);
  const session = useQuery({
    queryKey: ["dictation", sessionId],
    queryFn: () => getDictation(sessionId),
    refetchInterval: retranscriptionBaseline === null ? false : 800,
  });
  const audio = useQuery({
    queryKey: ["dictation-audio", sessionId],
    queryFn: () => getDictationAudio(sessionId),
    enabled: ["review_ready", "finalization_failed", "missing_chunks"].includes(
      session.data?.session.status ?? "",
    ),
  });
  const project = useQuery({
    queryKey: ["project", session.data?.session.projectId],
    queryFn: () => getProject(session.data?.session.projectId ?? ""),
    enabled: Boolean(session.data?.session.projectId),
  });
  const initialContent = session.data?.session.draft?.content;
  const editor = useEditor({
    extensions: editorExtensions,
    content: initialContent,
    immediatelyRender: false,
    editorProps: {
      attributes: {
        "aria-label": "Редактируемый черновик расшифровки",
        spellcheck: String(writerPreferences.spellcheck),
      },
    },
  });

  useEffect(() => {
    if (editor && initialContent) {
      editor.commands.setContent(initialContent, { emitUpdate: false });
    }
  }, [editor, initialContent]);

  useEffect(() => {
    if (!editor) {
      return;
    }
    const handleUpdate = () => {
      setSaveState("saving");
      window.clearTimeout(saveTimer.current);
      saveTimer.current = window.setTimeout(async () => {
        try {
          await saveTranscriptDraft(sessionId, editor.getJSON() as TiptapNode);
          setSaveState("saved");
        } catch {
          setSaveState(navigator.onLine ? "pending" : "local");
        }
      }, 1_000);
    };
    editor.on("update", handleUpdate);
    return () => {
      editor.off("update", handleUpdate);
      window.clearTimeout(saveTimer.current);
    };
  }, [editor, sessionId]);

  const uncertain = useMemo(
    () =>
      (
        session.data?.session.suggestionArtifact?.segments ??
        session.data?.session.batchArtifact?.segments
      )?.filter((segment) => segment.confidence !== undefined && segment.confidence < 0.75) ?? [],
    [
      session.data?.session.batchArtifact?.segments,
      session.data?.session.suggestionArtifact?.segments,
    ],
  );
  const retranscribe = useMutation({
    mutationFn: () => retranscribeDictation(sessionId),
    onSuccess: () => {
      setRetranscriptionBaseline(session.data?.session.suggestionArtifact?.id ?? "");
      showToast({
        title: "Повторная расшифровка запущена",
        description: "Исходник и ваши правки останутся без изменений.",
      });
    },
  });
  useEffect(() => {
    const suggestionID = session.data?.session.suggestionArtifact?.id;
    if (
      retranscriptionBaseline !== null &&
      session.data?.session.retranscriptionStatus === "failed"
    ) {
      setRetranscriptionBaseline(null);
      showToast({
        title: "Повторная расшифровка не завершена",
        description: "Аудио, исходные расшифровки и ваши правки сохранены. Попробуйте ещё раз.",
      });
      return;
    }
    if (
      retranscriptionBaseline !== null &&
      suggestionID &&
      suggestionID !== retranscriptionBaseline
    ) {
      setRetranscriptionBaseline(null);
      showToast({
        title: "Новая расшифровка готова",
        description: "Она сохранена отдельным предложением. Сравните и выберите нужные изменения.",
      });
    }
  }, [
    retranscriptionBaseline,
    session.data?.session.retranscriptionStatus,
    session.data?.session.suggestionArtifact?.id,
    showToast,
  ]);
  const playParagraphSource = (target: EventTarget | null) => {
    const element = (target as HTMLElement | null)?.closest<HTMLElement>("[data-source-refs]");
    const refs = element?.dataset.sourceRefs?.split(",").filter(Boolean) ?? [];
    if (!element || refs.length === 0) {
      return;
    }
    const segments = [
      ...(session.data?.session.suggestionArtifact?.segments ?? []),
      ...(session.data?.session.batchArtifact?.segments ?? []),
      ...(session.data?.session.liveArtifact?.segments ?? []),
    ];
    if (editor) {
      highlightSourceNode(editor, element);
    }
    const segment = segments.find((candidate) => refs.includes(candidate.id));
    if (!segment) {
      return;
    }
    if (audioRef.current) {
      playAudioRange(audioRef.current, segment.startMs, segment.endMs);
    }
  };
  const transfer = useMutation({
    mutationFn: async () => {
      if (!editor || !project.data) {
        throw new Error("Черновик ещё не готов");
      }
      let target = project.data.nodes.find((node) => node.id === destination);
      if (mode === "new_scene") {
        const created = await createNode(project.data.project.id, {
          kind: "scene",
          title: `Сцена из диктовки ${new Intl.DateTimeFormat("ru", {
            day: "numeric",
            month: "short",
          }).format(new Date())}`,
        });
        target = created.node;
      } else if (target) {
        // The project outline intentionally contains no revision body. Read the
        // canonical document before composing an append so existing text cannot
        // be mistaken for an empty document.
        target = (await getNode(target.id)).node;
      }
      if (!target) {
        throw new Error("Выберите документ");
      }
      const draft = editor.getJSON() as TiptapNode;
      const content =
        mode === "replace" || !target.content ? draft : mergeTranscriptAtEnd(target.content, draft);
      return {
        target,
        previousContent: target.content,
        response: await transferTranscript(sessionId, {
          nodeId: target.id,
          baseRevision: target.currentRevision,
          content,
        }),
      };
    },
    onSuccess: ({ previousContent, response, target }) => {
      void queryClient.invalidateQueries({ queryKey: ["project", project.data?.project.id] });
      showToast({
        title: "Черновик вставлен в рукопись",
        description: "Вставка сохранена одной редакцией.",
        actionLabel: previousContent ? "Отменить" : undefined,
        onAction: previousContent
          ? () => {
              void saveRevision(target.id, {
                baseRevision: response.revision.revision,
                content: previousContent,
                source: "transcript_transfer_undo",
                operationId: response.operationId,
              }).then(async () => {
                await queryClient.invalidateQueries({
                  queryKey: ["project", target.projectId],
                });
                showToast({ title: "Вставка отменена" });
              });
            }
          : undefined,
      });
      void navigate({
        to: "/projects/$projectId/studio/$nodeId",
        params: { projectId: target.projectId, nodeId: target.id },
      });
    },
  });

  if (session.isPending) {
    return <Spinner label="Открываем расшифровку…" />;
  }
  const data = session.data?.session;
  if (!data?.draft) {
    return (
      <main className="grid min-h-full place-items-center p-5">
        <Banner
          variant="warning"
          title="Черновик ещё не готов"
          body="Вернитесь к экрану обработки записи."
          action={
            <Button asChild variant="secondary" size="compact">
              <Link to="/dictations/$sessionId/finalizing" params={{ sessionId }}>
                Проверить
              </Link>
            </Button>
          }
        />
      </main>
    );
  }
  if (project.isPending) {
    return <Spinner label="Открываем проект…" />;
  }
  if (!project.data) {
    return (
      <main className="grid min-h-full place-items-center p-5">
        <Banner
          variant="danger"
          title="Проект недоступен"
          body="Черновик сохранён. Попробуйте открыть его позже через центр восстановления."
        />
      </main>
    );
  }
  const lateBatch =
    data.draft.manuallyEdited &&
    Boolean(data.suggestionArtifact?.text ?? data.batchArtifact?.text) &&
    (data.suggestionArtifact?.text ?? data.batchArtifact?.text) !== data.draft.baseSourceText;

  return (
    <ProjectUtilityShell
      project={project.data.project}
      nodes={project.data.nodes}
      activeNodeId={data.destinationNodeId}
      saveState={saveState}
      mobileSaveStatus
    >
      <div className="flex h-full min-h-0 flex-col">
        <header className="flex min-h-[82px] items-center gap-3 px-4 sm:px-8">
          <div>
            <p className="text-[10px] font-semibold tracking-[0.14em] text-[var(--ink-tertiary)] uppercase">
              Голосовая сессия · {formatClock(data.durationMs)}
            </p>
            <h1 className="font-display mt-1 text-[28px] leading-tight">
              {project.data.nodes.find((node) => node.id === data.destinationNodeId)?.title ??
                "Проверка расшифровки"}
            </h1>
          </div>
          <div className="ml-auto flex items-center gap-2">
            {lateBatch ? (
              <Button asChild variant="secondary">
                <Link to="/dictations/$sessionId/diff" params={{ sessionId }}>
                  <GitCompareArrows className="size-4" aria-hidden="true" />
                  Сравнить
                </Link>
              </Button>
            ) : null}
            <Button
              type="button"
              className="lg:hidden"
              aria-label="Вставить в рукопись"
              onClick={() => setTransferOpen(true)}
            >
              <Send className="size-4" aria-hidden="true" />
              <span className="hidden sm:inline">Вставить в рукопись</span>
            </Button>
          </div>
        </header>
        {data.status === "finalizing" ? (
          <Banner
            className="m-3 mb-0"
            variant="info"
            title="Это живой черновик"
            body="Можно редактировать уже сейчас. Финальная расшифровка появится как предложение и не заменит ваши правки."
          />
        ) : null}
        {lateBatch ? (
          <Banner
            className="m-3 mb-0"
            variant="info"
            title="Готова уточнённая расшифровка"
            body="Ваши правки не заменены. Сравните варианты и примите только нужные изменения."
            action={
              <Button asChild variant="secondary" size="compact">
                <Link to="/dictations/$sessionId/diff" params={{ sessionId }}>
                  Сравнить
                </Link>
              </Button>
            }
          />
        ) : null}
        {retranscriptionBaseline !== null ? (
          <Banner
            className="m-3 mb-0"
            variant="info"
            title="Повторно расшифровываем со словарём"
            body="Новый результат появится отдельным предложением. Редактируемый черновик и исходные артефакты не меняются."
          />
        ) : null}
        <div className="flex min-h-0 flex-1 flex-col lg:grid lg:grid-cols-[minmax(0,1fr)_320px]">
          <section className="flex min-h-0 flex-1 flex-col border-r border-[var(--border-subtle)] bg-[var(--surface-paper)]">
            <FormattingToolbar
              editor={editor}
              projectId={data.projectId}
              onFocusMode={() => undefined}
              onFind={() => {
                void requestInput({
                  title: "Найти в расшифровке",
                  description: "Поиск выполняется по редактируемому черновику.",
                  label: "Текст для поиска",
                  icon: Search,
                  confirmLabel: "Найти",
                }).then((query) => {
                  if (
                    query &&
                    !editor?.getText().toLocaleLowerCase().includes(query.toLocaleLowerCase())
                  ) {
                    showToast({ title: "Ничего не найдено" });
                  }
                });
              }}
            />
            <fieldset
              className="min-h-0 flex-1 overflow-y-auto"
              onClickCapture={(event) => playParagraphSource(event.target)}
              onKeyDownCapture={(event) => {
                if (event.altKey && event.key === "Enter") {
                  playParagraphSource(event.target);
                }
              }}
            >
              <legend className="sr-only">Черновик с привязанными аудиодиапазонами</legend>
              <EditorContent editor={editor} className="editor-content" />
            </fieldset>
          </section>
          <aside className="max-h-[42vh] overflow-y-auto bg-[var(--surface-panel)] p-4 lg:max-h-none">
            <h2 className="text-xs font-semibold tracking-[0.14em] text-[var(--ink-tertiary)] uppercase">
              Проверка
            </h2>
            <p className="mt-4 text-sm font-medium">
              {uncertain.length > 0
                ? `${uncertain.length} мест для проверки`
                : "Неуверенных мест не найдено"}
            </p>
            <div className="mt-3 grid gap-2">
              {uncertain.map((segment) => (
                <button
                  key={segment.id}
                  type="button"
                  className="rounded-[var(--radius-sm)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-3 text-left text-sm hover:border-[var(--warning)]"
                  onClick={() => {
                    if (audioRef.current) {
                      audioRef.current.currentTime = segment.startMs / 1_000;
                      void audioRef.current.play();
                    }
                  }}
                >
                  <span className="font-mono text-[11px] text-[var(--warning)]">
                    {formatClock(segment.startMs)}
                  </span>
                  <span className="mt-1 block">{segment.text}</span>
                </button>
              ))}
            </div>
            <div className="mt-5 grid gap-2">
              <Button asChild variant="secondary">
                <Link to="/projects/$projectId/dictionary" params={{ projectId: data.projectId }}>
                  <ListPlus className="size-4" aria-hidden="true" />
                  Добавить термин в словарь
                </Link>
              </Button>
              <Button asChild variant="ghost">
                <Link to="/dictations/$sessionId/diff" params={{ sessionId }}>
                  <Search className="size-4" aria-hidden="true" />
                  Сравнить с исходником
                </Link>
              </Button>
              <Button
                type="button"
                variant="ghost"
                disabled={retranscribe.isPending || retranscriptionBaseline !== null}
                onClick={() => retranscribe.mutate()}
              >
                <RefreshCw
                  className={`size-4 ${retranscriptionBaseline !== null ? "animate-spin" : ""}`}
                  aria-hidden="true"
                />
                {retranscriptionBaseline !== null
                  ? "Расшифровываем…"
                  : "Повторить расшифровку со словарём"}
              </Button>
            </div>
            <div className="mt-6 border-t border-[var(--border-subtle)] pt-5">
              <h3 className="text-sm font-semibold">История черновика</h3>
              <ol className="mt-3 grid gap-2">
                {data.draftRevisions?.slice(0, 5).map((revision) => (
                  <li
                    key={revision.id}
                    className="rounded-[var(--radius-sm)] bg-[var(--surface-subtle)] px-3 py-2"
                  >
                    <p className="text-xs font-medium">{revision.source}</p>
                    <p className="mt-0.5 text-[11px] text-[var(--ink-tertiary)]">
                      Редакция {revision.revision}
                    </p>
                  </li>
                ))}
              </ol>
            </div>
          </aside>
        </div>
        <div className="sticky bottom-0 z-10 border-t border-[var(--border-subtle)] bg-[var(--surface-panel)] px-3 py-2 sm:px-5">
          <div className="mx-auto flex max-w-[1352px] flex-col gap-2 sm:flex-row sm:items-center">
            <div className="min-w-0 flex-1">
              {audio.data ? (
                <audio
                  ref={audioRef}
                  className="h-10 w-full"
                  controls
                  preload="metadata"
                  src={audio.data.url}
                >
                  <track kind="captions" />
                </audio>
              ) : (
                <p className="py-2 text-sm text-[var(--ink-tertiary)]">Загружаем аудио…</p>
              )}
            </div>
            <Button
              type="button"
              className="hidden shrink-0 lg:inline-flex"
              aria-label="Вставить в рукопись"
              onClick={() => setTransferOpen(true)}
            >
              <Send className="size-4" aria-hidden="true" />
              Вставить в рукопись
            </Button>
          </div>
        </div>
        <ModalDialog
          open={transferOpen}
          onOpenChange={setTransferOpen}
          title="Вставить в рукопись"
          description="Сначала проверьте место и результат операции. Исходная расшифровка останется неизменной."
          icon={Send}
          footer={
            <>
              <Button type="button" variant="secondary" onClick={() => setTransferOpen(false)}>
                Отмена
              </Button>
              <Button
                type="button"
                disabled={transfer.isPending || (!destination && mode !== "new_scene")}
                onClick={() => transfer.mutate()}
              >
                <Save className="size-4" aria-hidden="true" />
                {transfer.isPending ? "Вставляем…" : "Подтвердить вставку"}
              </Button>
            </>
          }
        >
          <div className="grid gap-4">
            <Field label="Документ">
              <Select value={destination} onChange={(event) => setDestination(event.target.value)}>
                <option value="">Выберите документ</option>
                {project.data?.nodes
                  .filter((node) => !node.trashedAt)
                  .map((node) => (
                    <option key={node.id} value={node.id}>
                      {node.title}
                    </option>
                  ))}
              </Select>
            </Field>
            <Field label="Способ вставки">
              <Select value={mode} onChange={(event) => setMode(event.target.value as typeof mode)}>
                <option value="append">В конец документа</option>
                <option value="replace">Заменить пустой документ</option>
                <option value="new_scene">Создать новую сцену</option>
              </Select>
            </Field>
          </div>
          {transfer.error ? (
            <Banner
              className="mt-4"
              variant="danger"
              title="Не удалось вставить"
              body={transfer.error.message}
            />
          ) : null}
        </ModalDialog>
      </div>
    </ProjectUtilityShell>
  );
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
