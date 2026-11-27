import { Plus, Tags, Trash2 } from "lucide-react";
import { type FormEvent, useEffect, useMemo, useState } from "react";
import type {
  CanvasFieldType,
  CanvasRelationType,
  CanvasTypeDefinition,
  CanvasTypeSchema,
} from "~entities/canvas";
import { t } from "~shared/i18n";
import { Button, Checkbox, Field, Input, ModalDialog, Select } from "~shared/ui";

interface CanvasSchemaEditorProps {
  open: boolean;
  schema: CanvasTypeSchema;
  pending?: boolean;
  onOpenChange: (open: boolean) => void;
  onSave: (schema: CanvasTypeSchema) => void;
}

const fieldTypes: Array<{ value: CanvasFieldType; label: string }> = [
  { value: "short-text", label: t("Короткий текст") },
  { value: "markdown", label: "Markdown" },
  { value: "number", label: t("Число") },
  { value: "checkbox", label: t("Флаг") },
  { value: "single-select", label: t("Один вариант") },
  { value: "multi-select", label: t("Несколько вариантов") },
  { value: "date", label: t("Дата") },
  { value: "duration", label: t("Длительность") },
  { value: "document", label: t("Документ") },
  { value: "media", label: t("Медиа") },
  { value: "canvas", label: t("Доска") },
];

const colors = ["1", "2", "3", "4", "5", "6"] as const;

