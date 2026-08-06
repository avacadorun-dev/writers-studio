import {
  closestCenter,
  DndContext,
  type DragEndEvent,
  KeyboardSensor,
  PointerSensor,
  TouchSensor,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CheckCheck,
  ChevronLeft,
  ChevronRight,
  Download,
  FileArchive,
  FileText,
  FolderOpen,
  GripVertical,
  ImagePlus,
  LoaderCircle,
  RotateCcw,
} from "lucide-react";
import { useEffect, useId, useMemo, useRef, useState } from "react";

import {
  createExport,
  createMarkdownTreeExport,
  type ExportFormat,
  type ExportProfile,
  getExport,
  getExportProfile,
  getNode,
  getProjectOutline,
  listProjectMedia,
  saveExportProfile,
  uploadProjectImage,
} from "~entities/project";
import { Banner, Button, Checkbox, Field, Input, Select, Spinner } from "~shared/ui";
import { ProjectUtilityShell } from "~widgets/project-utility-shell";

import {
  applyExportPreset,
  buildExportQueue,
  buildPublicationPreviewPages,
  type ExportQueueItem,
  reorderQueue,
} from "../model/export-model";

interface ExportInitialOptions {
  format?: ExportFormat;
  nodeIds?: string[];
  chapterTitles?: boolean;
}

