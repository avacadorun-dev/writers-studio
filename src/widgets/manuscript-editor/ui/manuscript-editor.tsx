import { type Editor, EditorContent, useEditor } from "@tiptap/react";
import {
  BookMarked,
  Headphones,
  MessageSquarePlus,
  Mic,
  Minimize2,
  Paperclip,
  Search,
  Sparkles,
  X,
} from "lucide-react";
import {
  type DragEvent as ReactDragEvent,
  type KeyboardEvent as ReactKeyboardEvent,
  type MouseEvent as ReactMouseEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { AssistantLivePreview, AssistantSelectionAttachment } from "~entities/assistant";
import { getSegmentAudioSource } from "~entities/dictation";
import {
  type AnnotationAnchor,
  type DocumentAnnotation,
  type DocumentNode,
  type TiptapNode,
  uploadProjectImage,
} from "~entities/project";
import { useEditorAutosave } from "~features/autosave";
import {
  annotationAnchorFromSelection,
  editorExtensions,
  FormattingToolbar,
  isExternalFileDrag,
  mappedAnnotationAnchors,
  migrateFencedCodeBlocks,
  migrateInlineCodeMarks,
  revealDocumentAnnotation,
  setDocumentAnnotations,
  validateEditorImageFile,
} from "~features/editor";
import {
  cn,
  createClientId,
  getEditorSnapshot,
  loadWriterPreferences,
  maximumNodeTitleLength,
  normalizeNodeTitle,
  type WriterPreferences,
  writerPreferencesChangedEvent,
} from "~shared/lib";
import { Banner, Button, type SaveState, SaveStatus, useActionDialog, useToast } from "~shared/ui";
import {
  placeSelectionContextMenu,
  type SelectionContextMenuPosition,
} from "../model/selection-context-menu";

interface ManuscriptEditorProps {
  node: DocumentNode;
  nodeTypeLabel: string;
  highlight?: string;
  onSaveState: (state: SaveState) => void;
  onDictate: () => void;
  onRenameNode?: (title: string) => void;
  assistantSelection?: AssistantSelectionAttachment;
  assistantPreview?: AssistantLivePreview;
  onAssistantSelectionChange?: (selection?: AssistantSelectionAttachment) => void;
  onAssistantPreviewInvalidated?: () => void;
  integratedWorkspace?: boolean;
  annotations?: DocumentAnnotation[];
  activeAnnotationId?: string;
  onCreateNote?: (anchor: AnnotationAnchor, baseRevision: number) => void;
  onLinkDictionary?: (anchor: AnnotationAnchor, baseRevision: number) => void;
  onAnnotationSelect?: (annotationId: string) => void;
  onRevisionSaved?: () => void;
}

const fallbackDocument: TiptapNode = {
  type: "doc",
  attrs: { schemaVersion: 1 },
  content: [
    {
      type: "paragraph",
      attrs: { blockId: createClientId(), sourceRefs: [] },
    },
  ],
};

export function ManuscriptEditor({
  node,
  nodeTypeLabel,
  highlight,
  onSaveState,
  onDictate,
  onRenameNode,
  assistantSelection,
  assistantPreview,
  onAssistantSelectionChange,
  onAssistantPreviewInvalidated,
  integratedWorkspace = false,
  annotations,
  activeAnnotationId,
  onCreateNote,
  onLinkDictionary,
  onAnnotationSelect,
  onRevisionSaved,
}: ManuscriptEditorProps) {
  const [currentRevision, setCurrentRevision] = useState(node.currentRevision);
  const [focusMode, setFocusMode] = useState(false);
  const [textStats, setTextStats] = useState({ words: 0, characters: 0 });
  const [preferences, setPreferences] = useState<WriterPreferences>(loadWriterPreferences);
  const [recovered, setRecovered] = useState(false);
  const [contextPanel, setContextPanel] = useState<"overview" | "stats" | null>(null);
  const [selectionMenu, setSelectionMenu] = useState<SelectionContextMenuPosition | null>(null);
  const [imageDrop, setImageDrop] = useState<{ position: number; top: number } | null>(null);
  const [imageUploading, setImageUploading] = useState(false);
  const [editingNodeTitle, setEditingNodeTitle] = useState(false);
  const [nodeTitleDraft, setNodeTitleDraft] = useState(node.title);
  const selectionMenuRef = useRef<HTMLDivElement | null>(null);
  const nodeTitleInputRef = useRef<HTMLInputElement | null>(null);
  const editorScrollRef = useRef<HTMLDivElement | null>(null);
  const sourceAudioRef = useRef<HTMLAudioElement | null>(null);
  const loadedDocument = useRef<{ nodeId: string; revision: number } | null>(null);
  const currentRevisionRef = useRef(node.currentRevision);
  const attachedSelectionRef = useRef<{ from: number; to: number } | undefined>(undefined);
  const annotationsRef = useRef(annotations);
  const annotationSelectRef = useRef(onAnnotationSelect);
  const { showToast } = useToast();
  const { requestInput } = useActionDialog();
  useEffect(() => {
    if (!editingNodeTitle) setNodeTitleDraft(node.title);
  }, [editingNodeTitle, node.title]);
  useEffect(() => {
    if (!editingNodeTitle) return;
    nodeTitleInputRef.current?.focus();
    nodeTitleInputRef.current?.select();
  }, [editingNodeTitle]);
  const commitNodeTitle = () => {
    const title = normalizeNodeTitle(nodeTitleDraft, node.title);
    setEditingNodeTitle(false);
    if (title) onRenameNode?.(title);
  };
  const updateTextStats = useCallback((text: string) => {
    const words = text.trim() ? text.trim().split(/\s+/u).length : 0;
    setTextStats({ words, characters: Array.from(text).length });
  }, []);
  const initialContent = useMemo(
    () => migrateInlineCodeMarks(migrateFencedCodeBlocks(node.content ?? fallbackDocument)),
    [node.content],
  );
  const editor = useEditor({
    extensions: editorExtensions,
    content: initialContent,
    immediatelyRender: false,
    editorProps: {
      attributes: {
        "aria-label": `Текст документа «${node.title}»`,
        spellcheck: String(preferences.spellcheck),
      },
      handleClick: (_view, _position, event) => {
        const target = event.target instanceof Element ? event.target : null;
        const annotationId =
          target?.closest<HTMLElement>("[data-annotation-id]")?.dataset.annotationId;
        if (!annotationId) return false;
        annotationSelectRef.current?.(annotationId);
        return true;
      },
    },
  });
  const handleRevision = useCallback(
    (revision: number) => {
      loadedDocument.current = { nodeId: node.id, revision };
      currentRevisionRef.current = revision;
      setCurrentRevision(revision);
      onRevisionSaved?.();
    },
    [node.id, onRevisionSaved],
  );
  const autosave = useEditorAutosave({
    editor,
    nodeId: node.id,
    projectId: node.projectId,
    revision: currentRevision,
    onRevision: handleRevision,
    getAnnotationAnchors:
      annotations === undefined || !editor ? undefined : () => mappedAnnotationAnchors(editor),
  });

  useEffect(() => {
    annotationsRef.current = annotations;
    annotationSelectRef.current = onAnnotationSelect;
    if (editor && annotations) setDocumentAnnotations(editor, annotations);
  }, [annotations, editor, onAnnotationSelect]);

  useEffect(() => {
    if (!editor || !activeAnnotationId || !annotations) return;
    setDocumentAnnotations(editor, annotations);
    window.requestAnimationFrame(() => revealDocumentAnnotation(editor, activeAnnotationId));
  }, [activeAnnotationId, annotations, editor]);

  useEffect(() => {
    onSaveState(autosave.saveState);
  }, [autosave.saveState, onSaveState]);

  useEffect(() => {
    currentRevisionRef.current = node.currentRevision;
    setCurrentRevision(node.currentRevision);
    setRecovered(false);
    if (!editor) {
      return;
    }
    if (
      loadedDocument.current?.nodeId === node.id &&
      loadedDocument.current.revision === node.currentRevision
    ) {
      if (highlight) {
        selectFirstMatch(editor, highlight);
      }
      return;
    }
    loadedDocument.current = {
      nodeId: node.id,
      revision: node.currentRevision,
    };
    let active = true;
    void getEditorSnapshot(node.id).then((snapshot) => {
      if (!active) {
        return;
      }
      const canRecover =
        snapshot &&
        !snapshot.synced &&
        snapshot.baseRevision >= node.currentRevision &&
        snapshot.savedAt > new Date(node.updatedAt).getTime();
      editor.commands.setContent(
        canRecover
          ? migrateInlineCodeMarks(migrateFencedCodeBlocks(snapshot.content))
          : initialContent,
        {
          emitUpdate: false,
        },
      );
      if (annotationsRef.current) {
        setDocumentAnnotations(editor, annotationsRef.current);
      }
      setRecovered(Boolean(canRecover));
      if (canRecover) {
        showToast({
          title: "Локальная версия восстановлена",
          description: "Последние несинхронизированные правки снова в редакторе.",
        });
      }
      updateTextStats(editor.getText());
      if (highlight) {
        selectFirstMatch(editor, highlight);
      }
    });
    return () => {
      active = false;
    };
  }, [
    editor,
    highlight,
    initialContent,
    node.currentRevision,
    node.id,
    node.updatedAt,
    showToast,
    updateTextStats,
  ]);

  useEffect(() => {
    if (!editor) {
      return;
    }
    const handleUpdate = () => updateTextStats(editor.getText());
    editor.on("update", handleUpdate);
    updateTextStats(editor.getText());
    return () => {
      editor.off("update", handleUpdate);
    };
  }, [editor, updateTextStats]);

  useEffect(() => {
    if (!editor || !assistantPreview || assistantPreview.stale) return;
    const invalidatePreview = () => onAssistantPreviewInvalidated?.();
    editor.on("update", invalidatePreview);
    return () => {
      editor.off("update", invalidatePreview);
    };
  }, [assistantPreview, editor, onAssistantPreviewInvalidated]);

  const attachSelection = useCallback(async () => {
    if (!editor || editor.state.selection.empty || !onAssistantSelectionChange) return;
    const { from, to } = editor.state.selection;
    await autosave.flush();
    const ranges = stableSelectionRanges(editor, from, to);
    const quote = ranges
      .map((range) => range.expectedText)
      .join("\n\n")
      .trim();
    if (!quote || ranges.length === 0) {
      showToast({
        title: "Не удалось прикрепить выделение",
        description: "Выделите текст внутри одного или нескольких абзацев.",
      });
      return;
    }
    attachedSelectionRef.current = { from, to };
    onAssistantSelectionChange({
      nodeId: node.id,
      baseRevision: currentRevisionRef.current,
      ranges,
      quote,
    });
    showToast({ title: "Фрагмент прикреплён к следующему поручению" });
  }, [autosave, editor, node.id, onAssistantSelectionChange, showToast]);

  const startAnnotation = useCallback(
    async (kind: "note" | "dictionary_link") => {
      if (!editor) return;
      const anchor = annotationAnchorFromSelection(editor);
      if (!anchor) {
        showToast({
          title: "Выделите фрагмент в одном абзаце",
          description: "Заметка или ссылка не может пересекать границу между абзацами.",
        });
        return;
      }
      await autosave.flush();
      if (kind === "note") onCreateNote?.(anchor, currentRevisionRef.current);
      else onLinkDictionary?.(anchor, currentRevisionRef.current);
    },
    [autosave, editor, onCreateNote, onLinkDictionary, showToast],
  );

  const openSelectionMenu = useCallback(
    (x: number, y: number) => {
      if (!editor || editor.state.selection.empty) return false;
      setSelectionMenu(placeSelectionContextMenu(x, y, window.innerWidth, window.innerHeight));
      return true;
    },
    [editor],
  );

  const handleSelectionContextMenu = useCallback(
    (event: ReactMouseEvent<HTMLDivElement>) => {
      if (!openSelectionMenu(event.clientX, event.clientY)) return;
      event.preventDefault();
      event.stopPropagation();
    },
    [openSelectionMenu],
  );

  const preserveSelectionOnRightClick = useCallback(
    (event: ReactMouseEvent<HTMLDivElement>) => {
      if (event.button === 2 && editor && !editor.state.selection.empty) {
        event.preventDefault();
      }
    },
    [editor],
  );

  const handleSelectionMenuKey = useCallback(
    (event: ReactKeyboardEvent<HTMLDivElement>) => {
      if (!editor || editor.state.selection.empty) return;
      if (!(event.key === "ContextMenu" || (event.shiftKey && event.key === "F10"))) return;
      const coordinates = editor.view.coordsAtPos(editor.state.selection.from);
      if (!openSelectionMenu(coordinates.left, coordinates.bottom + 4)) return;
      event.preventDefault();
    },
    [editor, openSelectionMenu],
  );

  useEffect(() => {
    if (!selectionMenu) return;
    const focusMenu = window.requestAnimationFrame(() => {
      selectionMenuRef.current?.querySelector<HTMLButtonElement>("[role='menuitem']")?.focus();
    });
    const closeOnPointerDown = (event: PointerEvent) => {
      if (!selectionMenuRef.current?.contains(event.target as Node)) setSelectionMenu(null);
    };
    const closeOnKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setSelectionMenu(null);
        editor?.commands.focus();
      }
    };
    const close = () => setSelectionMenu(null);
    document.addEventListener("pointerdown", closeOnPointerDown);
    document.addEventListener("keydown", closeOnKeyDown);
    window.addEventListener("resize", close);
    window.addEventListener("scroll", close, true);
    return () => {
      window.cancelAnimationFrame(focusMenu);
      document.removeEventListener("pointerdown", closeOnPointerDown);
      document.removeEventListener("keydown", closeOnKeyDown);
      window.removeEventListener("resize", close);
      window.removeEventListener("scroll", close, true);
    };
  }, [editor, selectionMenu]);

  const runSelectionAction = useCallback((action: () => void | Promise<void>) => {
    setSelectionMenu(null);
    void action();
  }, []);

  const revealAttachedSelection = useCallback(() => {
    if (!editor || !attachedSelectionRef.current) return;
    editor.commands.setTextSelection(attachedSelectionRef.current);
    editor.commands.focus();
  }, [editor]);

  const imageDropPosition = useCallback(
    (event: ReactDragEvent<HTMLElement>) => {
      if (!editor || !editorScrollRef.current) return null;
      const result = editor.view.posAtCoords({ left: event.clientX, top: event.clientY });
      if (!result) return null;
      try {
        const caret = editor.view.coordsAtPos(result.pos);
        const viewport = editorScrollRef.current;
        const viewportBounds = viewport.getBoundingClientRect();
        return {
          position: result.pos,
          top: caret.top - viewportBounds.top + viewport.scrollTop,
        };
      } catch {
        return null;
      }
    },
    [editor],
  );

  const handleImageDragOver = useCallback(
    (event: ReactDragEvent<HTMLElement>) => {
      if (!isExternalFileDrag(event.dataTransfer)) return;
      event.preventDefault();
      event.dataTransfer.dropEffect = "copy";
      const position = imageDropPosition(event);
      if (position) setImageDrop(position);
    },
    [imageDropPosition],
  );

  const handleImageDrop = useCallback(
    (event: ReactDragEvent<HTMLElement>) => {
      if (!editor || !isExternalFileDrag(event.dataTransfer)) return;
      event.preventDefault();
      const position = imageDropPosition(event)?.position ?? imageDrop?.position;
      setImageDrop(null);
      const file = event.dataTransfer.files[0];
      if (!file || position === undefined) return;
      const validation = validateEditorImageFile(file);
      if (!validation.valid) {
        showToast({ title: "Изображение не добавлено", description: validation.message });
        return;
      }
      setImageUploading(true);
      void uploadProjectImage(node.projectId, file)
        .then(({ media }) => {
          if (editor.isDestroyed) return;
          editor
            .chain()
            .focus()
            .insertContentAt(position, {
              type: "imageBlock",
              attrs: {
                src: media.contentUrl,
                assetId: media.id,
                alt: media.originalName.replace(/\.[^.]+$/u, ""),
                caption: "",
                width: media.width ?? 1,
                height: media.height ?? 1,
              },
            })
            .run();
          showToast({ title: "Изображение добавлено" });
        })
        .catch((error: unknown) => {
          showToast({
            title: "Не удалось загрузить изображение",
            description: error instanceof Error ? error.message : "Повторите попытку",
          });
        })
        .finally(() => setImageUploading(false));
    },
    [editor, imageDrop?.position, imageDropPosition, node.projectId, showToast],
  );

  useEffect(() => {
    const refreshPreferences = () => setPreferences(loadWriterPreferences());
    window.addEventListener(writerPreferencesChangedEvent, refreshPreferences);
    window.addEventListener("storage", refreshPreferences);
    return () => {
      window.removeEventListener(writerPreferencesChangedEvent, refreshPreferences);
      window.removeEventListener("storage", refreshPreferences);
    };
  }, []);

  useEffect(() => {
    if (!editor) {
      return;
    }
    editor.setOptions({
      editorProps: {
        ...editor.options.editorProps,
        attributes: {
          ...editor.options.editorProps.attributes,
          "aria-label": `Текст документа «${node.title}»`,
          spellcheck: String(preferences.spellcheck),
        },
      },
    });
  }, [editor, node.title, preferences.spellcheck]);

  const findInDocument = useCallback(() => {
    if (!editor) {
      return;
    }
    void requestInput({
      title: "Найти в документе",
      description: "Первое совпадение будет выделено в открытом документе.",
      label: "Текст для поиска",
      icon: Search,
      confirmLabel: "Найти",
    }).then((query) => {
      if (!query) {
        return;
      }
      if (!selectFirstMatch(editor, query)) {
        showToast({
          title: "Ничего не найдено",
          description: `В документе нет «${query}».`,
        });
      }
    });
  }, [editor, requestInput, showToast]);

  useEffect(() => {
    const handleFind = () => findInDocument();
    const handleFocus = () => setFocusMode(true);
    const handleKeyDown = (event: KeyboardEvent) => {
      const modifier = event.metaKey || event.ctrlKey;
      if (modifier && !event.shiftKey && event.key.toLocaleLowerCase() === "f") {
        event.preventDefault();
        findInDocument();
      } else if (modifier && event.shiftKey && event.key === "Enter") {
        event.preventDefault();
        setFocusMode((current) => !current);
      }
    };
    window.addEventListener("writers-studio-find", handleFind);
    window.addEventListener("writers-studio-focus", handleFocus);
    window.addEventListener("keydown", handleKeyDown);
    return () => {
      window.removeEventListener("writers-studio-find", handleFind);
      window.removeEventListener("writers-studio-focus", handleFocus);
      window.removeEventListener("keydown", handleKeyDown);
    };
  }, [findInDocument]);

  const paragraphAttributes = editor?.getAttributes("paragraph");
  const sourceRefs = Array.isArray(paragraphAttributes?.sourceRefs)
    ? (paragraphAttributes.sourceRefs as string[])
    : [];

  useEffect(
    () => () => {
      sourceAudioRef.current?.pause();
      sourceAudioRef.current = null;
    },
    [],
  );

  const playSource = useCallback(async () => {
    const segmentId = sourceRefs[0];
    if (!segmentId) {
      showToast({
        title: "Для выбранного абзаца нет аудиоисточника",
        description: "Источник появляется у текста, перенесённого из диктовки.",
      });
      return;
    }
    try {
      const source = await getSegmentAudioSource(segmentId);
      sourceAudioRef.current?.pause();
      const audio = new Audio(source.url);
      sourceAudioRef.current = audio;
      const startPlayback = () => {
        audio.currentTime = source.startMs / 1_000;
        void audio.play();
      };
      const stopAtRangeEnd = () => {
        if (audio.currentTime * 1_000 >= source.endMs) {
          audio.pause();
          audio.removeEventListener("timeupdate", stopAtRangeEnd);
        }
      };
      audio.addEventListener("timeupdate", stopAtRangeEnd);
      if (audio.readyState >= HTMLMediaElement.HAVE_METADATA) {
        startPlayback();
      } else {
        audio.addEventListener("loadedmetadata", startPlayback, { once: true });
        audio.load();
      }
    } catch (error) {
      showToast({
        title: "Источник недоступен",
        description: error instanceof Error ? error.message : "Не удалось открыть аудио",
      });
    }
  }, [showToast, sourceRefs]);

  useEffect(() => {
    const showOverview = () =>
      setContextPanel((current) => (current === "overview" ? null : "overview"));
    const showStats = () => setContextPanel((current) => (current === "stats" ? null : "stats"));
    const playCurrentSource = () => void playSource();
    window.addEventListener("writers-studio-overview", showOverview);
    window.addEventListener("writers-studio-stats", showStats);
    window.addEventListener("writers-studio-play-source", playCurrentSource);
    return () => {
      window.removeEventListener("writers-studio-overview", showOverview);
      window.removeEventListener("writers-studio-stats", showStats);
      window.removeEventListener("writers-studio-play-source", playCurrentSource);
    };
  }, [playSource]);

  return (
    <section
      className={
        focusMode
          ? "fixed inset-0 z-50 flex min-h-0 min-w-0 flex-col bg-[var(--surface-app)]"
          : "relative flex min-h-0 min-w-0 flex-1 flex-col"
      }
      aria-label="Редактор документа"
    >
      {focusMode ? (
        <div className="flex h-14 items-center justify-between border-b border-[var(--border-subtle)] bg-[var(--surface-panel)] px-4">
          <SaveStatus state={autosave.saveState} />
          <Button type="button" variant="ghost" onClick={() => setFocusMode(false)}>
            <Minimize2 className="size-4" aria-hidden="true" />
            Выйти из фокуса
          </Button>
        </div>
      ) : null}
      <div
        data-workspace-layout={integratedWorkspace ? "integrated" : "floating"}
        className={cn(
          "manuscript-surface flex min-h-0 min-w-0 w-full flex-1 flex-col overflow-hidden bg-[var(--surface-paper)]",
          !integratedWorkspace &&
            "mx-auto my-5 max-w-[800px] rounded-[var(--radius-lg)] border border-[var(--border-subtle)] lg:my-4 lg:max-w-[760px] xl:my-7 xl:max-w-[800px]",
        )}
      >
        <FormattingToolbar
          editor={editor}
          projectId={node.projectId}
          onFocusMode={() => setFocusMode(true)}
          onFind={findInDocument}
          integratedWorkspace={integratedWorkspace}
        />
        {autosave.saveState === "conflict" ? (
          <Banner
            className="m-4 mb-0"
            variant="danger"
            title="Мы сохранили оба варианта"
            body="Откройте конфликтующую копию в дереве проекта и выберите нужный текст."
          />
        ) : recovered ? (
          <Banner
            className="m-4 mb-0"
            variant="info"
            title="Восстановлены правки с этого устройства"
            body="Они сохранятся на сервере после восстановления соединения."
          />
        ) : null}
        <div ref={editorScrollRef} className="relative min-h-0 flex-1 overflow-y-auto">
          <div
            className={cn(
              "manuscript-heading px-5 pt-8 sm:pt-10",
              integratedWorkspace ? "editor-shell-width mx-auto sm:px-14" : "w-full sm:px-[72px]",
            )}
          >
            <p className="text-[11px] font-semibold tracking-[0.14em] text-[var(--ink-tertiary)] uppercase">
              {nodeTypeLabel}
            </p>
            {editingNodeTitle ? (
              <input
                ref={nodeTitleInputRef}
                value={nodeTitleDraft}
                maxLength={maximumNodeTitleLength}
                aria-label={`Новое название для «${node.title}»`}
                className="font-display mt-2 w-full rounded border border-[var(--focus)] bg-[var(--surface-paper)] px-2 text-[34px] leading-[1.25] outline-none"
                onFocus={(event) => event.currentTarget.select()}
                onChange={(event) => setNodeTitleDraft(event.target.value)}
                onBlur={commitNodeTitle}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    event.preventDefault();
                    commitNodeTitle();
                  } else if (event.key === "Escape") {
                    event.preventDefault();
                    setNodeTitleDraft(node.title);
                    setEditingNodeTitle(false);
                  }
                }}
              />
            ) : (
              <h1
                className="font-display mt-2 w-fit max-w-full cursor-text rounded px-1 text-[34px] leading-[1.25] hover:bg-[var(--surface-subtle)]"
                title="Дважды нажмите, чтобы переименовать"
                onDoubleClick={() => onRenameNode && setEditingNodeTitle(true)}
              >
                <span className="sr-only">Название материала: </span>
                {node.title}
              </h1>
            )}
            <div className="mt-6 h-px w-16 bg-[var(--border-strong)]" aria-hidden="true" />
            {assistantSelection ? (
              <div className="mt-5 flex items-start gap-3 rounded-[var(--radius-md)] border border-[var(--accent)]/30 bg-[var(--accent-soft)] px-3 py-2 text-xs">
                <Paperclip
                  className="mt-0.5 size-3.5 shrink-0 text-[var(--accent)]"
                  aria-hidden="true"
                />
                <button
                  type="button"
                  className="min-w-0 flex-1 text-left"
                  onClick={revealAttachedSelection}
                >
                  <span className="block font-semibold text-[var(--accent-ink)]">
                    Выделение прикреплено
                  </span>
                  <span className="mt-0.5 block truncate text-[var(--ink-secondary)]">
                    {assistantSelection.quote}
                  </span>
                </button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="size-7 min-h-7"
                  aria-label="Убрать прикреплённое выделение"
                  onClick={() => onAssistantSelectionChange?.(undefined)}
                >
                  <X className="size-3.5" aria-hidden="true" />
                </Button>
              </div>
            ) : null}
            <div className="mt-5 flex min-h-10 items-center" aria-live="polite">
              {editor?.isEmpty ? (
                <div className="flex flex-wrap gap-2">
                  <Button
                    type="button"
                    variant="secondary"
                    onClick={() => editor.chain().focus("start").run()}
                  >
                    Начать писать
                  </Button>
                  <Button type="button" variant="secondary" onClick={onDictate}>
                    <Mic className="size-4" aria-hidden="true" />
                    Диктовать
                  </Button>
                </div>
              ) : null}
            </div>
          </div>
          {assistantPreview ? (
            <AssistantProposalPreview
              preview={assistantPreview}
              integratedWorkspace={integratedWorkspace}
            />
          ) : null}
          {imageDrop ? (
            <div
              data-testid="editor-image-drop-indicator"
              className="pointer-events-none absolute inset-x-5 z-20 mx-auto max-w-[var(--editor-max-width)]"
              style={{ top: imageDrop.top }}
              aria-hidden="true"
            >
              <div className="h-0.5 bg-[var(--accent)] shadow-[0_0_0_3px_var(--accent-soft)]" />
              <span className="mt-1 inline-flex rounded-full bg-[var(--accent-surface)] px-2 py-1 text-[10px] font-semibold text-[var(--ink-on-inverse)]">
                {imageUploading ? "Загружаем изображение…" : "Вставить изображение здесь"}
              </span>
            </div>
          ) : null}
          <EditorContent
            editor={editor}
            className="editor-content"
            onMouseDown={preserveSelectionOnRightClick}
            onContextMenu={handleSelectionContextMenu}
            onKeyDown={handleSelectionMenuKey}
            onDragOver={handleImageDragOver}
            onDragLeave={(event) => {
              const related = event.relatedTarget;
              if (related instanceof Node && event.currentTarget.contains(related)) return;
              setImageDrop(null);
            }}
            onDrop={handleImageDrop}
          />
        </div>
        <footer className="flex h-10 shrink-0 items-center border-t border-[var(--border-subtle)] bg-[var(--surface-panel)] text-[11px] text-[var(--ink-tertiary)]">
          <div
            className={cn(
              "flex w-full items-center gap-3 px-5",
              integratedWorkspace ? "editor-shell-width mx-auto sm:px-14" : "sm:px-[72px]",
            )}
          >
            <span aria-live="polite">
              {new Intl.NumberFormat("ru").format(textStats.words)} слов
            </span>
            {sourceRefs.length > 0 ? (
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="size-8 min-h-8"
                aria-label="Прослушать источник этого абзаца"
                title="Прослушать источник"
                onClick={() => void playSource()}
              >
                <Headphones className="size-4" aria-hidden="true" />
              </Button>
            ) : null}
          </div>
        </footer>
      </div>
      {selectionMenu ? (
        <div
          ref={selectionMenuRef}
          role="menu"
          aria-label="Действия с выделением"
          className="fixed z-[90] grid w-[268px] rounded-[var(--radius-md)] border border-[var(--border-strong)] bg-[var(--surface-raised)] p-1 shadow-[0_20px_40px_var(--shadow-color)]"
          style={{ left: selectionMenu.x, top: selectionMenu.y }}
        >
          {onLinkDictionary ? (
            <button
              type="button"
              role="menuitem"
              className="flex min-h-10 items-center gap-3 rounded-[var(--radius-sm)] px-3 text-left text-sm hover:bg-[var(--surface-subtle)] focus-visible:bg-[var(--surface-subtle)] focus-visible:outline-none"
              onClick={() => runSelectionAction(() => startAnnotation("dictionary_link"))}
            >
              <BookMarked className="size-4 text-[var(--accent)]" aria-hidden="true" />
              Связать со словарём
            </button>
          ) : null}
          {onCreateNote ? (
            <button
              type="button"
              role="menuitem"
              className="flex min-h-10 items-center gap-3 rounded-[var(--radius-sm)] px-3 text-left text-sm hover:bg-[var(--surface-subtle)] focus-visible:bg-[var(--surface-subtle)] focus-visible:outline-none"
              onClick={() => runSelectionAction(() => startAnnotation("note"))}
            >
              <MessageSquarePlus className="size-4 text-[var(--warning)]" aria-hidden="true" />
              Добавить заметку
            </button>
          ) : null}
          {onAssistantSelectionChange ? (
            <button
              type="button"
              role="menuitem"
              className="flex min-h-10 items-center gap-3 rounded-[var(--radius-sm)] px-3 text-left text-sm hover:bg-[var(--surface-subtle)] focus-visible:bg-[var(--surface-subtle)] focus-visible:outline-none"
              onClick={() => runSelectionAction(attachSelection)}
            >
              <Paperclip className="size-4 text-[var(--ink-secondary)]" aria-hidden="true" />
              Прикрепить к ассистенту
            </button>
          ) : null}
        </div>
      ) : null}
      {contextPanel ? (
        <aside className="absolute top-4 right-4 z-20 w-[280px] rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-raised)] p-5 shadow-[0_10px_24px_var(--shadow-color)]">
          <div className="flex items-start justify-between gap-3">
            <div>
              <p className="text-[10px] font-semibold tracking-[0.14em] text-[var(--ink-tertiary)] uppercase">
                {contextPanel === "overview" ? "Обзор документа" : "Статистика"}
              </p>
              <h2 className="font-display mt-2 text-2xl">{node.title}</h2>
            </div>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-9 min-h-9"
              aria-label="Закрыть контекстную панель"
              onClick={() => setContextPanel(null)}
            >
              <Minimize2 className="size-4" aria-hidden="true" />
            </Button>
          </div>
          {contextPanel === "overview" ? (
            <dl className="mt-5 grid gap-3 text-sm">
              <div className="flex justify-between gap-3">
                <dt className="text-[var(--ink-secondary)]">Статус</dt>
                <dd>{node.status}</dd>
              </div>
              <div className="flex justify-between gap-3">
                <dt className="text-[var(--ink-secondary)]">Редакция</dt>
                <dd>{currentRevision}</dd>
              </div>
            </dl>
          ) : (
            <dl className="mt-5 grid gap-3 text-sm">
              <div className="flex justify-between gap-3">
                <dt className="text-[var(--ink-secondary)]">Слов</dt>
                <dd>{new Intl.NumberFormat("ru").format(textStats.words)}</dd>
              </div>
              <div className="flex justify-between gap-3">
                <dt className="text-[var(--ink-secondary)]">Знаков</dt>
                <dd>{new Intl.NumberFormat("ru").format(textStats.characters)}</dd>
              </div>
              <div className="flex justify-between gap-3">
                <dt className="text-[var(--ink-secondary)]">Чтение</dt>
                <dd>{formatReadingTime(textStats.words)}</dd>
              </div>
              {preferences.wordGoal > 0 ? (
                <div className="flex justify-between gap-3">
                  <dt className="text-[var(--ink-secondary)]">Цель</dt>
                  <dd>
                    {new Intl.NumberFormat("ru").format(textStats.words)} /{" "}
                    {new Intl.NumberFormat("ru").format(preferences.wordGoal)}
                  </dd>
                </div>
              ) : null}
            </dl>
          )}
        </aside>
      ) : null}
    </section>
  );
}

