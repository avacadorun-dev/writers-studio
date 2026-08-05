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
    <div className="grid gap-2 sm:grid-cols-2" role="radiogroup" aria-label="Цветовая тема">
      {editorStyleOptions.map((option) => {
        const selected = option.id === value;
        return (
          <label
            key={option.id}
            className={cn(
              "group relative cursor-pointer rounded-[var(--radius-md)] border bg-[var(--surface-raised)] p-3 text-left transition-colors",
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
              checked={selected}
              className="peer sr-only"
              onChange={() => onChange(option.id)}
            />
            <span className="flex items-center justify-between gap-3">
              <span className="text-sm font-medium text-[var(--ink-primary)]">{option.label}</span>
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
            <span
              className="theme-palette-preview mt-3 grid grid-cols-6 overflow-hidden rounded-[var(--radius-sm)] border border-[var(--theme-preview-border)]"
              data-style-preview={option.id}
              role="img"
              aria-label={`Палитра: ${previewTokens.map((token) => token.label).join(", ")}`}
            >
              {previewTokens.map((token) => (
                <span
                  key={token.id}
                  className="theme-palette-swatch h-6"
                  data-preview-token={token.id}
                  title={token.label}
                  aria-hidden="true"
                />
              ))}
            </span>
          </label>
        );
      })}
    </div>
  );
}