export function ExportPage({
  projectId,
  initialOptions,
}: {
  projectId: string;
  initialOptions?: ExportInitialOptions;
}) {
  const queryClient = useQueryClient();
  const profileQuery = useQuery({
    queryKey: ["export-profile", projectId],
    queryFn: () => getExportProfile(projectId),
  });
  const project = useQuery({
    queryKey: ["project-outline", projectId],
    queryFn: () => getProjectOutline(projectId),
  });
  const media = useQuery({
    queryKey: ["project-media", projectId],
    queryFn: () => listProjectMedia(projectId),
  });
  const [profile, setProfile] = useState<ExportProfile>();
  const [profileRevision, setProfileRevision] = useState(1);
  const [queueIds, setQueueIds] = useState<string[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [jobId, setJobId] = useState<string>();
  const [markdownTreeJobId, setMarkdownTreeJobId] = useState<string>();
  const [queueScrollTop, setQueueScrollTop] = useState(0);
  const lastSaved = useRef("");
  const initialized = useRef(false);
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 180, tolerance: 8 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  const canonicalQueue = useMemo(
    () =>
      project.data ? buildExportQueue(project.data.nodes, project.data.project.structure) : [],
    [project.data],
  );
  const queueById = useMemo(
    () => new Map(canonicalQueue.map((item) => [item.id, item])),
    [canonicalQueue],
  );

  useEffect(() => {
    if (!profileQuery.data || !project.data || initialized.current) return;
    initialized.current = true;
    const loaded = {
      ...profileQuery.data.profile,
      ...(initialOptions?.format ? { lastFormat: initialOptions.format } : {}),
      ...(initialOptions?.chapterTitles === undefined
        ? {}
        : { chapterTitles: initialOptions.chapterTitles }),
    };
    setProfile(loaded);
    setProfileRevision(profileQuery.data.revision);
    lastSaved.current = JSON.stringify(profileQuery.data.profile);
    const defaultIds = canonicalQueue.map((item) => item.id);
    const prepared = initialOptions?.nodeIds?.filter((id) => queueById.has(id));
    setQueueIds(
      prepared ? [...prepared, ...defaultIds.filter((id) => !prepared.includes(id))] : defaultIds,
    );
    setSelected(
      new Set(prepared ?? canonicalQueue.filter((item) => !item.note).map((item) => item.id)),
    );
  }, [canonicalQueue, initialOptions, profileQuery.data, project.data, queueById]);

  const saveProfile = useMutation({
    mutationFn: (input: { baseRevision: number; profile: ExportProfile }) =>
      saveExportProfile(projectId, input),
    onSuccess: (response) => {
      setProfileRevision(response.revision);
      lastSaved.current = JSON.stringify(response.profile);
      queryClient.setQueryData(["export-profile", projectId], response);
    },
    onError: () => void queryClient.invalidateQueries({ queryKey: ["export-profile", projectId] }),
  });

  useEffect(() => {
    if (!profile || !initialized.current || JSON.stringify(profile) === lastSaved.current) return;
    const timer = window.setTimeout(() => {
      if (!saveProfile.isPending) saveProfile.mutate({ baseRevision: profileRevision, profile });
    }, 700);
    return () => window.clearTimeout(timer);
  }, [profile, profileRevision, saveProfile.isPending, saveProfile.mutate]);

  const orderedSelectedIds = queueIds.filter((id) => {
    const item = queueById.get(id);
    return selected.has(id) && item && (profile?.includeNotes || !item.note);
  });
  const create = useMutation({
    mutationFn: () => {
      if (!profile) throw new Error("export profile is unavailable");
      return createExport(projectId, {
        format: profile.lastFormat,
        nodeIds: orderedSelectedIds,
        titlePage: profile.titlePage,
        chapterTitles: profile.chapterTitles,
        includeNotes: profile.includeNotes,
        profile,
      });
    },
    onSuccess: (response) => setJobId(response.export.id),
  });
  const job = useQuery({
    queryKey: ["export", jobId],
    queryFn: () => getExport(jobId ?? ""),
    enabled: Boolean(jobId),
    refetchInterval: (query) =>
      ["queued", "running"].includes(query.state.data?.export.status ?? "") ? 1_000 : false,
  });
  const createMarkdownTree = useMutation({
    mutationFn: () => createMarkdownTreeExport(projectId),
    onSuccess: (response) => setMarkdownTreeJobId(response.export.id),
  });
  const markdownTreeJob = useQuery({
    queryKey: ["export", markdownTreeJobId],
    queryFn: () => getExport(markdownTreeJobId ?? ""),
    enabled: Boolean(markdownTreeJobId),
    refetchInterval: (query) =>
      ["queued", "running"].includes(query.state.data?.export.status ?? "") ? 1_000 : false,
  });
  const markdownTreeWorking =
    Boolean(markdownTreeJobId) &&
    (markdownTreeJob.isPending ||
      ["queued", "running"].includes(markdownTreeJob.data?.export.status ?? ""));
  const uploadCover = useMutation({
    mutationFn: (file: File) => uploadProjectImage(projectId, file),
    onSuccess: async ({ media: uploaded }) => {
      await queryClient.invalidateQueries({ queryKey: ["project-media", projectId] });
      setProfile((current) =>
        current
          ? { ...current, metadata: { ...current.metadata, coverAssetId: uploaded.id } }
          : current,
      );
    },
  });

  if (project.isPending || profileQuery.isPending || !project.data || !profile) {
    return <Spinner label="Готовим экспорт…" />;
  }

  const visibleQueue = queueIds
    .map((id) => queueById.get(id))
    .filter((item): item is ExportQueueItem =>
      Boolean(item && (profile.includeNotes || !item.note)),
    );
  const exportRowHeight = 52;
  const exportViewportHeight = 560;
  const virtualStart = Math.max(0, Math.floor(queueScrollTop / exportRowHeight) - 8);
  const virtualEnd = Math.min(
    visibleQueue.length,
    Math.ceil((queueScrollTop + exportViewportHeight) / exportRowHeight) + 8,
  );
  const virtualQueue = visibleQueue.slice(virtualStart, virtualEnd);
  const imageMedia = media.data?.media.filter((item) => item.mediaType.startsWith("image/")) ?? [];
  const cover = imageMedia.find((item) => item.id === profile.metadata.coverAssetId);
  const previewDocuments = orderedSelectedIds
    .map((id) => project.data.nodes.find((node) => node.id === id))
    .filter((node): node is NonNullable<typeof node> => Boolean(node));

  const updateProfile = (patch: Partial<ExportProfile>) =>
    setProfile((current) => (current ? { ...current, ...patch } : current));
  const updateStyle = (patch: Partial<ExportProfile["style"]>) =>
    setProfile((current) =>
      current ? { ...current, style: { ...current.style, ...patch, preset: "custom" } } : current,
    );
  const toggleNotes = (includeNotes: boolean) => {
    updateProfile({ includeNotes });
    setSelected((current) => {
      const next = new Set(current);
      for (const item of canonicalQueue.filter((candidate) => candidate.note)) {
        if (includeNotes) next.add(item.id);
        else next.delete(item.id);
      }
      return next;
    });
  };

  return (
    <ProjectUtilityShell project={project.data.project} nodes={project.data.nodes}>
      <div className="mx-auto w-full max-w-[1500px] px-5 py-7 lg:px-8">
        <header className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p className="text-[10px] font-semibold tracking-[0.14em] text-[var(--ink-tertiary)] uppercase">
              Сборка рукописи
            </p>
            <h1 className="font-display mt-2 text-[34px] leading-tight">Экспорт</h1>
            <p className="mt-1 text-xs text-[var(--ink-tertiary)]">
              {saveProfile.isPending
                ? "Сохраняем профиль…"
                : saveProfile.isError
                  ? "Конфликт сохранения профиля — обновите страницу"
                  : "Профиль оформления сохранён в проекте"}
            </p>
          </div>
          <Button
            type="button"
            disabled={orderedSelectedIds.length === 0 || create.isPending}
            onClick={() => create.mutate()}
          >
            <Download className="size-4" aria-hidden="true" />
            {create.isPending ? "Собираем…" : `Собрать ${profile.lastFormat.toUpperCase()}`}
          </Button>
        </header>

        {initialOptions ? (
          <Banner
            className="mt-5"
            title="Параметры подготовлены ассистентом"
            body="Проверьте порядок, оформление и состав. Проект не изменится до подтверждения других действий."
          />
        ) : null}

        <section className="mt-5 flex flex-col gap-4 rounded-[var(--radius-lg)] border border-[var(--accent)]/30 bg-[var(--accent-soft)] p-5 sm:flex-row sm:items-center">
          <FileArchive className="size-6 shrink-0 text-[var(--accent)]" aria-hidden="true" />
          <div className="min-w-0 flex-1">
            <h2 className="text-sm font-semibold">Полная резервная копия Writers Studio</h2>
            <p className="mt-1 text-xs text-[var(--ink-secondary)]">
              Структура, тексты, версии, медиа и профиль экспорта в переносимом файле проекта.
            </p>
          </div>
          <Button asChild variant="secondary">
            <a href={`/api/projects/${projectId}/transfer`} download>
              <Download className="size-4" aria-hidden="true" /> Скачать .wstudio
            </a>
          </Button>
        </section>

        <section className="mt-3 flex flex-col gap-4 rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-panel)] p-5 sm:flex-row sm:items-center">
          <FolderOpen className="size-6 shrink-0 text-[var(--accent)]" aria-hidden="true" />
          <div className="min-w-0 flex-1">
            <h2 className="text-sm font-semibold">Markdown-дерево для Obsidian</h2>
            <p className="mt-1 text-xs text-[var(--ink-secondary)]">
              Все активные документы, вложенные папки, frontmatter, переносимые ссылки и изображения
              в ZIP.
            </p>
          </div>
          {markdownTreeJob.data?.export.status === "ready" &&
          markdownTreeJob.data.export.downloadUrl ? (
            <Button asChild variant="secondary">
              <a href={markdownTreeJob.data.export.downloadUrl} download>
                <Download className="size-4" /> Скачать ZIP
              </a>
            </Button>
          ) : (
            <Button
              type="button"
              variant="secondary"
              disabled={createMarkdownTree.isPending || markdownTreeWorking}
              onClick={() => createMarkdownTree.mutate()}
            >
              {createMarkdownTree.isPending || markdownTreeWorking ? (
                <LoaderCircle className="size-4 animate-spin" />
              ) : (
                <Download className="size-4" />
              )}
              {markdownTreeWorking ? "Собираем ZIP…" : "Экспортировать дерево"}
            </Button>
          )}
        </section>
        {createMarkdownTree.isError || markdownTreeJob.data?.export.status === "failed" ? (
          <Banner
            className="mt-3"
            variant="danger"
            title="Markdown-дерево не экспортировано"
            body={createMarkdownTree.error?.message ?? "Не удалось собрать ZIP."}
          />
        ) : null}

        <div className="mt-5 grid gap-5 xl:grid-cols-[360px_minmax(380px,1fr)_minmax(340px,0.9fr)]">
          <aside className="h-fit rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-panel)] p-5">
            <h2 className="font-display text-xl">Оформление</h2>
            <div className="mt-4 grid gap-4">
              <Field label="Формат">
                <Select
                  value={profile.lastFormat}
                  onChange={(event) =>
                    updateProfile({ lastFormat: event.target.value as ExportFormat })
                  }
                >
                  <option value="docx">DOCX · редактор</option>
                  <option value="epub">EPUB · электронная книга</option>
                  <option value="pdf">PDF · готово к чтению</option>
                  <option value="markdown">Markdown</option>
                  <option value="txt">TXT</option>
                </Select>
              </Field>
              <Field label="Пресет">
                <Select
                  value={profile.style.preset}
                  onChange={(event) => {
                    const preset = event.target.value as ExportProfile["style"]["preset"];
                    if (preset !== "custom") setProfile(applyExportPreset(profile, preset));
                  }}
                >
                  <option value="book">Книга</option>
                  <option value="modern">Современный</option>
                  <option value="manuscript">Рукопись</option>
                  {profile.style.preset === "custom" ? <option value="custom">Свой</option> : null}
                </Select>
              </Field>
              <Field label="Шрифт">
                <Select
                  value={profile.style.fontFamily}
                  onChange={(event) =>
                    updateStyle({
                      fontFamily: event.target.value as ExportProfile["style"]["fontFamily"],
                    })
                  }
                >
                  <option value="literata">Literata</option>
                  <option value="source-serif">Source Serif 4</option>
                  <option value="inter">Inter</option>
                  <option value="ibm-plex-mono">IBM Plex Mono</option>
                </Select>
              </Field>
              <div className="grid grid-cols-2 gap-3">
                <Field label="Кегль">
                  <Select
                    value={profile.style.fontSizePt}
                    onChange={(event) => updateStyle({ fontSizePt: Number(event.target.value) })}
                  >
                    {[10, 11, 12, 13, 14, 15, 16].map((value) => (
                      <option key={value} value={value}>
                        {value} pt
                      </option>
                    ))}
                  </Select>
                </Field>
                <Field label="Интервал">
                  <Select
                    value={profile.style.lineHeight}
                    onChange={(event) => updateStyle({ lineHeight: Number(event.target.value) })}
                  >
                    {[1.2, 1.4, 1.5, 1.55, 1.6, 1.8, 2].map((value) => (
                      <option key={value} value={value}>
                        {String(value).replace(".", ",")}
                      </option>
                    ))}
                  </Select>
                </Field>
              </div>
              <Field label="Абзацы">
                <Select
                  value={profile.style.paragraphMode}
                  onChange={(event) =>
                    updateStyle({ paragraphMode: event.target.value as "indent" | "spaced" })
                  }
                >
                  <option value="indent">Красная строка</option>
                  <option value="spaced">Интервалы между абзацами</option>
                </Select>
              </Field>
              {profile.style.paragraphMode === "indent" ? (
                <Field label="Отступ первой строки">
                  <Select
                    value={profile.style.firstLineIndentEm}
                    onChange={(event) =>
                      updateStyle({ firstLineIndentEm: Number(event.target.value) })
                    }
                  >
                    {[0.5, 1, 1.25, 1.5, 2].map((value) => (
                      <option key={value} value={value}>
                        {String(value).replace(".", ",")} em
                      </option>
                    ))}
                  </Select>
                </Field>
              ) : (
                <Field label="Интервал после абзаца">
                  <Select
                    value={profile.style.paragraphSpaceEm}
                    onChange={(event) =>
                      updateStyle({ paragraphSpaceEm: Number(event.target.value) })
                    }
                  >
                    {[0.25, 0.5, 0.75, 1, 1.5].map((value) => (
                      <option key={value} value={value}>
                        {String(value).replace(".", ",")} em
                      </option>
                    ))}
                  </Select>
                </Field>
              )}
              <Field label="Выравнивание">
                <Select
                  value={profile.style.textAlign}
                  onChange={(event) =>
                    updateStyle({ textAlign: event.target.value as "left" | "justify" })
                  }
                >
                  <option value="left">По левому краю</option>
                  <option value="justify">По ширине</option>
                </Select>
              </Field>
              <Field
                label="Разделитель сцен"
                hint="Показывается между документами, если разрыв страницы выключен"
              >
                <Select
                  value={profile.style.sceneSeparator}
                  onChange={(event) =>
                    updateStyle({
                      sceneSeparator: event.target
                        .value as ExportProfile["style"]["sceneSeparator"],
                    })
                  }
                >
                  <option value="asterisks">* * *</option>
                  <option value="dots">· · ·</option>
                  <option value="line">—</option>
                </Select>
              </Field>
              <div className="grid grid-cols-2 gap-3">
                <Field
                  label="Страница"
                  hint={profile.lastFormat === "epub" ? "EPUB адаптируется к экрану" : undefined}
                >
                  <Select
                    disabled={profile.lastFormat === "epub"}
                    value={profile.style.pageSize}
                    onChange={(event) =>
                      updateStyle({ pageSize: event.target.value as "a4" | "a5" | "letter" })
                    }
                  >
                    <option value="a4">A4</option>
                    <option value="a5">A5</option>
                    <option value="letter">Letter</option>
                  </Select>
                </Field>
                <Field label="Поля">
                  <Select
                    disabled={profile.lastFormat === "epub"}
                    value={profile.style.marginMm}
                    onChange={(event) => updateStyle({ marginMm: Number(event.target.value) })}
                  >
                    {[15, 20, 25, 30].map((value) => (
                      <option key={value} value={value}>
                        {value} мм
                      </option>
                    ))}
                  </Select>
                </Field>
              </div>
              <div className="grid gap-3 border-t border-[var(--border-subtle)] pt-4 text-sm">
                <CheckOption
                  checked={profile.titlePage}
                  onChange={(titlePage) => updateProfile({ titlePage })}
                  label="Титульная страница"
                />
                <CheckOption
                  checked={profile.tableOfContents}
                  onChange={(tableOfContents) => updateProfile({ tableOfContents })}
                  label="Оглавление"
                />
                <CheckOption
                  checked={profile.chapterTitles}
                  onChange={(chapterTitles) => updateProfile({ chapterTitles })}
                  label="Заголовки документов"
                />
                <CheckOption
                  checked={profile.chapterPageBreak}
                  onChange={(chapterPageBreak) => updateProfile({ chapterPageBreak })}
                  label="Новый документ с новой страницы"
                />
                <CheckOption
                  checked={profile.includeNotes}
                  onChange={toggleNotes}
                  label="Включить заметки"
                />
              </div>
            </div>
          </aside>

          <section className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-5">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="font-display mr-auto text-2xl">Что войдёт в файл</h2>
              <Button
                variant="ghost"
                size="compact"
                onClick={() => setSelected(new Set(visibleQueue.map((item) => item.id)))}
              >
                <CheckCheck className="size-4" /> Все
              </Button>
              <Button
                variant="ghost"
                size="compact"
                onClick={() => {
                  setQueueIds(canonicalQueue.map((item) => item.id));
                  setSelected(
                    new Set(
                      canonicalQueue
                        .filter((item) => profile.includeNotes || !item.note)
                        .map((item) => item.id),
                    ),
                  );
                }}
              >
                <RotateCcw className="size-4" /> Сбросить
              </Button>
            </div>
            <p className="mt-2 text-xs text-[var(--ink-tertiary)]">
              Порядок ниже относится только к этому экспортному файлу.
            </p>
            <DndContext
              sensors={sensors}
              collisionDetection={closestCenter}
              onDragEnd={(event: DragEndEvent) => {
                if (event.over)
                  setQueueIds((ids) =>
                    reorderQueue(ids, String(event.active.id), String(event.over?.id)),
                  );
              }}
            >
              <SortableContext
                items={virtualQueue.map((item) => item.id)}
                strategy={verticalListSortingStrategy}
              >
                <div
                  className="mt-4 overflow-y-auto"
                  style={{
                    height: Math.min(exportViewportHeight, visibleQueue.length * exportRowHeight),
                  }}
                  onScroll={(event) => setQueueScrollTop(event.currentTarget.scrollTop)}
                  data-testid="virtualized-export-queue"
                >
                  <div style={{ height: virtualStart * exportRowHeight }} aria-hidden="true" />
                  <div className="grid gap-1">
                    {virtualQueue.map((item) => (
                      <SortableExportRow
                        key={item.id}
                        item={item}
                        checked={selected.has(item.id)}
                        onCheckedChange={() =>
                          setSelected((current) => {
                            const next = new Set(current);
                            if (next.has(item.id)) next.delete(item.id);
                            else next.add(item.id);
                            return next;
                          })
                        }
                      />
                    ))}
                  </div>
                  <div
                    style={{ height: (visibleQueue.length - virtualEnd) * exportRowHeight }}
                    aria-hidden="true"
                  />
                </div>
              </SortableContext>
            </DndContext>
          </section>

          <aside className="h-fit space-y-5 xl:sticky xl:top-5">
            <section className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-panel)] p-5">
              <h2 className="font-display text-xl">Издание</h2>
              <div className="mt-4 grid gap-4">
                <Field label="Название">
                  <Input
                    value={profile.metadata.title}
                    maxLength={200}
                    onChange={(event) =>
                      setProfile({
                        ...profile,
                        metadata: { ...profile.metadata, title: event.target.value },
                      })
                    }
                  />
                </Field>
                <Field label="Автор">
                  <Input
                    value={profile.metadata.author}
                    maxLength={200}
                    onChange={(event) =>
                      setProfile({
                        ...profile,
                        metadata: { ...profile.metadata, author: event.target.value },
                      })
                    }
                  />
                </Field>
                <Field label="Подзаголовок">
                  <Input
                    value={profile.metadata.subtitle}
                    maxLength={200}
                    onChange={(event) =>
                      setProfile({
                        ...profile,
                        metadata: { ...profile.metadata, subtitle: event.target.value },
                      })
                    }
                  />
                </Field>
                <Field label="Аннотация">
                  <textarea
                    className="min-h-24 w-full resize-y rounded-[var(--radius-sm)] border border-[var(--border-strong)] bg-[var(--surface-raised)] p-3 text-sm"
                    value={profile.metadata.annotation}
                    maxLength={4000}
                    onChange={(event) =>
                      setProfile({
                        ...profile,
                        metadata: { ...profile.metadata, annotation: event.target.value },
                      })
                    }
                  />
                </Field>
                <Field label="Обложка">
                  <Select
                    value={profile.metadata.coverAssetId ?? ""}
                    onChange={(event) =>
                      setProfile({
                        ...profile,
                        metadata: {
                          ...profile.metadata,
                          coverAssetId: event.target.value || undefined,
                        },
                      })
                    }
                  >
                    <option value="">Без обложки</option>
                    {imageMedia.map((item) => (
                      <option key={item.id} value={item.id}>
                        {item.originalName}
                      </option>
                    ))}
                  </Select>
                </Field>
                <label className="inline-flex cursor-pointer items-center gap-2 text-sm text-[var(--accent)]">
                  <ImagePlus className="size-4" />{" "}
                  {uploadCover.isPending ? "Загружаем…" : "Загрузить новую обложку"}
                  <input
                    className="sr-only"
                    type="file"
                    accept="image/png,image/jpeg,image/gif"
                    disabled={uploadCover.isPending}
                    onChange={(event) => {
                      const file = event.target.files?.[0];
                      if (file) uploadCover.mutate(file);
                    }}
                  />
                </label>
              </div>
            </section>

            <PublicationPreview
              profile={profile}
              coverUrl={cover?.contentUrl}
              documents={previewDocuments}
            />

            {job.data?.export.status === "ready" && job.data.export.downloadUrl ? (
              <Button asChild className="w-full">
                <a href={job.data.export.downloadUrl}>
                  <Download className="size-4" /> Скачать {profile.lastFormat.toUpperCase()}
                </a>
              </Button>
            ) : jobId && job.data?.export.status !== "failed" ? (
              <div className="flex items-center gap-2 rounded-[var(--radius-md)] bg-[var(--accent-soft)] px-3 py-3 text-sm text-[var(--accent-ink)]">
                <LoaderCircle className="size-4 animate-spin" /> Собираем файл…
              </div>
            ) : null}
            {create.error || job.data?.export.status === "failed" ? (
              <Banner
                variant="danger"
                title="Экспорт не готов"
                body={exportFailureMessage(job.data?.export.errorCode)}
              />
            ) : null}
          </aside>
        </div>
      </div>
    </ProjectUtilityShell>
  );
}

