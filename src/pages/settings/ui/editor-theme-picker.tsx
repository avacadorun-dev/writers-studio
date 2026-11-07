import { Check } from "lucide-react";

import { cn, type EditorStyle, editorStyleOptions } from "~shared/lib";

const previewTokens = [
  { id: "background", label: "Фон" },
  { id: "panel", label: "Панель" },
  { id: "document", label: "Документ" },
  { id: "surface", label: "Поверхность" },
  { id: "border", label: "Граница" },
  { id: "text-primary", label: "Основной текст" },
  { id: "text-secondary", label: "Вторичный текст" },
  { id: "accent", label: "Акцент" },
  { id: "accent-secondary", label: "Дополнительный акцент" },
  { id: "selected", label: "Выбранное" },
  { id: "hover", label: "Наведение" },
  { id: "error", label: "Ошибка" },
] as const;

export function EditorThemePicker({
  value,
  onChange,
}: {
  value: EditorStyle;
  onChange: (style: EditorStyle) => void;
}) {
  return (
    <div
      className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4"
      role="radiogroup"
      aria-label="Цветовая тема"
    >
      {editorStyleOptions.map((option) => {
        const selected = option.id === value;
        return (
          <label
            key={option.id}
            className={cn(
              "group relative min-w-0 cursor-pointer rounded-[var(--radius-lg)] border bg-[var(--surface-raised)] p-3 text-left transition-colors",
              "hover:border-[var(--accent)] hover:bg-[var(--surface-subtle)] has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-[var(--focus)]",
              selected
                ? "border-[var(--accent)] shadow-[inset_0_0_0_1px_var(--accent)]"
                : "border-[var(--border-subtle)]",
            )}
          >
            <input
              type="radio"
              name="editor-style"
              value={option.id}
              aria-label={option.label}
              checked={selected}
              className="peer sr-only"
              onChange={() => onChange(option.id)}
            />
            <span
              className="theme-palette-preview block overflow-hidden rounded-[var(--radius-md)] border border-[var(--theme-preview-border)] bg-[var(--theme-preview-background)]"
              data-style-preview={option.id}
              role="img"
              aria-label={`Предпросмотр темы «${option.label}»`}
            >
              <span className="flex h-[100px]" aria-hidden="true">
                <span className="flex w-10 shrink-0 flex-col gap-2 border-r border-[var(--theme-preview-border)] bg-[var(--theme-preview-panel)] p-2.5">
                  <span className="mb-1 size-3 rounded-full bg-[var(--theme-preview-accent)]" />
                  <span className="h-1 rounded-full bg-[var(--theme-preview-text-secondary)]/40" />
                  <span className="h-1 rounded-full bg-[var(--theme-preview-accent-secondary)]/70" />
                  <span className="h-1 rounded-full bg-[var(--theme-preview-text-secondary)]/25" />
                </span>
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className="flex h-5 items-center gap-1 border-b border-[var(--theme-preview-border)] bg-[var(--theme-preview-surface)] px-2.5">
                    <span className="size-1 rounded-full bg-[var(--theme-preview-accent)]" />
                    <span className="h-0.5 w-8 rounded-full bg-[var(--theme-preview-text-secondary)]/35" />
                    <span className="ml-auto h-1 w-5 rounded-full bg-[var(--theme-preview-accent-secondary)]" />
                  </span>
                  <span className="mx-auto mt-2 flex w-[72%] min-w-0 flex-1 flex-col rounded-t border border-b-0 border-[var(--theme-preview-border)] bg-[var(--theme-preview-document)] px-3 pt-1.5">
                    <span className="font-display text-lg leading-6 text-[var(--theme-preview-text-primary)]">
                      Aa
                    </span>
                    <span className="mt-1 h-0.5 w-full rounded-full bg-[var(--theme-preview-text-secondary)]/40" />
                    <span className="mt-1.5 h-0.5 w-[85%] rounded-full bg-[var(--theme-preview-text-secondary)]/30" />
                    <span className="mt-1.5 h-0.5 w-[60%] rounded-full bg-[var(--theme-preview-text-secondary)]/30" />
                  </span>
                </span>
              </span>
              <span className="grid grid-cols-12 border-t border-[var(--theme-preview-border)]">
                {previewTokens.map((token) => (
                  <span
                    key={token.id}
                    className="theme-palette-swatch h-2"
                    data-preview-token={token.id}
                    title={token.label}
                    aria-hidden="true"
                  />
                ))}
              </span>
            </span>
            <span className="mt-3 flex min-h-5 items-center justify-between gap-3">
              <span className="text-[13px] font-medium text-[var(--ink-primary)]">
                {option.label}
              </span>
              <span
                className={cn(
                  "grid size-5 shrink-0 place-items-center rounded-full border transition-colors",
                  selected
                    ? "border-[var(--accent-surface)] bg-[var(--accent-surface)] text-[var(--ink-on-solid)]"
                    : "border-[var(--border-strong)] text-transparent",
                )}
                aria-hidden="true"
              >
                <Check className="size-3" strokeWidth={3} />
              </span>
            </span>
          </label>
        );
      })}
    </div>
  );
}
