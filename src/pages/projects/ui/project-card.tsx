import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  Archive,
  ArrowRight,
  BookMarked,
  BookOpen,
  Clapperboard,
  Clock3,
  FileText,
  Gamepad2,
  Layers3,
  Mic,
  MoreHorizontal,
  NotebookPen,
  PencilLine,
  RotateCcw,
  Trash2,
} from "lucide-react";
import { useState } from "react";
import {
  type Project,
  type ProjectStatus,
  purgeProject,
  setProjectStatus,
} from "~entities/project";
import { EditProjectDialog } from "~features/edit-project";
import {
  Button,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  useActionDialog,
  useToast,
} from "~shared/ui";

const kindLabels: Record<Project["kind"], string> = {
  novel: "Роман",
  story: "Рассказ",
  screenplay: "Сценарий",
  nonfiction: "Нон-фикшн",
  game_design: "Гейм-дизайн",
  free: "Свободный проект",
  custom: "Своя структура",
};

const kindIcons = {
  novel: BookOpen,
  story: FileText,
  screenplay: Clapperboard,
  nonfiction: BookMarked,
  game_design: Gamepad2,
  free: NotebookPen,
  custom: Layers3,
};

export function ProjectCard({ project }: { project: Project }) {
  const KindIcon = kindIcons[project.kind];
  const queryClient = useQueryClient();
  const { showToast } = useToast();
  const { requestInput } = useActionDialog();
  const [editing, setEditing] = useState(false);
  const status = useMutation({
    mutationFn: (nextStatus: ProjectStatus) => setProjectStatus(project.id, nextStatus),
    onSuccess: async ({ project: updated }) => {
      await queryClient.invalidateQueries({ queryKey: ["projects"] });
      showToast({
        title:
          updated.status === "active"
            ? "Проект восстановлен"
            : updated.status === "archived"
              ? "Проект перемещён в архив"
              : "Проект перемещён в корзину",
      });
    },
    onError: (error) =>
      showToast({ title: "Не удалось изменить проект", description: error.message }),
  });
  const purge = useMutation({
    mutationFn: (confirmation: string) => purgeProject(project.id, confirmation),
    onSuccess: async ({ notice }) => {
      await queryClient.invalidateQueries({ queryKey: ["projects"] });
      showToast({ title: "Проект удалён", description: notice });
    },
    onError: (error) => showToast({ title: "Удаление не завершено", description: error.message }),
  });

  return (
    <article className="project-card group" data-kind={project.kind}>
      <div className="project-card-top">
        <span className="project-card-symbol" aria-hidden="true">
          <KindIcon className="size-[18px]" />
        </span>
        <span className="project-card-kind">{kindLabels[project.kind]}</span>
        <div className="project-card-menu">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={`Действия: ${project.title}`}
              >
                <MoreHorizontal className="size-4" aria-hidden="true" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" aria-label={`Действия с проектом «${project.title}»`}>
              {project.status !== "trash" ? (
                <DropdownMenuItem onSelect={() => setEditing(true)}>
                  <PencilLine className="size-4" aria-hidden="true" />
                  Редактировать
                </DropdownMenuItem>
              ) : null}
              {project.status !== "active" ? (
                <DropdownMenuItem onSelect={() => status.mutate("active")}>
                  <RotateCcw className="size-4" aria-hidden="true" />
                  Восстановить
                </DropdownMenuItem>
              ) : null}
              {project.status === "active" ? (
                <DropdownMenuItem onSelect={() => status.mutate("archived")}>
                  <Archive className="size-4" aria-hidden="true" />В архив
                </DropdownMenuItem>
              ) : null}
              {project.status !== "trash" ? (
                <DropdownMenuItem destructive onSelect={() => status.mutate("trash")}>
                  <Trash2 className="size-4" aria-hidden="true" />В корзину
                </DropdownMenuItem>
              ) : (
                <DropdownMenuItem
                  destructive
                  onSelect={() => {
                    void requestInput({
                      title: "Удалить проект безвозвратно?",
                      description:
                        "Будут удалены рукопись, версии и связанные записи. Резервные копии внешних систем могут храниться по их правилам.",
                      label: "Название проекта",
                      requiredValue: project.title,
                      icon: Trash2,
                      tone: "danger",
                      confirmLabel: "Удалить безвозвратно",
                    }).then((confirmation) => {
                      if (confirmation !== undefined) {
                        purge.mutate(confirmation);
                      }
                    });
                  }}
                >
                  <Trash2 className="size-4" aria-hidden="true" />
                  Удалить навсегда
                </DropdownMenuItem>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>
      <Link
        to="/projects/$projectId"
        params={{ projectId: project.id }}
        className="project-card-open focus-visible:outline-none"
      >
        <h2>{project.title}</h2>
        <p>{project.synopsis || "Новая рукопись ждёт первого абзаца."}</p>
      </Link>
      <div className="project-card-footer">
        <time
          className="project-card-date"
          dateTime={project.updatedAt}
          title="Последнее изменение"
        >
          <Clock3 className="size-3" aria-hidden="true" />
          {new Intl.DateTimeFormat("ru", { day: "numeric", month: "long" }).format(
            new Date(project.updatedAt),
          )}
        </time>
        <div className="flex items-center gap-1">
          <Button
            asChild
            variant="ghost"
            size="icon"
            className="project-card-dictate sm:hidden"
            aria-label={`Диктовать в проект ${project.title}`}
          >
            <Link
              to="/projects/$projectId"
              params={{ projectId: project.id }}
              search={{ dictate: true }}
            >
              <Mic className="size-4" aria-hidden="true" />
            </Link>
          </Button>
          <ArrowRight
            className="size-4 transition-transform group-hover:translate-x-1"
            aria-hidden="true"
          />
        </div>
      </div>
      <EditProjectDialog project={project} open={editing} onOpenChange={setEditing} />
    </article>
  );
}