export function CanvasSchemaEditor({
  open,
  schema,
  pending = false,
  onOpenChange,
  onSave,
}: CanvasSchemaEditorProps) {
  const [draft, setDraft] = useState(schema);
  const [selectedCardId, setSelectedCardId] = useState(schema.cardTypes[0]?.id ?? "");

  useEffect(() => {
    if (!open) return;
    setDraft(schema);
    setSelectedCardId(schema.cardTypes[0]?.id ?? "");
  }, [open, schema]);

  const selectedCard = draft.cardTypes.find((item) => item.id === selectedCardId);
  const relationNames = useMemo(
    () => new Map(draft.relationTypes.map((item) => [item.id, item.name])),
    [draft.relationTypes],
  );

  const updateCard = (update: (card: CanvasTypeDefinition) => CanvasTypeDefinition) => {
    setDraft((current) => ({
      ...current,
      cardTypes: current.cardTypes.map((item) =>
        item.id === selectedCardId ? update(item) : item,
      ),
    }));
  };

  const addCard = () => {
    const id = nextTypeId("custom-card", new Set(draft.cardTypes.map((item) => item.id)));
    setDraft((current) => ({
      ...current,
      cardTypes: [
        ...current.cardTypes,
        {
          id,
          name: t("Новый тип"),
          category: "custom",
          color: "5",
          icon: "StickyNote",
          fields: [],
        },
      ],
    }));
    setSelectedCardId(id);
  };

  const addField = () => {
    if (!selectedCard) return;
    const id = nextTypeId("field", new Set(selectedCard.fields.map((item) => item.id)));
    updateCard((card) => ({
      ...card,
      fields: [...card.fields, { id, name: t("Новое поле"), type: "short-text" }],
    }));
  };

  const updateRelation = (
    relationId: string,
    update: (relation: CanvasRelationType) => CanvasRelationType,
  ) => {
    setDraft((current) => ({
      ...current,
      relationTypes: current.relationTypes.map((item) =>
        item.id === relationId ? update(item) : item,
      ),
    }));
  };

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    onSave(draft);
  };

  return (
    <ModalDialog
      open={open}
      onOpenChange={(next) => !pending && onOpenChange(next)}
      title={t("Каталог Canvas")}
      description={t(
        "Типы и связи общие для всех досок проекта и сохраняются отдельными ревизиями.",
      )}
      icon={Tags}
      className="sm:w-[min(920px,calc(100vw-40px))]"
    >
      <form onSubmit={handleSubmit} className="grid gap-5 lg:grid-cols-[230px_1fr]">
        <aside className="space-y-2 border-b border-[var(--border-subtle)] pb-4 lg:border-b-0 lg:border-r lg:pr-4">
          <div className="flex items-center justify-between gap-2">
            <strong className="text-sm">{t("Типы карточек")}</strong>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-8"
              aria-label={t("Добавить тип карточки")}
              onClick={addCard}
            >
              <Plus className="size-4" aria-hidden="true" />
              <span className="sr-only">{t("Добавить тип карточки")}</span>
            </Button>
          </div>
          <div className="max-h-72 space-y-1 overflow-auto">
            {draft.cardTypes.map((card) => (
              <button
                key={card.id}
                type="button"
                onClick={() => setSelectedCardId(card.id)}
                className={`flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-sm ${
                  selectedCardId === card.id
                    ? "bg-[var(--accent-soft)] text-[var(--accent-ink)]"
                    : "hover:bg-[var(--surface-subtle)]"
                }`}
              >
                <span className="size-2.5 shrink-0 rounded-full bg-[var(--accent)]" />
                <span className="truncate">{card.name}</span>
              </button>
            ))}
          </div>
        </aside>

        <section className="min-w-0 space-y-5">
          {selectedCard ? (
            <>
              <div className="grid gap-3 sm:grid-cols-2">
                <Field label={t("Название")}>
                  <Input
                    value={selectedCard.name}
                    maxLength={120}
                    onChange={(event) =>
                      updateCard((card) => ({ ...card, name: event.target.value }))
                    }
                  />
                </Field>
                <Field
                  label={t("Стабильный ID")}
                  hint={t("Используется в данных и не меняется автоматически.")}
                >
                  <Input value={selectedCard.id} readOnly />
                </Field>
                <Field label={t("Цвет")}>
                  <Select
                    value={selectedCard.color}
                    onChange={(event) =>
                      updateCard((card) => ({
                        ...card,
                        color: event.target.value as CanvasTypeDefinition["color"],
                      }))
                    }
                  >
                    {colors.map((color) => (
                      <option key={color} value={color}>
                        {t("Цвет ")}
                        {color}
                      </option>
                    ))}
                  </Select>
                </Field>
                <Field label={t("Иконка Lucide")}>
                  <Input
                    value={selectedCard.icon}
                    maxLength={80}
                    onChange={(event) =>
                      updateCard((card) => ({ ...card, icon: event.target.value }))
                    }
                  />
                </Field>
              </div>

              <div>
                <div className="mb-2 flex items-center justify-between gap-2">
                  <strong className="text-sm">{t("Поля")}</strong>
                  <Button type="button" variant="secondary" size="compact" onClick={addField}>
                    <Plus className="size-3.5" aria-hidden="true" />
                    {t(" Добавить поле")}
                  </Button>
                </div>
                <div className="space-y-2">
                  {selectedCard.fields.map((field, index) => (
                    <div
                      key={field.id}
                      className="grid items-center gap-2 rounded-lg border border-[var(--border-subtle)] p-2 sm:grid-cols-[1fr_150px_auto_auto]"
                    >
                      <Input
                        className="h-9"
                        aria-label={t("Название поля {0}", index + 1)}
                        value={field.name}
                        onChange={(event) =>
                          updateCard((card) => ({
                            ...card,
                            fields: card.fields.map((item) =>
                              item.id === field.id ? { ...item, name: event.target.value } : item,
                            ),
                          }))
                        }
                      />
                      <Select
                        className="h-9"
                        aria-label={t("Тип поля {0}", index + 1)}
                        value={field.type}
                        onChange={(event) =>
                          updateCard((card) => ({
                            ...card,
                            fields: card.fields.map((item) =>
                              item.id === field.id
                                ? { ...item, type: event.target.value as CanvasFieldType }
                                : item,
                            ),
                          }))
                        }
                      >
                        {fieldTypes.map((item) => (
                          <option key={item.value} value={item.value}>
                            {item.label}
                          </option>
                        ))}
                      </Select>
                      <div className="flex items-center gap-2 text-xs">
                        <Checkbox
                          aria-label={t("Поле {0} обязательно", field.name)}
                          checked={field.required ?? false}
                          onCheckedChange={(checked) =>
                            updateCard((card) => ({
                              ...card,
                              fields: card.fields.map((item) =>
                                item.id === field.id
                                  ? { ...item, required: checked === true }
                                  : item,
                              ),
                            }))
                          }
                        />
                        {t("Обязательно")}
                      </div>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        className="size-8 text-[var(--danger)]"
                        aria-label={t("Удалить поле {0}", field.name)}
                        onClick={() =>
                          updateCard((card) => ({
                            ...card,
                            fields: card.fields.filter((item) => item.id !== field.id),
                          }))
                        }
                      >
                        <Trash2 className="size-3.5" aria-hidden="true" />
                      </Button>
                    </div>
                  ))}
                </div>
              </div>
            </>
          ) : null}

          <div>
            <div className="mb-2 flex items-center justify-between gap-2">
              <strong className="text-sm">{t("Типы связей")}</strong>
              <Button
                type="button"
                variant="secondary"
                size="compact"
                onClick={() => {
                  const id = nextTypeId(
                    "custom-relation",
                    new Set(draft.relationTypes.map((item) => item.id)),
                  );
                  setDraft((current) => ({
                    ...current,
                    relationTypes: [
                      ...current.relationTypes,
                      { id, name: t("Новая связь"), color: "5", directed: true },
                    ],
                  }));
                }}
              >
                <Plus className="size-3.5" aria-hidden="true" />
                {t(" Добавить связь")}
              </Button>
            </div>
            <div className="max-h-56 space-y-2 overflow-auto">
              {draft.relationTypes.map((relation) => (
                <div
                  key={relation.id}
                  className="grid items-center gap-2 rounded-lg border border-[var(--border-subtle)] p-2 sm:grid-cols-[1fr_100px_auto_auto]"
                >
                  <Input
                    className="h-9"
                    value={relation.name}
                    aria-label={t("Название связи {0}", relation.id)}
                    onChange={(event) =>
                      updateRelation(relation.id, (item) => ({ ...item, name: event.target.value }))
                    }
                  />
                  <Select
                    className="h-9"
                    value={relation.color}
                    aria-label={t("Цвет связи {0}", relation.id)}
                    onChange={(event) =>
                      updateRelation(relation.id, (item) => ({
                        ...item,
                        color: event.target.value as CanvasRelationType["color"],
                      }))
                    }
                  >
                    {colors.map((color) => (
                      <option key={color} value={color}>
                        {t("Цвет ")}
                        {color}
                      </option>
                    ))}
                  </Select>
                  <div className="flex items-center gap-2 text-xs">
                    <Checkbox
                      aria-label={t("Связь {0} направленная", relation.name)}
                      checked={relation.directed}
                      onCheckedChange={(checked) =>
                        updateRelation(relation.id, (item) => ({
                          ...item,
                          directed: checked === true,
                        }))
                      }
                    />
                    {t("Стрелка")}
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="size-8 text-[var(--danger)]"
                    aria-label={t("Удалить связь {0}", relation.name)}
                    title={t("Используется: {0}", relationNames.get(relation.id) ?? relation.id)}
                    onClick={() =>
                      setDraft((current) => ({
                        ...current,
                        cardTypes: current.cardTypes.map((card) => ({
                          ...card,
                          allowedRelations: card.allowedRelations?.filter(
                            (id) => id !== relation.id,
                          ),
                        })),
                        relationTypes: current.relationTypes.filter(
                          (item) => item.id !== relation.id,
                        ),
                      }))
                    }
                  >
                    <Trash2 className="size-3.5" aria-hidden="true" />
                  </Button>
                </div>
              ))}
            </div>
          </div>

          <div className="flex justify-end gap-2 border-t border-[var(--border-subtle)] pt-4">
            <Button
              type="button"
              variant="ghost"
              disabled={pending}
              onClick={() => onOpenChange(false)}
            >
              {t("Отмена")}
            </Button>
            <Button type="submit" disabled={pending}>
              {pending ? t("Сохраняем…") : t("Сохранить каталог")}
            </Button>
          </div>
        </section>
      </form>
    </ModalDialog>
  );
}

export function nextTypeId(base: string, existing: Set<string>): string {
  if (!existing.has(base)) return base;
  let suffix = 2;
  while (existing.has(`${base}-${suffix}`)) suffix += 1;
  return `${base}-${suffix}`;
}