function exportFailureMessage(errorCode?: string): string {
  switch (errorCode) {
    case "media_unavailable":
      return "Обложка или изображение недоступно. Проверьте медиа проекта и повторите сборку.";
    case "invalid_export_profile":
      return "Проверьте метаданные и параметры оформления.";
    case "pdf_renderer_unavailable":
      return "Сервис PDF временно недоступен. Можно собрать DOCX или EPUB и повторить позже.";
    default:
      return "Проверьте параметры и попробуйте создать файл ещё раз.";
  }
}

function SortableExportRow({
  item,
  checked,
  onCheckedChange,
}: {
  item: ExportQueueItem;
  checked: boolean;
  onCheckedChange: () => void;
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    setActivatorNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: item.id });
  return (
    <div
      ref={setNodeRef}
      data-export-queue-row=""
      className={`flex min-h-12 items-center gap-2 rounded-[var(--radius-sm)] px-2 hover:bg-[var(--surface-subtle)] ${isDragging ? "z-10 opacity-60" : ""}`}
      style={{ transform: CSS.Transform.toString(transform), transition }}
    >
      <button
        ref={setActivatorNodeRef}
        type="button"
        className="ui-tooltip grid size-8 touch-none place-items-center rounded text-[var(--ink-tertiary)] hover:bg-[var(--surface-raised)]"
        aria-label={`Изменить порядок «${item.title}»`}
        data-ui-tooltip={`Изменить порядок «${item.title}»`}
        data-ui-tooltip-side="right"
        {...attributes}
        {...listeners}
      >
        <GripVertical className="size-4" />
      </button>
      <Checkbox
        checked={checked}
        onCheckedChange={onCheckedChange}
        aria-label={`Включить «${item.title}»`}
      />
      <FileText className="size-4 shrink-0 text-[var(--ink-tertiary)]" />
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm">{item.title}</p>
        <p className="truncate text-[11px] text-[var(--ink-tertiary)]">{item.breadcrumb}</p>
      </div>
      <span className="text-[10px] text-[var(--ink-tertiary)]">{item.kind}</span>
    </div>
  );
}

