import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Settings2 } from "lucide-react";
import { type FormEvent, useEffect, useState } from "react";

import {
  type Project,
  type ProjectWorkspace,
  type UpdateProjectInput,
  updateProject,
} from "~entities/project";
import { Button, Field, Input, ModalDialog, Select, useToast } from "~shared/ui";

interface EditProjectDialogProps {
  project: Project;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

function formFromProject(project: Project): UpdateProjectInput {
  return {
    title: project.title,
    language: project.language,
    synopsis: project.synopsis,
    wordGoal: project.wordGoal,
  };
}

export function EditProjectDialog({ project, open, onOpenChange }: EditProjectDialogProps) {
  const queryClient = useQueryClient();
  const { showToast } = useToast();
  const [form, setForm] = useState<UpdateProjectInput>(() => formFromProject(project));

  useEffect(() => {
    if (open) {
      setForm(formFromProject(project));
    }
  }, [open, project]);

  const mutation = useMutation({
    mutationFn: (input: UpdateProjectInput) => updateProject(project.id, input),
    onSuccess: ({ project: updated }) => {
      queryClient.setQueryData<ProjectWorkspace>(["project", project.id], (current) =>
        current ? { ...current, project: updated } : current,
      );
      queryClient.setQueriesData<{ projects: Project[] }>({ queryKey: ["projects"] }, (current) =>
        current
          ? {
              ...current,
              projects: current.projects.map((item) => (item.id === updated.id ? updated : item)),
            }
          : current,
      );
      void queryClient.invalidateQueries({ queryKey: ["project", project.id] });
      void queryClient.invalidateQueries({ queryKey: ["projects"] });
      onOpenChange(false);
      showToast({ title: "Проект обновлён" });
    },
    onError: (error) =>
      showToast({ title: "Не удалось обновить проект", description: error.message }),
  });

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    mutation.mutate({
      ...form,
      title: form.title.trim(),
      synopsis: form.synopsis.trim(),
    });
  };

  const unchanged =
    form.title.trim() === project.title &&
    form.language === project.language &&
    form.synopsis.trim() === project.synopsis &&
    form.wordGoal === project.wordGoal;

  return (
    <ModalDialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!mutation.isPending) onOpenChange(nextOpen);
      }}
      title="Настройки проекта"
      description="Название и замысел обновятся во всём рабочем пространстве. Тип и структура проекта не изменятся."
      icon={Settings2}
    >
      <form onSubmit={handleSubmit} className="grid gap-4">
        <Field label="Название проекта" htmlFor="edit-project-title">
          <Input
            id="edit-project-title"
            autoFocus
            required
            minLength={1}
            maxLength={120}
            value={form.title}
            onChange={(event) => setForm((current) => ({ ...current, title: event.target.value }))}
          />
        </Field>
        <Field label="Коротко о замысле" htmlFor="edit-project-synopsis">
          <textarea
            id="edit-project-synopsis"
            rows={4}
            value={form.synopsis}
            placeholder="О чём эта история"
            onChange={(event) =>
              setForm((current) => ({ ...current, synopsis: event.target.value }))
            }
            className="w-full resize-y rounded-[var(--radius-sm)] border border-[var(--border-strong)] bg-[var(--surface-raised)] px-3 py-2 text-[15px] leading-6 text-[var(--ink-primary)] placeholder:text-[var(--ink-tertiary)] focus:border-[var(--focus)] focus:outline-2 focus:outline-offset-1 focus:outline-[var(--focus)]"
          />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Язык диктовки" htmlFor="edit-project-language">
            <Select
              id="edit-project-language"
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
          <Field
            label="Цель по словам"
            htmlFor="edit-project-word-goal"
            hint="Оставьте поле пустым, чтобы убрать цель."
          >
            <Input
              id="edit-project-word-goal"
              type="number"
              inputMode="numeric"
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
          </Field>
        </div>
        <div className="mt-2 flex justify-end gap-2">
          <Button
            type="button"
            variant="ghost"
            disabled={mutation.isPending}
            onClick={() => onOpenChange(false)}
          >
            Отмена
          </Button>
          <Button type="submit" disabled={mutation.isPending || unchanged || !form.title.trim()}>
            {mutation.isPending ? "Сохраняем…" : "Сохранить изменения"}
          </Button>
        </div>
      </form>
    </ModalDialog>
  );
}
