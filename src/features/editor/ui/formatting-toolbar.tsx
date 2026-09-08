import type { Editor } from "@tiptap/react";
import {
  Bold,
  Check,
  CircleAlert,
  Code2,
  Columns3,
  Focus,
  ImagePlus,
  Italic,
  Lightbulb,
  Link as LinkIcon,
  List,
  ListOrdered,
  ListTodo,
  Minus,
  MoreHorizontal,
  Plus,
  Quote,
  Redo2,
  Rows3,
  Search,
  Sigma,
  Strikethrough,
  Table2,
  TextCursorInput,
  Trash2,
  TriangleAlert,
  Underline as UnderlineIcon,
  Undo2,
  Workflow,
} from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { uploadProjectImage } from "~entities/project";
import {
  cn,
  type EditorLineHeight,
  type EditorListItemSpacing,
  type EditorParagraphSpacing,
  type EditorToolbarMode,
  loadWriterPreferences,
  saveWriterPreferences,
  type WriterPreferences,
  writerPreferencesChangedEvent,
} from "~shared/lib";
import {
  Button,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  Select,
  useActionDialog,
  useToast,
} from "~shared/ui";
import { codeLanguageOptions } from "../model/code-languages";
import { validateEditorImageFile } from "../model/image-drop";

interface FormattingToolbarProps {
  editor: Editor | null;
  onFocusMode: () => void;
  onFind: () => void;
  projectId: string;
  integratedWorkspace?: boolean;
}

export type ToolbarPresentation = "expanded" | "compact";

export function resolveToolbarPresentation(
  mode: EditorToolbarMode,
  availableWidth: number,
  viewportWidth = availableWidth,
): ToolbarPresentation {
  if (mode === "expanded") return "expanded";
  if (mode === "compact") return "compact";
  return viewportWidth >= 1280 && availableWidth >= 790 ? "expanded" : "compact";
}

