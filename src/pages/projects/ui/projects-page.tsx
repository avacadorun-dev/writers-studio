import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import {
  Archive,
  BookOpen,
  Clock3,
  FileArchive,
  FileUp,
  FolderOpen,
  LayoutGrid,
  LifeBuoy,
  LoaderCircle,
  Plus,
  Search,
  Settings,
  Trash2,
} from "lucide-react";
import { useRef, useState } from "react";

import {
  importMarkdownTree,
  importProjectFile,
  listProjects,
  type MarkdownTreeImportReport,
  type ProjectImportResult,
  type ProjectStatus,
} from "~entities/project";
import { listLocalRecordings } from "~shared/lib";
import { Banner, Button, Field, Input, ModalDialog, Select, Spinner, useToast } from "~shared/ui";
import { RequireSession } from "~widgets/require-session";
import { WorkspaceHeader } from "~widgets/workspace-header";

import { ProjectCard } from "./project-card";
import "./projects.css";

type ProjectFilter = "recent" | "active" | "archived" | "trash";

const filters = [
  { value: "recent", label: "Недавние", icon: Clock3 },
  { value: "active", label: "Все", icon: LayoutGrid },
  { value: "archived", label: "Архив", icon: Archive },
  { value: "trash", label: "Корзина", icon: Trash2 },
] satisfies Array<{ value: ProjectFilter; label: string; icon: typeof Clock3 }>;

function ProjectFilters({
  status,
  onChange,
  presentation,
}: {
  status: ProjectFilter;
  onChange: (value: ProjectFilter) => void;
  presentation: "sidebar" | "tabs";
}) {
  return (
    <fieldset
      className={`library-filters library-filters-${presentation}`}
      aria-label="Фильтр проектов"
    >
      {filters.map(({ value, label, icon: Icon }) => (
        <button
          key={value}
          type="button"
          onClick={() => onChange(value)}
          className="library-filter"
          data-active={status === value}
          aria-pressed={status === value}
        >
          <Icon className="size-[17px]" aria-hidden="true" />
          {label}
        </button>
      ))}
    </fieldset>
  );
}

interface DirectoryManifestEntry {
  id: string;
  path: string;
  kind: "directory" | "file";
  size: number;
  file?: File;
}

interface PickedDirectory {
  rootName: string;
  entries: DirectoryManifestEntry[];
}

interface DirectoryPickerFileHandle {
  kind: "file";
  getFile: () => Promise<File>;
}

interface DirectoryPickerHandle {
  kind: "directory";
  name: string;
  entries: () => AsyncIterableIterator<[string, DirectoryPickerHandle | DirectoryPickerFileHandle]>;
}

async function readDirectoryHandle(root: DirectoryPickerHandle): Promise<PickedDirectory> {
  const entries: DirectoryManifestEntry[] = [];
  const walk = async (directory: DirectoryPickerHandle, relative: string): Promise<void> => {
    for await (const [name, handle] of directory.entries()) {
      const childPath = `${relative}/${name}`;
      if (handle.kind === "directory") {
        entries.push({ id: crypto.randomUUID(), path: childPath, kind: "directory", size: 0 });
        await walk(handle, childPath);
      } else {
        const file = await handle.getFile();
        entries.push({
          id: crypto.randomUUID(),
          path: childPath,
          kind: "file",
          size: file.size,
          file,
        });
      }
    }
  };
  await walk(root, root.name);
  return { rootName: root.name, entries };
}

function webkitDirectory(files: FileList): PickedDirectory {
  const entries = Array.from(files).map((file) => ({
    id: crypto.randomUUID(),
    path: file.webkitRelativePath || file.name,
    kind: "file" as const,
    size: file.size,
    file,
  }));
  const rootName = entries[0]?.path.split("/")[0] ?? "";
  return { rootName, entries };
}