function stableSelectionRanges(editor: Editor, from: number, to: number) {
  const ranges: AssistantSelectionAttachment["ranges"] = [];
  editor.state.doc.forEach((block, offset, index) => {
    if (!["paragraph", "heading", "blockquote", "codeBlock"].includes(block.type.name)) return;
    const contentStart = offset + 1;
    const contentEnd = contentStart + block.content.size;
    const overlapStart = Math.max(from, contentStart);
    const overlapEnd = Math.min(to, contentEnd);
    if (overlapEnd <= overlapStart) return;
    const localStart = overlapStart - contentStart;
    const localEnd = overlapEnd - contentStart;
    const expectedText = block.textBetween(localStart, localEnd, "\n");
    if (!expectedText) return;
    const prefix = block.textBetween(0, localStart, "\n");
    const start = Array.from(prefix).length;
    const length = Array.from(expectedText).length;
    const explicitBlockId = block.attrs?.blockId;
    ranges.push({
      blockId:
        typeof explicitBlockId === "string" && explicitBlockId.trim()
          ? explicitBlockId
          : `__writers_studio_legacy_block_${index}`,
      start,
      end: start + length,
      expectedText,
    });
  });
  return ranges;
}

function AssistantProposalPreview({
  preview,
  integratedWorkspace,
}: {
  preview: AssistantLivePreview;
  integratedWorkspace: boolean;
}) {
  const edits = Array.isArray(preview.arguments.edits)
    ? preview.arguments.edits.filter(isPreviewEdit)
    : [];
  const fallback = manuscriptSnapshotText(preview.proposedSnapshot);
  return (
    <aside
      data-testid="assistant-live-preview"
      aria-label="Неприменённое предложение ассистента"
      className={cn(
        "mt-6 rounded-[var(--radius-md)] border px-4 py-4",
        integratedWorkspace ? "editor-shell-width mx-auto" : "mx-5 sm:mx-[72px]",
        preview.stale
          ? "border-[var(--danger)]/35 bg-[var(--danger-soft)]"
          : "border-[var(--success)]/35 bg-[var(--diff-insert)]",
      )}
    >
      <div className="flex items-center gap-2 text-[10px] font-semibold tracking-[0.12em] uppercase">
        <Sparkles className="size-3.5" aria-hidden="true" />
        {preview.stale
          ? "Предложение устарело · применить нельзя"
          : "Предложение · ещё не применено"}
      </div>
      {edits.length > 0 ? (
        <div className="mt-3 space-y-3">
          {edits.map((edit) => (
            <div
              key={`${edit.blockId}:${edit.start}:${edit.end}`}
              className="grid gap-2 sm:grid-cols-2"
            >
              <div className="rounded-[var(--radius-sm)] bg-[var(--diff-delete)] px-3 py-2">
                <span className="text-[9px] font-semibold tracking-wider text-[var(--danger)] uppercase">
                  Было
                </span>
                <p className="font-manuscript mt-1 whitespace-pre-wrap text-sm leading-6">
                  {edit.expectedText || "Пустой диапазон"}
                </p>
              </div>
              <div className="rounded-[var(--radius-sm)] bg-[var(--surface-paper)]/70 px-3 py-2">
                <span className="text-[9px] font-semibold tracking-wider text-[var(--success)] uppercase">
                  Предложено
                </span>
                <p className="font-manuscript mt-1 whitespace-pre-wrap text-sm leading-6">
                  {edit.replacement || "Удалить этот фрагмент"}
                </p>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <p className="font-manuscript mt-3 line-clamp-6 whitespace-pre-wrap text-sm leading-6">
          {fallback || "Ассистент подготовил структурное изменение этого документа."}
        </p>
      )}
      <p className="mt-3 text-[11px] text-[var(--ink-secondary)]">
        Это отдельный preview: рукопись, autosave и история отмены пока не изменены.
      </p>
    </aside>
  );
}

function isPreviewEdit(value: unknown): value is {
  blockId: string;
  start: number;
  end: number;
  expectedText: string;
  replacement: string;
} {
  if (!value || typeof value !== "object") return false;
  const edit = value as Record<string, unknown>;
  return (
    typeof edit.blockId === "string" &&
    typeof edit.start === "number" &&
    typeof edit.end === "number" &&
    typeof edit.expectedText === "string" &&
    typeof edit.replacement === "string"
  );
}

function manuscriptSnapshotText(value: unknown): string {
  if (!value || typeof value !== "object") return "";
  const record = value as Record<string, unknown>;
  if (typeof record.text === "string") return record.text;
  if (typeof record.plainText === "string") return record.plainText;
  if (!Array.isArray(record.content)) return "";
  const separator = record.type === "doc" ? "\n\n" : "";
  return record.content.map(manuscriptSnapshotText).filter(Boolean).join(separator);
}

function formatReadingTime(words: number): string {
  if (words === 0) {
    return "0 мин чтения";
  }
  return `≈ ${Math.max(1, Math.ceil(words / 180))} мин чтения`;
}

function selectFirstMatch(editor: Editor, query: string): boolean {
  let match: { from: number; to: number } | undefined;
  editor.state.doc.descendants((proseNode, position) => {
    if (match || !proseNode.isText || !proseNode.text) {
      return;
    }
    const index = proseNode.text.toLocaleLowerCase().indexOf(query.toLocaleLowerCase());
    if (index >= 0) {
      match = { from: position + index, to: position + index + query.length };
    }
  });
  if (!match) {
    return false;
  }
  editor.commands.setTextSelection(match);
  editor.commands.focus();
  return true;
}
