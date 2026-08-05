import type { Editor } from "@tiptap/react";
import {
  Bold,
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
  Strikethrough,
  Table2,
  Trash2,
  TriangleAlert,
  Underline as UnderlineIcon,
  Undo2,
  Workflow,
} from "lucide-react";
import { useRef, useState } from "react";

import { uploadProjectImage } from "~entities/project";
import { cn } from "~shared/lib";
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

interface FormattingToolbarProps {
  editor: Editor | null;
  onFocusMode: () => void;
  onFind: () => void;
  projectId: string;
  integratedWorkspace?: boolean;
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
  const [imageUploading, setImageUploading] = useState(false);
  if (!editor) {
    return (
      <div
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
      className={cn(
        "flex shrink-0 items-center gap-0.5 border-b border-[var(--border-subtle)] px-2",
        integratedWorkspace
          ? "h-14 bg-[var(--surface-panel)]"
          : "min-h-11 bg-[var(--surface-paper)]",
      )}
      role="toolbar"
      aria-label="Форматирование текста"
    >
      <Select
        aria-label="Стиль абзаца"
        className="mr-1 h-9 w-36 shrink-0 border-0 bg-transparent text-[13px]"
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
      {formatButtons.slice(0, 2).map(({ label, icon: Icon, active, action }) => (
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
          aria-pressed={active}
          onClick={action}
        >
          <Icon className="size-4" aria-hidden="true" />
        </Button>
      ))}
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-9 min-h-9 shrink-0"
        aria-label="Вставить разделитель сцены"
        onClick={() => editor.chain().focus().insertContent({ type: "sceneBreak" }).run()}
      >
        <Minus className="size-4" aria-hidden="true" />
      </Button>
      <span className="mx-1 h-5 w-px shrink-0 bg-[var(--border-subtle)]" aria-hidden="true" />
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-9 min-h-9 shrink-0"
            aria-label="Вставить блок"
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
              void requestSelect({
                title: "Вставить блок кода",
                description: "Язык сохраняется в документе и Markdown-экспорте.",
                label: "Язык",
                defaultValue: "plain",
                icon: Code2,
                confirmLabel: "Вставить",
                options: [
                  { value: "plain", label: "Обычный текст" },
                  { value: "javascript", label: "JavaScript" },
                  { value: "typescript", label: "TypeScript" },
                  { value: "json", label: "JSON" },
                  { value: "lua", label: "Lua" },
                  { value: "csharp", label: "C#" },
                  { value: "cpp", label: "C++" },
                  { value: "python", label: "Python" },
                  { value: "sql", label: "SQL" },
                ],
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
        onClick={() => editor.chain().focus().undo().run()}
      >
        <Undo2 className="size-4" aria-hidden="true" />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-9 min-h-9 shrink-0"
        disabled={!editor.can().redo()}
        aria-label="Повторить"
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
        onClick={onFocusMode}
      >
        <Focus className="size-4" aria-hidden="true" />
      </Button>
      <div className="flex-1" />
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-9 min-h-9 shrink-0"
            aria-label="Дополнительное форматирование"
          >
            <MoreHorizontal className="size-4" aria-hidden="true" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" aria-label="Дополнительное форматирование">
          {formatButtons.slice(2).map(({ label, icon: Icon, action }) => (
            <DropdownMenuItem key={label} onSelect={action}>
              <Icon className="size-4" aria-hidden="true" />
              {label}
            </DropdownMenuItem>
          ))}
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
                    width: media.width,
                    height: media.height,
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
