import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Search, SpellCheck2, Trash2 } from "lucide-react";
import { type FormEvent, useState } from "react";

import {
  createDictionaryTerm,
  type DictionaryTerm,
  deleteDictionaryTerm,
  getProject,
  listDictionary,
  updateDictionaryTerm,
} from "~entities/project";
import {
  Banner,
  Button,
  Field,
  Input,
  Select,
  Spinner,
  Switch,
  useActionDialog,
  useToast,
} from "~shared/ui";
import { ProjectUtilityShell } from "~widgets/project-utility-shell";

type TermForm = Pick<DictionaryTerm, "canonical" | "variants" | "kind" | "language">;

const emptyForm: TermForm = {
  canonical: "",
  variants: [],
  kind: "character",
  language: "ru",
};

export function DictionaryPage({ projectId }: { projectId: string }) {
  const queryClient = useQueryClient();
  const { showToast } = useToast();
  const { requestConfirmation } = useActionDialog();
  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState<TermForm>(emptyForm);
  const [variants, setVariants] = useState("");
  const [query, setQuery] = useState("");
  const [kindFilter, setKindFilter] = useState<DictionaryTerm["kind"] | "all">("all");
  const project = useQuery({
    queryKey: ["project", projectId],
    queryFn: () => getProject(projectId),
  });
  const dictionary = useQuery({
    queryKey: ["dictionary", projectId],
    queryFn: () => listDictionary(projectId),
  });
  const add = useMutation({
    mutationFn: () =>
      createDictionaryTerm(projectId, {
        ...form,
        variants: variants
          .split(",")
          .map((variant) => variant.trim())
          .filter(Boolean),
      }),
    onSuccess: (response) => {
      setEditing(false);
      setForm(emptyForm);
      setVariants("");
      void queryClient.invalidateQueries({ queryKey: ["dictionary", projectId] });
      showToast({ title: "Термин добавлен", description: response.notice });
    },
  });
  const toggle = useMutation({
    mutationFn: (term: DictionaryTerm) =>
      updateDictionaryTerm(term.id, {
        canonical: term.canonical,
        variants: term.variants,
        kind: term.kind,
        language: term.language,
        active: !term.active,
      }),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["dictionary", projectId] }),
  });
  const remove = useMutation({
    mutationFn: async (term: DictionaryTerm) => {
      await deleteDictionaryTerm(term.id);
      return term;
    },
    onSuccess: (term) => {
      void queryClient.invalidateQueries({ queryKey: ["dictionary", projectId] });
      showToast({
        title: "Термин удалён",
        description: "Уже написанный текст и исходные расшифровки не изменены.",
        actionLabel: "Отменить",
        onAction: () => {
          void createDictionaryTerm(projectId, {
            canonical: term.canonical,
            variants: term.variants,
            kind: term.kind,
            language: term.language,
          }).then(() => queryClient.invalidateQueries({ queryKey: ["dictionary", projectId] }));
        },
      });
    },
  });

  if (project.isPending || !project.data) {
    return <Spinner label="Открываем словарь…" />;
  }

  const normalizedQuery = query.trim().toLocaleLowerCase();
  const terms =
    dictionary.data?.terms.filter(
      (term) =>
        (kindFilter === "all" || term.kind === kindFilter) &&
        (!normalizedQuery ||
          term.canonical.toLocaleLowerCase().includes(normalizedQuery) ||
          term.variants.some((variant) => variant.toLocaleLowerCase().includes(normalizedQuery))),
    ) ?? [];

  return (
    <ProjectUtilityShell
      project={project.data.project}
      nodes={project.data.nodes}
      expandedTree
      contentClassName="px-5 py-7 lg:px-8"
    >
      <div className="mx-auto w-full max-w-[1096px]">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p className="text-[10px] font-semibold tracking-[0.14em] text-[var(--ink-tertiary)] uppercase">
              Проект · русский и английский
            </p>
            <h1 className="font-display mt-2 text-[34px] leading-tight">Словарь имён и терминов</h1>
            <p className="mt-2 text-sm text-[var(--ink-secondary)]">
              Словарь помогает будущим расшифровкам. Уже написанный текст не изменится.
            </p>
          </div>
          <Button type="button" onClick={() => setEditing(true)}>
            <Plus className="size-4" aria-hidden="true" />
            Добавить термин
          </Button>
        </div>
        <div className="mt-5 grid gap-2 sm:grid-cols-[minmax(0,1fr)_180px]">
          <div className="relative">
            <Search
              className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-[var(--ink-tertiary)]"
              aria-hidden="true"
            />
            <Input
              className="pl-10"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Найти имя, вариант или термин"
              aria-label="Поиск по словарю"
            />
          </div>
          <Select
            value={kindFilter}
            onChange={(event) =>
              setKindFilter(event.target.value as DictionaryTerm["kind"] | "all")
            }
            aria-label="Фильтр по типу термина"
          >
            <option value="all">Все типы</option>
            <option value="character">Персонажи</option>
            <option value="place">Места</option>
            <option value="term">Термины</option>
            <option value="other">Другое</option>
          </Select>
        </div>
        {editing ? (
          <form
            className="my-6 grid gap-4 rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-5 sm:grid-cols-2"
            onSubmit={(event: FormEvent) => {
              event.preventDefault();
              add.mutate();
            }}
          >
            <Field label="Правильное написание">
              <Input
                autoFocus
                required
                maxLength={160}
                value={form.canonical}
                onChange={(event) =>
                  setForm((current) => ({ ...current, canonical: event.target.value }))
                }
              />
            </Field>
            <Field label="Варианты через запятую" hint="Например: Аня, Анна Сергеевна">
              <Input value={variants} onChange={(event) => setVariants(event.target.value)} />
            </Field>
            <Field label="Тип">
              <Select
                value={form.kind}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    kind: event.target.value as TermForm["kind"],
                  }))
                }
              >
                <option value="character">Персонаж</option>
                <option value="place">Место</option>
                <option value="term">Термин</option>
                <option value="other">Другое</option>
              </Select>
            </Field>
            <Field label="Язык">
              <Select
                value={form.language}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    language: event.target.value as "ru" | "en",
                  }))
                }
              >
                <option value="ru">Русский</option>
                <option value="en">English</option>
              </Select>
            </Field>
            {add.error ? (
              <Banner
                className="sm:col-span-2"
                variant="danger"
                title="Не удалось добавить"
                body={add.error.message}
              />
            ) : null}
            <div className="flex justify-end gap-2 sm:col-span-2">
              <Button type="button" variant="ghost" onClick={() => setEditing(false)}>
                Отмена
              </Button>
              <Button type="submit" disabled={add.isPending || !form.canonical.trim()}>
                Сохранить термин
              </Button>
            </div>
          </form>
        ) : null}
        {dictionary.isPending ? (
          <Spinner label="Загружаем словарь…" />
        ) : (
          <div className="mt-6 overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-paper)]">
            {terms.map((term) => (
              <article
                key={term.id}
                className="flex items-center gap-4 border-b border-[var(--border-subtle)] px-4 py-3 last:border-b-0"
              >
                <SpellCheck2 className="size-4 shrink-0 text-[var(--accent)]" aria-hidden="true" />
                <div className="min-w-0 flex-1">
                  <h2 className="text-sm font-medium">{term.canonical}</h2>
                  <p className="truncate text-xs text-[var(--ink-tertiary)]">
                    {term.variants.length ? term.variants.join(", ") : "Без вариантов"} ·{" "}
                    {kindLabel(term.kind)}
                  </p>
                </div>
                <label
                  htmlFor={`dictionary-active-${term.id}`}
                  className="flex cursor-pointer items-center gap-2 text-xs text-[var(--ink-secondary)]"
                >
                  <Switch
                    id={`dictionary-active-${term.id}`}
                    checked={term.active}
                    disabled={toggle.isPending}
                    onCheckedChange={() => toggle.mutate(term)}
                  />
                  Активен
                </label>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label={`Удалить ${term.canonical}`}
                  onClick={() => {
                    void requestConfirmation({
                      title: `Удалить термин «${term.canonical}»?`,
                      description:
                        "Уже написанный текст и исходные расшифровки останутся неизменными. Удаление можно отменить через уведомление.",
                      icon: Trash2,
                      tone: "danger",
                      confirmLabel: "Удалить термин",
                    }).then((confirmed) => {
                      if (confirmed) remove.mutate(term);
                    });
                  }}
                >
                  <Trash2 className="size-4" aria-hidden="true" />
                </Button>
              </article>
            ))}
            {terms.length === 0 ? (
              <div className="p-10 text-center">
                <SpellCheck2
                  className="mx-auto size-8 text-[var(--ink-tertiary)]"
                  aria-hidden="true"
                />
                <p className="font-display mt-3 text-2xl">
                  {dictionary.data?.terms.length ? "Ничего не найдено" : "Словарь пока пуст"}
                </p>
                <p className="mt-1 text-sm text-[var(--ink-tertiary)]">
                  {dictionary.data?.terms.length
                    ? "Измените запрос или фильтр."
                    : "Добавьте имена и термины перед следующей записью."}
                </p>
              </div>
            ) : null}
          </div>
        )}
      </div>
    </ProjectUtilityShell>
  );
}

function kindLabel(kind: DictionaryTerm["kind"]): string {
  return {
    character: "персонаж",
    place: "место",
    term: "термин",
    other: "другое",
  }[kind];
}