function PublicationPreview({
  profile,
  coverUrl,
  documents,
}: {
  profile: ExportProfile;
  coverUrl?: string;
  documents: Array<{ id: string; title: string; plainText?: string }>;
}) {
  const [selectedPageId, setSelectedPageId] = useState("");
  const pages = buildPublicationPreviewPages(profile, Boolean(coverUrl), documents);
  const activePage = pages.find((page) => page.id === selectedPageId) ?? pages[0];
  const activePageIndex = activePage ? pages.findIndex((page) => page.id === activePage.id) : -1;
  const activeDocumentIndex =
    activePage?.kind === "document"
      ? documents.findIndex((document) => document.id === activePage.documentId)
      : -1;
  const activeDocument = activeDocumentIndex >= 0 ? documents[activeDocumentIndex] : undefined;
  const nextDocument = activeDocumentIndex >= 0 ? documents[activeDocumentIndex + 1] : undefined;
  const previewNode = useQuery({
    queryKey: ["node-preview", activeDocument?.id],
    queryFn: () => getNode(activeDocument?.id ?? ""),
    enabled: Boolean(activeDocument?.id),
  });
  const font = {
    literata: "Literata",
    "source-serif": "Source Serif 4",
    inter: "Inter",
    "ibm-plex-mono": "IBM Plex Mono",
  }[profile.style.fontFamily];
  const style = {
    fontFamily: font,
    fontSize: `${profile.style.fontSizePt}px`,
    lineHeight: profile.style.lineHeight,
  };
  return (
    <section className="overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-subtle)] p-4">
      <div className="mb-3 flex items-center justify-between gap-3">
        <h2 className="text-xs font-semibold tracking-wider text-[var(--ink-tertiary)] uppercase">
          Предпросмотр
        </h2>
        <span className="text-[11px] text-[var(--ink-tertiary)]">
          {activePageIndex >= 0 ? `${activePageIndex + 1} из ${pages.length}` : "Нет страниц"}
        </span>
      </div>
      {pages.length > 0 ? (
        <div className="mb-3 flex items-center gap-1.5">
          <Button
            type="button"
            variant="ghost"
            size="compact"
            className="px-2"
            aria-label="Предыдущая страница предпросмотра"
            disabled={activePageIndex <= 0}
            onClick={() => setSelectedPageId(pages[activePageIndex - 1]?.id ?? "")}
          >
            <ChevronLeft className="size-4" aria-hidden="true" />
          </Button>
          <Select
            className="h-9 min-w-0 flex-1 text-xs"
            aria-label="Страница предпросмотра"
            value={activePage?.id ?? ""}
            onChange={(event) => setSelectedPageId(event.target.value)}
          >
            {pages.map((page) => (
              <option key={page.id} value={page.id}>
                {page.label}
              </option>
            ))}
          </Select>
          <Button
            type="button"
            variant="ghost"
            size="compact"
            className="px-2"
            aria-label="Следующая страница предпросмотра"
            disabled={activePageIndex < 0 || activePageIndex >= pages.length - 1}
            onClick={() => setSelectedPageId(pages[activePageIndex + 1]?.id ?? "")}
          >
            <ChevronRight className="size-4" aria-hidden="true" />
          </Button>
        </div>
      ) : null}
      <article
        className="mx-auto aspect-[0.707] max-h-[560px] overflow-hidden bg-[var(--surface-panel)] px-[12%] py-[10%] text-[var(--ink-primary)] shadow-sm"
        style={style}
        aria-label={activePage?.label ?? "Пустой предпросмотр"}
      >
        {activePage?.kind === "cover" && coverUrl ? (
          <div className="flex h-full items-center justify-center">
            <img src={coverUrl} alt="Обложка" className="max-h-full w-full object-contain" />
          </div>
        ) : null}
        {activePage?.kind === "title" ? (
          <div className="flex h-full flex-col justify-center text-center">
            <h3 className="font-display text-[22px] leading-tight">{profile.metadata.title}</h3>
            {profile.metadata.subtitle ? (
              <p className="mt-2 text-xs">{profile.metadata.subtitle}</p>
            ) : null}
            {profile.metadata.author ? (
              <p className="mt-5 text-xs tracking-wide uppercase">{profile.metadata.author}</p>
            ) : null}
          </div>
        ) : null}
        {activePage?.kind === "toc" ? (
          <div>
            <h4 className="text-center text-xs font-semibold tracking-wide uppercase">
              Оглавление
            </h4>
            <ol className="mt-2 grid gap-1 text-[10px]">
              {documents.map((document) => (
                <li key={document.title} className="truncate">
                  {document.title}
                </li>
              ))}
            </ol>
          </div>
        ) : null}
        {activePage?.kind === "document" ? (
          <>
            {profile.chapterTitles && activeDocument?.title ? (
              <h4 className="text-center text-sm font-semibold">{activeDocument.title}</h4>
            ) : null}
            <p
              className={`mt-4 ${profile.style.textAlign === "justify" ? "text-justify" : "text-left"}`}
              style={{
                textIndent:
                  profile.style.paragraphMode === "indent"
                    ? `${profile.style.firstLineIndentEm}em`
                    : 0,
              }}
            >
              {previewNode.data?.node.plainText?.slice(0, 650) ||
                activeDocument?.plainText?.slice(0, 650) ||
                (previewNode.isFetching ? "Загружаем текст…" : "В документе пока нет текста.")}
            </p>
            {!profile.chapterPageBreak && nextDocument ? (
              <>
                <div aria-hidden="true" className="my-5 text-center tracking-[0.3em]">
                  {previewSceneSeparator(profile.style.sceneSeparator)}
                </div>
                {profile.chapterTitles && nextDocument.title ? (
                  <h4 className="text-center text-sm font-semibold">{nextDocument.title}</h4>
                ) : null}
                <p
                  className={`mt-4 ${profile.style.textAlign === "justify" ? "text-justify" : "text-left"}`}
                  style={{
                    textIndent:
                      profile.style.paragraphMode === "indent"
                        ? `${profile.style.firstLineIndentEm}em`
                        : 0,
                  }}
                >
                  {nextDocument.plainText?.slice(0, 260)}
                </p>
              </>
            ) : null}
          </>
        ) : null}
        {!activePage ? (
          <p className="text-center text-sm text-[var(--ink-tertiary)]">
            Выберите документы, чтобы увидеть начало рукописи.
          </p>
        ) : null}
      </article>
    </section>
  );
}

function previewSceneSeparator(value: ExportProfile["style"]["sceneSeparator"]) {
  if (value === "dots") return "· · ·";
  if (value === "line") return "—";
  return "* * *";
}

function CheckOption({
  checked,
  onChange,
  label,
}: {
  checked: boolean;
  onChange: (value: boolean) => void;
  label: string;
}) {
  const id = useId();
  return (
    <label htmlFor={id} className="flex cursor-pointer items-center gap-3">
      <Checkbox id={id} checked={checked} onCheckedChange={(value) => onChange(value === true)} />
      {label}
    </label>
  );
}