function markdownTreeForm(
  source: "zip" | "directory",
  title: string,
  language: "ru" | "en",
  archive?: File,
  directory?: PickedDirectory,
): FormData {
  const form = new FormData();
  form.set("source", source);
  form.set("title", title.trim());
  form.set("language", language);
  if (source === "zip" && archive) {
    form.set("archive", archive);
  } else if (directory) {
    form.set(
      "manifest",
      JSON.stringify({
        rootName: directory.rootName,
        entries: directory.entries.map(({ id, path, kind, size }) => ({ id, path, kind, size })),
      }),
    );
    for (const entry of directory.entries) {
      if (entry.file) form.set(`entry.${entry.id}`, entry.file, entry.file.name);
    }
  }
  return form;
}

export function ProjectsPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { showToast } = useToast();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const zipInputRef = useRef<HTMLInputElement>(null);
  const directoryInputRef = useRef<HTMLInputElement>(null);
  const [status, setStatus] = useState<ProjectFilter>("recent");
  const [searchQuery, setSearchQuery] = useState("");
  const [markdownDialogOpen, setMarkdownDialogOpen] = useState(false);
  const [markdownSource, setMarkdownSource] = useState<"zip" | "directory">("zip");
  const [markdownTitle, setMarkdownTitle] = useState("");
  const [markdownLanguage, setMarkdownLanguage] = useState<"ru" | "en">("ru");
  const [markdownArchive, setMarkdownArchive] = useState<File>();
  const [markdownDirectory, setMarkdownDirectory] = useState<PickedDirectory>();
  const [directoryFallbackNotice, setDirectoryFallbackNotice] = useState(false);
  const [importReport, setImportReport] = useState<MarkdownTreeImportReport>();
  const [importedMarkdownProject, setImportedMarkdownProject] = useState<ProjectImportResult>();
  const [markdownProgress, setMarkdownProgress] = useState<{
    phase: "upload" | "processing";
    percent?: number;
  }>();
  const projects = useQuery({
    queryKey: ["projects", status === "recent" ? "active" : status],
    queryFn: () => listProjects((status === "recent" ? "active" : status) as ProjectStatus),
  });
  const recovery = useQuery({
    queryKey: ["recovery-local"],
    queryFn: listLocalRecordings,
    networkMode: "always",
  });
  const importProject = useMutation({
    mutationFn: importProjectFile,
    onSuccess: async ({ project, notice }) => {
      await queryClient.invalidateQueries({ queryKey: ["projects"] });
      showToast({ title: `Импортирован проект «${project.title}»`, description: notice });
      if (project.firstNodeId) {
        await navigate({
          to: "/projects/$projectId/studio/$nodeId",
          params: { projectId: project.projectId, nodeId: project.firstNodeId },
        });
        return;
      }
      await navigate({ to: "/projects/$projectId", params: { projectId: project.projectId } });
    },
  });
  const importMarkdown = useMutation({
    mutationFn: () =>
      importMarkdownTree(
        markdownTreeForm(
          markdownSource,
          markdownTitle,
          markdownLanguage,
          markdownArchive,
          markdownDirectory,
        ),
        setMarkdownProgress,
      ),
    onSuccess: async ({ project, report }) => {
      // Older servers could encode an empty Go slice as null. Normalizing here
      // keeps the report UI usable while a mixed-version deployment rolls out.
      const normalizedReport = { ...report, warnings: report.warnings ?? [] };
      setImportReport(normalizedReport);
      setImportedMarkdownProject(project);
      setMarkdownDialogOpen(false);
      setMarkdownProgress(undefined);
      await queryClient.invalidateQueries({ queryKey: ["projects"] });
      showToast({
        title: `Импортирован проект «${project.title}»`,
        description: `${report.documents} документов, ${report.directories} папок, ${report.images} изображений`,
      });
    },
  });
  const chooseDirectory = async () => {
    const picker = (
      window as Window & {
        showDirectoryPicker?: () => Promise<DirectoryPickerHandle>;
      }
    ).showDirectoryPicker;
    if (!picker) {
      setDirectoryFallbackNotice(true);
      directoryInputRef.current?.click();
      return;
    }
    try {
      const picked = await readDirectoryHandle(await picker());
      setMarkdownDirectory(picked);
      if (!markdownTitle) setMarkdownTitle(picked.rootName);
      setDirectoryFallbackNotice(false);
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") return;
      setDirectoryFallbackNotice(true);
      directoryInputRef.current?.click();
    }
  };
  const normalizedSearch = searchQuery.trim().toLocaleLowerCase("ru");
  const matchingProjects = projects.data?.projects.filter(
    (project) =>
      !normalizedSearch ||
      `${project.title} ${project.synopsis ?? ""}`
        .toLocaleLowerCase("ru")
        .includes(normalizedSearch),
  );
  const visibleProjects =
    status === "recent" && !normalizedSearch ? matchingProjects?.slice(0, 6) : matchingProjects;
  const hasRecovery = recovery.data?.some((recording) => recording.status !== "uploaded");

  return (
    <RequireSession>
      <div className="projects-workspace library-workspace">
        <WorkspaceHeader />
        <div className="library-shell">
          <aside className="library-sidebar" aria-label="Библиотека проектов">
            <div className="library-sidebar-title">
              <BookOpen className="size-4" aria-hidden="true" />
              Библиотека
            </div>
            <ProjectFilters status={status} onChange={setStatus} presentation="sidebar" />
            <div className="library-sidebar-divider" />
            <nav className="library-utilities" aria-label="Инструменты студии">
              <Link to="/recovery">
                <LifeBuoy className="size-[17px]" aria-hidden="true" />
                Восстановление
              </Link>
              <Link to="/settings">
                <Settings className="size-[17px]" aria-hidden="true" />
                Настройки
              </Link>
            </nav>
            <a href="/docs" className="library-sidebar-help">
              <BookOpen className="size-4" aria-hidden="true" />
              Руководство по студии
            </a>
          </aside>
          <section className="library-pane" aria-label="Библиотека">
            <div className="library-toolbar">
              <div className="library-heading">
                <h1>Ваши проекты</h1>
                <p>
                  {status === "recent"
                    ? "Недавно открытые и обновлённые"
                    : status === "archived"
                      ? "Завершённые и отложенные"
                      : status === "trash"
                        ? "Удалённые проекты"
                        : "Все тексты в одном месте"}
                </p>
              </div>
              <Button asChild className="library-new-button">
                <Link to="/projects/new">
                  <Plus className="size-[18px]" aria-hidden="true" />
                  Новый проект
                </Link>
              </Button>
            </div>
            <div className="library-controls">
              <label className="library-search">
                <Search className="size-[17px]" aria-hidden="true" />
                <input
                  type="search"
                  aria-label="Поиск проектов"
                  placeholder="Найти проект…"
                  value={searchQuery}
                  onChange={(event) => setSearchQuery(event.target.value)}
                />
              </label>
              <div className="library-imports">
                <input
                  ref={fileInputRef}
                  type="file"
                  accept=".wstudio,application/vnd.writers-studio.project+zip"
                  className="sr-only"
                  aria-label="Выбрать файл проекта Writers Studio"
                  onChange={(event) => {
                    const file = event.currentTarget.files?.[0];
                    event.currentTarget.value = "";
                    if (file) importProject.mutate(file);
                  }}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="compact"
                  aria-label="Проект Writers Studio"
                  disabled={importProject.isPending}
                  onClick={() => fileInputRef.current?.click()}
                >
                  {importProject.isPending ? (
                    <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
                  ) : (
                    <FileUp className="size-4" aria-hidden="true" />
                  )}
                  <span>{importProject.isPending ? "Импортируем…" : "Проект Writers Studio"}</span>
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="compact"
                  aria-label="Markdown / Obsidian"
                  onClick={() => setMarkdownDialogOpen(true)}
                >
                  <FolderOpen className="size-4" aria-hidden="true" />
                  <span>Markdown / Obsidian</span>
                </Button>
              </div>
            </div>
            <ProjectFilters status={status} onChange={setStatus} presentation="tabs" />
            {/* biome-ignore lint/a11y/noNoninteractiveTabindex: The scrollable project region must be reachable for keyboard scrolling. */}
            <main className="library-scroll" aria-label="Проекты" tabIndex={0}>
              {importProject.isError ? (
                <Banner
                  className="mt-6"
                  variant="danger"
                  title="Проект не импортирован"
                  body={importProject.error.message}
                />
              ) : null}
              {importReport ? (
                <Banner
                  className="mt-6"
                  variant={importReport.warningsCount > 0 ? "warning" : "success"}
                  title="Markdown-хранилище импортировано"
                  body={`${importReport.documents} документов · ${importReport.directories} папок · ${importReport.images} изображений · ${importReport.resolvedLinks} ссылок. Предупреждений: ${importReport.warningsCount}.`}
                  action={
                    importedMarkdownProject ? (
                      <Button
                        type="button"
                        variant="secondary"
                        onClick={() =>
                          void navigate({
                            to: "/projects/$projectId/studio/$nodeId",
                            params: {
                              projectId: importedMarkdownProject.projectId,
                              nodeId: importedMarkdownProject.firstNodeId,
                            },
                          })
                        }
                      >
                        Открыть проект
                      </Button>
                    ) : undefined
                  }
                />
              ) : null}
              {importReport?.warnings.length ? (
                <details className="mt-3 rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-panel)] p-4 text-sm">
                  <summary className="cursor-pointer font-medium">
                    Показать предупреждения ({importReport.warningsCount})
                  </summary>
                  <ul className="mt-3 grid gap-2 text-xs text-[var(--ink-secondary)]">
                    {importReport.warnings.map((warning) => (
                      <li key={`${warning.path}-${warning.code}-${warning.message}`}>
                        <span className="font-medium text-[var(--ink-primary)]">
                          {warning.path || "Хранилище"}:
                        </span>{" "}
                        {warning.message}
                      </li>
                    ))}
                  </ul>
                  {importReport.warningsCount > importReport.warnings.length ? (
                    <p className="mt-3 text-xs text-[var(--ink-tertiary)]">
                      Показаны первые {importReport.warnings.length} записей.
                    </p>
                  ) : null}
                </details>
              ) : null}

              {(status === "active" || status === "recent") && hasRecovery ? (
                <Banner
                  className="mt-6"
                  variant="info"
                  title="Незавершённые записи всегда можно восстановить"
                  body="Центр восстановления проверяет локальные аудиофрагменты при каждом запуске."
                  action={
                    <Button asChild variant="ghost" size="compact">
                      <Link to="/recovery">
                        <LifeBuoy className="size-4" aria-hidden="true" />
                        Восстановить
                      </Link>
                    </Button>
                  }
                />
              ) : null}

              {projects.isPending ? (
                <Spinner label="Открываем проекты…" />
              ) : projects.isError ? (
                <div className="mt-10 rounded-[var(--radius-lg)] border border-[var(--danger)]/30 bg-[var(--danger-soft)] p-6">
                  <h2 className="font-medium text-[var(--danger)]">Не удалось загрузить проекты</h2>
                  <Button
                    className="mt-4"
                    variant="secondary"
                    onClick={() => void projects.refetch()}
                  >
                    Повторить
                  </Button>
                </div>
              ) : visibleProjects?.length === 0 &&
                (normalizedSearch || (status !== "active" && status !== "recent")) ? (
                <section className="mt-10 grid min-h-72 place-items-center rounded-[var(--radius-lg)] border border-dashed border-[var(--border-strong)] bg-[var(--surface-panel)] p-8 text-center">
                  <div>
                    <h2 className="font-display text-3xl">
                      {normalizedSearch ? "Проекты не найдены" : "Здесь пока пусто"}
                    </h2>
                    <p className="mt-3 text-sm text-[var(--ink-secondary)]">
                      {normalizedSearch
                        ? "Попробуйте другое название или описание."
                        : "Проекты появятся здесь после соответствующего действия."}
                    </p>
                    {status === "active" ? (
                      <Button asChild className="mt-5">
                        <Link to="/projects/new">Создать проект</Link>
                      </Button>
                    ) : null}
                  </div>
                </section>
              ) : (
                <section className="library-grid" aria-label="Список проектов">
                  <Link to="/projects/new" className="project-create-card library-create-card">
                    <span className="library-create-icon">
                      <Plus className="size-6" aria-hidden="true" />
                    </span>
                    <span className="library-create-title">Новый проект</span>
                    <span className="library-create-description">
                      Выберите название и структуру
                    </span>
                    <span className="library-create-arrow" aria-hidden="true">
                      ↗
                    </span>
                  </Link>
                  {visibleProjects?.map((project) => (
                    <ProjectCard key={project.id} project={project} />
                  ))}
                </section>
              )}
            </main>
          </section>
        </div>
        <input
          ref={zipInputRef}
          className="hidden"
          aria-label="Выбрать ZIP-архив Markdown"
          type="file"
          accept=".zip,application/zip"
          onChange={(event) => {
            const file = event.currentTarget.files?.[0];
            if (!file) return;
            setMarkdownArchive(file);
            if (!markdownTitle) setMarkdownTitle(file.name.replace(/\.zip$/iu, ""));
          }}
        />
        <input
          ref={directoryInputRef}
          className="hidden"
          aria-label="Выбрать папку Markdown"
          type="file"
          multiple
          {...({ webkitdirectory: "" } as Record<string, string>)}
          onChange={(event) => {
            if (!event.currentTarget.files?.length) return;
            const picked = webkitDirectory(event.currentTarget.files);
            setMarkdownDirectory(picked);
            if (!markdownTitle) setMarkdownTitle(picked.rootName);
            setDirectoryFallbackNotice(true);
          }}
        />
        <ModalDialog
          open={markdownDialogOpen}
          onOpenChange={setMarkdownDialogOpen}
          title="Импорт Markdown / Obsidian"
          description="Создадим проект free и сохраним папки, Markdown, внутренние ссылки, frontmatter и локальные изображения."
          icon={FileArchive}
          footer={
            <>
              <Button type="button" variant="ghost" onClick={() => setMarkdownDialogOpen(false)}>
                Отмена
              </Button>
              <Button
                type="button"
                disabled={
                  importMarkdown.isPending ||
                  (markdownSource === "zip" ? !markdownArchive : !markdownDirectory)
                }
                onClick={() => importMarkdown.mutate()}
              >
                {importMarkdown.isPending ? (
                  <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
                ) : (
                  <FileUp className="size-4" aria-hidden="true" />
                )}
                {importMarkdown.isPending
                  ? markdownProgress?.phase === "processing"
                    ? "Обрабатываем хранилище…"
                    : `Загружаем${markdownProgress?.percent === undefined ? "…" : ` ${markdownProgress.percent}%…`}`
                  : "Создать проект"}
              </Button>
            </>
          }
        >
          <div className="grid gap-4">
            <Field label="Источник">
              <Select
                value={markdownSource}
                onChange={(event) => setMarkdownSource(event.target.value as "zip" | "directory")}
              >
                <option value="zip">ZIP-архив</option>
                <option value="directory">Локальная папка</option>
              </Select>
            </Field>
            {markdownSource === "zip" ? (
              <Button
                type="button"
                variant="secondary"
                onClick={() => zipInputRef.current?.click()}
              >
                <FileArchive className="size-4" />
                {markdownArchive?.name ?? "Выбрать ZIP"}
              </Button>
            ) : (
              <>
                <Button type="button" variant="secondary" onClick={() => void chooseDirectory()}>
                  <FolderOpen className="size-4" />
                  {markdownDirectory?.rootName ?? "Выбрать папку vault"}
                </Button>
                {directoryFallbackNotice ? (
                  <Banner
                    variant="warning"
                    title="Режим совместимости браузера"
                    body="Браузер передаёт файлы, но не сообщает о полностью пустых папках."
                  />
                ) : null}
              </>
            )}
            <Field label="Название проекта">
              <Input
                value={markdownTitle}
                maxLength={120}
                placeholder="Будет взято из корневой папки"
                onChange={(event) => setMarkdownTitle(event.target.value)}
              />
            </Field>
            <Field label="Язык">
              <Select
                value={markdownLanguage}
                onChange={(event) => setMarkdownLanguage(event.target.value as "ru" | "en")}
              >
                <option value="ru">Русский</option>
                <option value="en">English</option>
              </Select>
            </Field>
            {importMarkdown.isError ? (
              <Banner
                variant="danger"
                title="Markdown-хранилище не импортировано"
                body={importMarkdown.error.message}
              />
            ) : null}
          </div>
        </ModalDialog>
      </div>
    </RequireSession>
  );
}