export function FormattingToolbar({
  editor,
  onFocusMode,
  onFind,
  projectId,
  integratedWorkspace = false,
}: FormattingToolbarProps) {
  const { requestInput, requestSelect } = useActionDialog();
  const { showToast } = useToast();
  const imageInputRef = useRef<HTMLInputElement | null>(null);
  const toolbarRef = useRef<HTMLDivElement | null>(null);
  const [imageUploading, setImageUploading] = useState(false);
  const [availableWidth, setAvailableWidth] = useState(0);
  const [viewportWidth, setViewportWidth] = useState(() => window.innerWidth);
  const [preferences, setPreferences] = useState<WriterPreferences>(loadWriterPreferences);
  const presentation = resolveToolbarPresentation(
    preferences.toolbarMode,
    availableWidth,
    viewportWidth,
  );

  useEffect(() => {
    const toolbar = toolbarRef.current;
    if (!toolbar) return;
    const measure = () => {
      setAvailableWidth(toolbar.getBoundingClientRect().width);
      setViewportWidth(window.innerWidth);
    };
    measure();
    window.addEventListener("resize", measure);
    if (typeof ResizeObserver === "undefined") {
      return () => window.removeEventListener("resize", measure);
    }
    const observer = new ResizeObserver(measure);
    observer.observe(toolbar);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", measure);
    };
  }, []);

  useEffect(() => {
    const refresh = () => setPreferences(loadWriterPreferences());
    window.addEventListener(writerPreferencesChangedEvent, refresh);
    window.addEventListener("storage", refresh);
    return () => {
      window.removeEventListener(writerPreferencesChangedEvent, refresh);
      window.removeEventListener("storage", refresh);
    };
  }, []);

  const updatePreferences = (next: Partial<WriterPreferences>) => {
    setPreferences(saveWriterPreferences({ ...preferences, ...next }));
  };
  if (!editor) {
    return (
      <div
        ref={toolbarRef}
        className={cn(
          "shrink-0 border-b border-[var(--border-subtle)]",
          integratedWorkspace ? "h-14 bg-[var(--surface-panel)]" : "h-11 bg-[var(--surface-paper)]",
        )}
      />
    );
  }

  const formatButtons = [
    {
      label: "Курсив",
      icon: Italic,
      active: editor.isActive("italic"),
      action: () => editor.chain().focus().toggleItalic().run(),
    },
    {
      label: "Полужирный",
      icon: Bold,
      active: editor.isActive("bold"),
      action: () => editor.chain().focus().toggleBold().run(),
    },
    {
      label: "Подчёркнутый",
      icon: UnderlineIcon,
      active: editor.isActive("underline"),
      action: () => editor.chain().focus().toggleUnderline().run(),
    },
    {
      label: "Зачёркнутый",
      icon: Strikethrough,
      active: editor.isActive("strike"),
      action: () => editor.chain().focus().toggleStrike().run(),
    },
    {
      label: "Строчный код",
      icon: Code2,
      active: editor.isActive("code"),
      action: () => editor.chain().focus().toggleCode().run(),
    },
    {
      label: "Строчная формула",
      icon: Sigma,
      active: editor.isActive("inlineMath"),
      action: () => {
        void requestInput({
          title: "Вставить строчную формулу",
          description: "Введите исходник LaTeX без символов $. HTML и сетевые команды отключены.",
          label: "Исходник LaTeX",
          defaultValue: "x^2 + y^2",
          icon: Sigma,
          confirmLabel: "Вставить",
        }).then((latex) => {
          if (!latex?.trim()) return;
          editor.commands.insertInlineMath({ latex: latex.trim() });
        });
      },
    },
    {
      label: "Цитата",
      icon: Quote,
      active: editor.isActive("blockquote"),
      action: () => editor.chain().focus().toggleBlockquote().run(),
    },
    {
      label: "Маркированный список",
      icon: List,
      active: editor.isActive("bulletList"),
      action: () => editor.chain().focus().toggleBulletList().run(),
    },
    {
      label: "Нумерованный список",
      icon: ListOrdered,
      active: editor.isActive("orderedList"),
      action: () => editor.chain().focus().toggleOrderedList().run(),
    },
  ];

  return (
    <div
      ref={toolbarRef}
      data-toolbar-mode={preferences.toolbarMode}
      data-toolbar-presentation={presentation}
      className={cn(
        "flex shrink-0 items-center gap-0.5 overflow-hidden border-b border-[var(--border-subtle)] px-2",
        preferences.toolbarMode === "expanded" ? "min-h-14 flex-wrap py-1" : "min-h-14 flex-nowrap",
        integratedWorkspace ? "bg-[var(--surface-panel)]" : "min-h-11 bg-[var(--surface-paper)]",
      )}
      role="toolbar"
      aria-label="Форматирование текста"
    >
      <Select
        aria-label="Стиль абзаца"
        className={cn(
          "mr-1 h-9 shrink-0 border-0 bg-transparent text-[13px]",
          presentation === "compact" ? "w-28" : "w-36",
        )}
        value={
          editor.isActive("heading", { level: 1 })
            ? "h1"
            : editor.isActive("heading", { level: 2 })
              ? "h2"
              : editor.isActive("heading", { level: 3 })
                ? "h3"
                : "paragraph"
        }
        onChange={(event) => {
          const value = event.target.value;
          if (value === "paragraph") {
            editor.chain().focus().setParagraph().run();
          } else {
            editor
              .chain()
              .focus()
              .toggleHeading({ level: Number(value.slice(1)) as 1 | 2 | 3 })
              .run();
          }
        }}
      >
        <option value="paragraph">Обычный текст</option>
        <option value="h1">Заголовок 1</option>
        <option value="h2">Заголовок 2</option>
        <option value="h3">Заголовок 3</option>
      </Select>
      <span className="mx-1 h-5 w-px shrink-0 bg-[var(--border-subtle)]" aria-hidden="true" />
      {formatButtons
        .filter(({ label }) => presentation !== "expanded" || label !== "Строчная формула")
        .slice(0, presentation === "expanded" ? formatButtons.length : 2)
        .map(({ label, icon: Icon, active, action }) => (
          <Button
            key={label}
            type="button"
            variant="ghost"
            size="icon"
            className={cn(
              "size-9 min-h-9 shrink-0",
              active && "bg-[var(--accent-soft)] text-[var(--accent-ink)]",
            )}
            aria-label={label}
            title={label}
            aria-pressed={active}
            onClick={action}
          >
            <Icon className="size-4" aria-hidden="true" />
          </Button>
        ))}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-9 min-h-9 shrink-0"
            aria-label="Типографика"
            title="Типографика"
          >
            <TextCursorInput className="size-4" aria-hidden="true" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" aria-label="Типографика" className="w-64 p-3">
          <label htmlFor="editor-toolbar-line-height" className="grid gap-1.5 text-xs font-medium">
            Межстрочный интервал
            <Select
              id="editor-toolbar-line-height"
              value={preferences.lineHeight}
              onChange={(event) =>
                updatePreferences({ lineHeight: Number(event.target.value) as EditorLineHeight })
              }
            >
              <option value={1.5}>1,5 — плотный</option>
              <option value={1.75}>1,75 — обычный</option>
              <option value={2}>2 — свободный</option>
            </Select>
          </label>
          <label
            htmlFor="editor-toolbar-paragraph-spacing"
            className="mt-3 grid gap-1.5 text-xs font-medium"
          >
            Между абзацами
            <Select
              id="editor-toolbar-paragraph-spacing"
              value={preferences.paragraphSpacing}
              onChange={(event) =>
                updatePreferences({
                  paragraphSpacing: Number(event.target.value) as EditorParagraphSpacing,
                })
              }
            >
              <option value={0.65}>0,65 em</option>
              <option value={0.9}>0,9 em</option>
              <option value={1.1}>1,1 em</option>
            </Select>
          </label>
          <label
            htmlFor="editor-toolbar-list-spacing"
            className="mt-3 grid gap-1.5 text-xs font-medium"
          >
            Между пунктами списка
            <Select
              id="editor-toolbar-list-spacing"
              value={preferences.listItemSpacing}
              onChange={(event) =>
                updatePreferences({
                  listItemSpacing: Number(event.target.value) as EditorListItemSpacing,
                })
              }
            >
              <option value={0}>Без интервала</option>
              <option value={0.25}>0,25 em</option>
              <option value={0.5}>0,5 em</option>
            </Select>
          </label>
        </DropdownMenuContent>
      </DropdownMenu>
      {presentation === "expanded" ? (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="size-9 min-h-9 shrink-0"
          aria-label="Вставить разделитель сцены"
          title="Вставить разделитель сцены"
          onClick={() => editor.chain().focus().insertContent({ type: "sceneBreak" }).run()}
        >
          <Minus className="size-4" aria-hidden="true" />
        </Button>
      ) : null}
      <span className="mx-1 h-5 w-px shrink-0 bg-[var(--border-subtle)]" aria-hidden="true" />
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-9 min-h-9 shrink-0"
            aria-label="Вставить блок"
            title="Вставить блок"
          >
            <Plus className="size-4" aria-hidden="true" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" aria-label="Вставить блок">
          <DropdownMenuItem
            onSelect={() =>
              editor.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()
            }
          >
            <Table2 className="size-4" aria-hidden="true" />
            Таблица 3 × 3
          </DropdownMenuItem>
          <DropdownMenuItem
            onSelect={() => {
              void requestInput({
                title: "Вставить формулу",
                description: "Формула займёт отдельный блок. HTML и сетевые команды отключены.",
                label: "Исходник LaTeX",
                defaultValue: "\\frac{-b \\pm \\sqrt{b^2 - 4ac}}{2a}",
                icon: Sigma,
                confirmLabel: "Вставить",
                multiline: true,
                rows: 4,
              }).then((latex) => {
                if (!latex?.trim()) return;
                editor.commands.insertMathBlock({ latex: latex.trim() });
              });
            }}
          >
            <Sigma className="size-4" aria-hidden="true" />
            Формула
          </DropdownMenuItem>
          <DropdownMenuItem
            onSelect={() => {
              void requestSelect({
                title: "Вставить блок кода",
                description: "Язык сохраняется в документе и Markdown-экспорте.",
                label: "Язык",
                defaultValue: "plain",
                icon: Code2,
                confirmLabel: "Вставить",
                options: codeLanguageOptions.map((option) => ({
                  value: option.value || "plain",
                  label: option.label,
                })),
              }).then((language) => {
                if (!language) return;
                editor
                  .chain()
                  .focus()
                  .insertContent({
                    type: "codeBlock",
                    attrs: { language: language === "plain" ? null : language },
                  })
                  .run();
              });
            }}
          >
            <Code2 className="size-4" aria-hidden="true" />
            Блок кода
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={imageUploading}
            onSelect={() => imageInputRef.current?.click()}
          >
            <ImagePlus className="size-4" aria-hidden="true" />
            {imageUploading ? "Загружаем изображение…" : "Изображение"}
          </DropdownMenuItem>
          <DropdownMenuItem
            onSelect={() => {
              void requestInput({
                title: "Вставить Mermaid-схему",
                description:
                  "Поддерживаются flowchart, sequenceDiagram, stateDiagram, classDiagram и другие виды Mermaid.",
                label: "Исходный код Mermaid",
                defaultValue:
                  "flowchart TD\n  Start[Начало] --> Choice{Условие}\n  Choice --> Result[Результат]",
                icon: Workflow,
                confirmLabel: "Вставить",
                multiline: true,
                rows: 10,
              }).then((code) => {
                if (!code?.trim()) return;
                editor
                  .chain()
                  .focus()
                  .insertContent({ type: "mermaidBlock", attrs: { code: code.trim() } })
                  .run();
              });
            }}
          >
            <Workflow className="size-4" aria-hidden="true" />
            Mermaid-схема
          </DropdownMenuItem>
          <DropdownMenuItem
            onSelect={() =>
              editor
                .chain()
                .focus()
                .insertContent({
                  type: "callout",
                  attrs: { kind: "info" },
                  content: [
                    {
                      type: "paragraph",
                      content: [{ type: "text", text: "Важная информация" }],
                    },
                  ],
                })
                .run()
            }
          >
            <CircleAlert className="size-4" aria-hidden="true" />
            Блок «Информация»
          </DropdownMenuItem>
          <DropdownMenuItem
            onSelect={() =>
              editor
                .chain()
                .focus()
                .insertContent({
                  type: "callout",
                  attrs: { kind: "idea" },
                  content: [{ type: "paragraph", content: [{ type: "text", text: "Новая идея" }] }],
                })
                .run()
            }
          >
            <Lightbulb className="size-4" aria-hidden="true" />
            Блок «Идея»
          </DropdownMenuItem>
          <DropdownMenuItem
            onSelect={() =>
              editor
                .chain()
                .focus()
                .insertContent({
                  type: "callout",
                  attrs: { kind: "warning" },
                  content: [
                    {
                      type: "paragraph",
                      content: [{ type: "text", text: "Риск или ограничение" }],
                    },
                  ],
                })
                .run()
            }
          >
            <TriangleAlert className="size-4" aria-hidden="true" />
            Блок «Предупреждение»
          </DropdownMenuItem>
          <DropdownMenuItem
            onSelect={() =>
              editor
                .chain()
                .focus()
                .insertContent({
                  type: "callout",
                  attrs: { kind: "todo" },
                  content: [
                    { type: "paragraph", content: [{ type: "text", text: "Что нужно сделать" }] },
                  ],
                })
                .run()
            }
          >
            <ListTodo className="size-4" aria-hidden="true" />
            Блок «TODO»
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-9 min-h-9 shrink-0"
        disabled={!editor.can().undo()}
        aria-label="Отменить"
        title="Отменить"
        onClick={() => editor.chain().focus().undo().run()}
      >
        <Undo2 className="size-4" aria-hidden="true" />
      </Button>
      {presentation === "expanded" ? (
        <>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-9 min-h-9 shrink-0"
            disabled={!editor.can().redo()}
            aria-label="Повторить"
            title="Повторить"
            onClick={() => editor.chain().focus().redo().run()}
          >
            <Redo2 className="size-4" aria-hidden="true" />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-9 min-h-9 shrink-0"
            aria-label="Найти в документе"
            title="Найти в документе"
            onClick={onFind}
          >
            <Search className="size-4" aria-hidden="true" />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-9 min-h-9 shrink-0"
            aria-label="Режим фокуса"
            title="Режим фокуса"
            onClick={onFocusMode}
          >
            <Focus className="size-4" aria-hidden="true" />
          </Button>
        </>
      ) : null}
      <div className="flex-1" />
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-9 min-h-9 shrink-0"
            aria-label="Дополнительное форматирование"
            title="Дополнительное форматирование"
          >
            <MoreHorizontal className="size-4" aria-hidden="true" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" aria-label="Дополнительное форматирование">
          {formatButtons
            .filter(({ label }, index) =>
              presentation === "expanded" ? label === "Строчная формула" : index >= 2,
            )
            .map(({ label, icon: Icon, active, action }) => (
              <DropdownMenuItem key={label} onSelect={action}>
                <Icon className="size-4" aria-hidden="true" />
                <span className="flex-1">{label}</span>
                {active ? <Check className="size-3.5" aria-label="Активно" /> : null}
              </DropdownMenuItem>
            ))}
          {presentation === "compact" ? (
            <>
              <DropdownMenuItem
                onSelect={() => editor.chain().focus().insertContent({ type: "sceneBreak" }).run()}
              >
                <Minus className="size-4" aria-hidden="true" />
                Вставить разделитель сцены
              </DropdownMenuItem>
              <DropdownMenuItem
                disabled={!editor.can().redo()}
                onSelect={() => editor.chain().focus().redo().run()}
              >
                <Redo2 className="size-4" aria-hidden="true" />
                Повторить
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={onFind}>
                <Search className="size-4" aria-hidden="true" />
                Найти в документе
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={onFocusMode}>
                <Focus className="size-4" aria-hidden="true" />
                Режим фокуса
              </DropdownMenuItem>
            </>
          ) : null}
          <DropdownMenuItem
            onSelect={() => {
              const previous = editor.getAttributes("link").href as string | undefined;
              void requestInput({
                title: "Добавить ссылку",
                description:
                  "Вставьте полный адрес. Пустое поле удалит ссылку с выделенного текста.",
                label: "Адрес ссылки",
                defaultValue: previous ?? "https://",
                icon: LinkIcon,
                confirmLabel: "Применить",
              }).then((url) => {
                if (url === undefined) {
                  return;
                }
                if (url === "") {
                  editor.chain().focus().extendMarkRange("link").unsetLink().run();
                  return;
                }
                editor.chain().focus().extendMarkRange("link").setLink({ href: url }).run();
              });
            }}
          >
            <LinkIcon className="size-4" aria-hidden="true" />
            Добавить ссылку
          </DropdownMenuItem>
          {editor.isActive("table") ? (
            <>
              <DropdownMenuItem onSelect={() => editor.chain().focus().addRowAfter().run()}>
                <Rows3 className="size-4" aria-hidden="true" />
                Добавить строку
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => editor.chain().focus().addColumnAfter().run()}>
                <Columns3 className="size-4" aria-hidden="true" />
                Добавить столбец
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => editor.chain().focus().deleteRow().run()}>
                <Trash2 className="size-4" aria-hidden="true" />
                Удалить строку
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => editor.chain().focus().deleteColumn().run()}>
                <Trash2 className="size-4" aria-hidden="true" />
                Удалить столбец
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => editor.chain().focus().deleteTable().run()}>
                <Trash2 className="size-4" aria-hidden="true" />
                Удалить таблицу
              </DropdownMenuItem>
            </>
          ) : null}
        </DropdownMenuContent>
      </DropdownMenu>
      <input
        ref={imageInputRef}
        type="file"
        accept="image/png,image/jpeg,image/gif"
        className="sr-only"
        aria-label="Выбрать изображение"
        onChange={(event) => {
          const input = event.currentTarget;
          const file = input.files?.[0];
          if (!file) return;
          const validation = validateEditorImageFile(file);
          if (!validation.valid) {
            showToast({ title: "Изображение не добавлено", description: validation.message });
            input.value = "";
            return;
          }
          setImageUploading(true);
          void uploadProjectImage(projectId, file)
            .then(({ media }) => {
              editor
                .chain()
                .focus()
                .insertContent({
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
            .finally(() => {
              setImageUploading(false);
              input.value = "";
            });
        }}
      />
    </div>
  );
}
