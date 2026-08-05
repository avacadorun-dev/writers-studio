import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import {
  ArrowLeft,
  ArrowRight,
  Blocks,
  BookMarked,
  BookOpen,
  Clapperboard,
  FileText,
  Gamepad2,
  Languages,
  Lightbulb,
  Shapes,
  Target,
  Type,
} from "lucide-react";
import { type FormEvent, useState } from "react";

import { type CreateProjectInput, createProject, type ProjectKind } from "~entities/project";
import { APIError } from "~shared/api";
import { Button, Field, Input, Select } from "~shared/ui";
import { RequireSession } from "~widgets/require-session";
import { WorkspaceHeader } from "~widgets/workspace-header";

const projectTypes: Array<{ value: ProjectKind; label: string; icon: typeof BookOpen }> = [
  { value: "novel", label: "Роман", icon: BookOpen },
  { value: "story", label: "Рассказ", icon: FileText },
  { value: "screenplay", label: "Сценарий", icon: Clapperboard },
  { value: "nonfiction", label: "Нон-фикшн", icon: BookMarked },
  { value: "game_design", label: "Гейм-дизайн", icon: Gamepad2 },
  { value: "free", label: "Свободный", icon: Shapes },
  { value: "custom", label: "Своя структура", icon: Blocks },
];

export function CreateProjectPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [form, setForm] = useState<CreateProjectInput>({
    title: "",
    kind: "novel",
    language: "ru",
    synopsis: "",
  });
  const mutation = useMutation({
    mutationFn: createProject,
    onSuccess: (workspace) => {
      void queryClient.invalidateQueries({ queryKey: ["projects"] });
      const firstNode = workspace.nodes[0];
      if (firstNode) {
        void navigate({
          to: "/projects/$projectId/studio/$nodeId",
          params: { projectId: workspace.project.id, nodeId: firstNode.id },
        });
      }
    },
  });

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    mutation.mutate(form);
  };

  return (
    <RequireSession>
      <div className="min-h-full">
        <WorkspaceHeader />
        <main className="mx-auto w-full max-w-[864px] px-4 pt-12 pb-16 sm:px-8">
          <section className="text-center">
            <p className="flex items-center justify-center gap-2 text-[10px] font-semibold tracking-[0.16em] text-[var(--ink-tertiary)] uppercase">
              <span className="size-2 rounded-full bg-[var(--accent)]" aria-hidden="true" />
              <span className="size-2 rounded-full bg-[var(--border-strong)]" aria-hidden="true" />
              Основа проекта
            </p>
            <h1 className="font-display mt-7 text-[40px] leading-[1.15]">
              О чём вы хотите писать?
            </h1>
            <p className="mt-6 text-[14px] leading-6 text-[var(--ink-secondary)]">
              Достаточно рабочего названия. Всё остальное можно изменить позже.
            </p>
          </section>

          <form onSubmit={handleSubmit} className="mt-7">
            <div className="grid gap-6 rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-5 sm:p-7">
              <Field
                label="Рабочее название"
                htmlFor="project-title"
                error={
                  mutation.error instanceof APIError && mutation.error.code === "invalid_project"
                    ? mutation.error.message
                    : undefined
                }
              >
                <div className="relative">
                  <Type
                    className="pointer-events-none absolute top-1/2 left-3 size-[18px] -translate-y-1/2 text-[var(--ink-tertiary)]"
                    aria-hidden="true"
                  />
                  <Input
                    id="project-title"
                    aria-describedby={
                      mutation.error instanceof APIError &&
                      mutation.error.code === "invalid_project"
                        ? "project-title-description"
                        : undefined
                    }
                    className="pl-10"
                    autoFocus
                    required
                    minLength={1}
                    maxLength={120}
                    value={form.title}
                    placeholder="Дом у тихой воды"
                    onChange={(event) =>
                      setForm((current) => ({ ...current, title: event.target.value }))
                    }
                  />
                </div>
              </Field>
              <fieldset className="grid content-start gap-2 text-sm text-[var(--ink-secondary)]">
                <legend className="font-medium text-[var(--ink-primary)]">Тип проекта</legend>
                <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 lg:grid-cols-7">
                  {projectTypes.map((type) => (
                    <button
                      key={type.value}
                      type="button"
                      onClick={() => setForm((current) => ({ ...current, kind: type.value }))}
                      className="flex min-h-[84px] flex-col justify-between rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-panel)] p-3 text-left text-[12px] data-[selected=true]:border-[var(--accent)] data-[selected=true]:bg-[var(--accent-soft)] data-[selected=true]:text-[var(--accent-ink)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus)]"
                      data-selected={form.kind === type.value}
                      aria-pressed={form.kind === type.value}
                    >
                      <type.icon className="size-[18px]" aria-hidden="true" />
                      {type.label}
                    </button>
                  ))}
                </div>
              </fieldset>
              <div className="grid gap-5 sm:grid-cols-2">
                <Field label="Язык диктовки" htmlFor="project-language">
                  <div className="relative">
                    <Languages
                      className="pointer-events-none absolute top-1/2 left-3 z-10 size-[18px] -translate-y-1/2 text-[var(--ink-tertiary)]"
                      aria-hidden="true"
                    />
                    <Select
                      id="project-language"
                      className="pl-10"
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
                  </div>
                </Field>
                <Field label="Цель по словам · необязательно" htmlFor="project-word-goal">
                  <div className="relative">
                    <Target
                      className="pointer-events-none absolute top-1/2 left-3 size-[18px] -translate-y-1/2 text-[var(--ink-tertiary)]"
                      aria-hidden="true"
                    />
                    <Input
                      id="project-word-goal"
                      className="pl-10"
                      type="number"
                      min={1}
                      max={10_000_000}
                      value={form.wordGoal ?? ""}
                      placeholder="Например, 80 000"
                      onChange={(event) =>
                        setForm((current) => ({
                          ...current,
                          wordGoal: event.target.value ? Number(event.target.value) : undefined,
                        }))
                      }
                    />
                  </div>
                </Field>
              </div>
              <Field label="Коротко о замысле · необязательно" htmlFor="project-synopsis">
                <div className="relative">
                  <Lightbulb
                    className="pointer-events-none absolute top-1/2 left-3 size-[18px] -translate-y-1/2 text-[var(--ink-tertiary)]"
                    aria-hidden="true"
                  />
                  <Input
                    id="project-synopsis"
                    className="pl-10"
                    value={form.synopsis}
                    placeholder="История возвращения в родной дом"
                    onChange={(event) =>
                      setForm((current) => ({ ...current, synopsis: event.target.value }))
                    }
                  />
                </div>
              </Field>
            </div>
            <div className="mt-6 flex items-center justify-between gap-3">
              <Button asChild variant="ghost">
                <Link to="/projects">
                  <ArrowLeft className="size-4" aria-hidden="true" />К проектам
                </Link>
              </Button>
              <Button type="submit" disabled={mutation.isPending}>
                <ArrowRight className="size-4" aria-hidden="true" />
                {mutation.isPending ? "Создаём…" : "Создать и открыть"}
              </Button>
            </div>
          </form>
        </main>
      </div>
    </RequireSession>
  );
}
