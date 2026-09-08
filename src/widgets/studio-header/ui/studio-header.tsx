import * as DropdownMenuPrimitive from "@radix-ui/react-dropdown-menu";
import { Link } from "@tanstack/react-router";
import {
  ArrowLeft,
  BookOpen,
  Brain,
  ChevronRight,
  Download,
  LogOut,
  MessageCircle,
  MoreHorizontal,
  PencilLine,
  Search,
  Server,
  Settings,
} from "lucide-react";
import { useEffect, useRef, useState } from "react";

import type { Project } from "~entities/project";
import { useSession } from "~entities/session";
import { EditProjectDialog } from "~features/edit-project";
import { maximumNodeTitleLength, normalizeNodeTitle } from "~shared/lib";
import { BrandMark, Button, type SaveState, SaveStatus } from "~shared/ui";

interface StudioHeaderProps {
  project: Project;
  nodeId?: string;
  nodeTitle: string;
  saveState: SaveState;
  onDictate?: () => void;
  onAssistant?: () => void;
  onRenameNode?: (title: string) => void;
}

export function StudioHeader({
  project,
  nodeId,
  nodeTitle,
  saveState,
  onDictate,
  onAssistant,
  onRenameNode,
}: StudioHeaderProps) {
  const session = useSession();
  const [editingProject, setEditingProject] = useState(false);
  const [editingNodeTitle, setEditingNodeTitle] = useState(false);
  const [nodeTitleDraft, setNodeTitleDraft] = useState(nodeTitle);
  const nodeTitleInputRef = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (!editingNodeTitle) setNodeTitleDraft(nodeTitle);
  }, [editingNodeTitle, nodeTitle]);
  useEffect(() => {
    if (!editingNodeTitle) return;
    nodeTitleInputRef.current?.focus();
    nodeTitleInputRef.current?.select();
  }, [editingNodeTitle]);
  const commitNodeTitle = () => {
    const title = normalizeNodeTitle(nodeTitleDraft, nodeTitle);
    setEditingNodeTitle(false);
    if (title) onRenameNode?.(title);
  };
  const initials = (session.data?.user.displayName || "Автор")
    .split(/\s+/)
    .map((part) => part[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();

  return (
    <header className="flex h-14 items-center gap-2 border-b border-[var(--border-subtle)] bg-[var(--surface-panel)] px-2 sm:px-[18px]">
      <Button asChild variant="ghost" size="icon" aria-label="К проектам">
        <Link to="/projects">
          <ArrowLeft className="size-4" aria-hidden="true" />
        </Link>
      </Button>
      <BrandMark className="hidden size-7 shrink-0 sm:block" aria-hidden="true" />
      <div className="flex min-w-0 items-center gap-2">
        <p className="truncate text-[14px] font-medium">{project.title}</p>
        <ChevronRight
          className="hidden size-3.5 shrink-0 text-[var(--ink-tertiary)] sm:block"
          aria-hidden="true"
        />
        {editingNodeTitle ? (
          <input
            ref={nodeTitleInputRef}
            value={nodeTitleDraft}
            maxLength={maximumNodeTitleLength}
            aria-label={`Новое название для «${nodeTitle}»`}
            className="hidden h-8 min-w-24 max-w-60 rounded border border-[var(--focus)] bg-[var(--surface-paper)] px-2 text-[13px] outline-none sm:block"
            onFocus={(event) => event.currentTarget.select()}
            onChange={(event) => setNodeTitleDraft(event.target.value)}
            onBlur={commitNodeTitle}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                commitNodeTitle();
              } else if (event.key === "Escape") {
                event.preventDefault();
                setNodeTitleDraft(nodeTitle);
                setEditingNodeTitle(false);
              }
            }}
          />
        ) : (
          <button
            type="button"
            className="hidden min-w-0 truncate rounded px-1 text-[13px] text-[var(--ink-secondary)] hover:bg-[var(--surface-subtle)] focus-visible:outline-2 sm:block"
            aria-label={`Материал «${nodeTitle}». Дважды нажмите, чтобы переименовать`}
            title="Дважды нажмите, чтобы переименовать"
            onDoubleClick={() => onRenameNode && setEditingNodeTitle(true)}
          >
            {nodeTitle}
          </button>
        )}
      </div>
      <div className="hidden md:block">
        <SaveStatus state={saveState} />
      </div>
      <div className="ml-auto flex items-center gap-1.5">
        <Button asChild variant="ghost" size="icon" aria-label="Поиск по проекту">
          <Link to="/projects/$projectId/search" params={{ projectId: project.id }}>
            <Search className="size-4" aria-hidden="true" />
          </Link>
        </Button>
        <Button
          asChild
          variant="ghost"
          size="compact"
          className="rounded-[var(--radius-pill)] px-3"
        >
          <Link
            to="/projects/$projectId/assistant-context"
            params={{ projectId: project.id }}
            aria-label="Контекст, память и руководство ассистента"
          >
            <Brain className="size-4" aria-hidden="true" />
            <span className="hidden xl:inline">Контекст</span>
          </Link>
        </Button>
        <Button
          type="button"
          variant="record"
          size="compact"
          onClick={onDictate}
          disabled={!onDictate}
          className="rounded-[var(--radius-pill)] px-4"
          aria-label={onDictate ? "Диктовать" : "Откройте документ, чтобы начать диктовку"}
        >
          <span className="size-2 rounded-full bg-current" aria-hidden="true" />
          <span className="hidden sm:inline">Диктовка</span>
        </Button>
        <Button
          type="button"
          variant="creative"
          size="compact"
          onClick={onAssistant}
          disabled={!onAssistant}
          className="rounded-[var(--radius-pill)] px-4"
          aria-label={onAssistant ? "Открыть ассистента" : "Откройте документ для ассистента"}
        >
          <MessageCircle className="size-4" aria-hidden="true" />
          <span className="hidden sm:inline">Ассистент</span>
        </Button>
        <Button asChild variant="ghost" size="icon" aria-label="Экспорт">
          <Link to="/projects/$projectId/export" params={{ projectId: project.id }}>
            <Download className="size-4" aria-hidden="true" />
          </Link>
        </Button>
        <DropdownMenuPrimitive.Root>
          <DropdownMenuPrimitive.Trigger asChild>
            <Button type="button" variant="ghost" size="icon" aria-label="Ещё">
              <MoreHorizontal className="size-4" aria-hidden="true" />
            </Button>
          </DropdownMenuPrimitive.Trigger>
          <DropdownMenuPrimitive.Portal>
            <DropdownMenuPrimitive.Content
              align="end"
              sideOffset={8}
              className="z-50 min-w-56 rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-raised)] p-1 shadow-[0_10px_24px_var(--shadow-color)]"
            >
              <DropdownMenuPrimitive.Item asChild>
                <a
                  href="/docs"
                  className="flex min-h-10 cursor-default items-center gap-2 rounded-[var(--radius-sm)] px-3 text-sm outline-none data-[highlighted]:bg-[var(--surface-subtle)]"
                >
                  <BookOpen className="size-4" aria-hidden="true" />
                  Документация
                </a>
              </DropdownMenuPrimitive.Item>
              <DropdownMenuPrimitive.Item asChild>
                <Link
                  to="/projects/$projectId/assistant-context"
                  params={{ projectId: project.id }}
                  className="flex min-h-10 cursor-default items-center gap-2 rounded-[var(--radius-sm)] px-3 text-sm outline-none data-[highlighted]:bg-[var(--surface-subtle)]"
                >
                  <Brain className="size-4" aria-hidden="true" />
                  Контекст ассистента
                </Link>
              </DropdownMenuPrimitive.Item>
              <DropdownMenuPrimitive.Item
                onSelect={() => setEditingProject(true)}
                className="flex min-h-10 cursor-default items-center gap-2 rounded-[var(--radius-sm)] px-3 text-sm outline-none data-[highlighted]:bg-[var(--surface-subtle)]"
              >
                <PencilLine className="size-4" aria-hidden="true" />
                Настройки проекта
              </DropdownMenuPrimitive.Item>
              <DropdownMenuPrimitive.Item asChild>
                <Link
                  to="/settings/assistant-providers"
                  className="flex min-h-10 cursor-default items-center gap-2 rounded-[var(--radius-sm)] px-3 text-sm outline-none data-[highlighted]:bg-[var(--surface-subtle)]"
                >
                  <Server className="size-4" aria-hidden="true" />
                  AI и провайдеры
                </Link>
              </DropdownMenuPrimitive.Item>
              <DropdownMenuPrimitive.Item asChild>
                <Link
                  to="/settings"
                  search={{ projectId: project.id, nodeId }}
                  className="flex min-h-10 cursor-default items-center gap-2 rounded-[var(--radius-sm)] px-3 text-sm outline-none data-[highlighted]:bg-[var(--surface-subtle)]"
                >
                  <Settings className="size-4" aria-hidden="true" />
                  Настройки редактора
                </Link>
              </DropdownMenuPrimitive.Item>
              <DropdownMenuPrimitive.Separator className="my-1 h-px bg-[var(--border-subtle)]" />
              <DropdownMenuPrimitive.Item asChild>
                <Link
                  to="/projects"
                  className="flex min-h-10 cursor-default items-center gap-2 rounded-[var(--radius-sm)] px-3 text-sm outline-none data-[highlighted]:bg-[var(--surface-subtle)]"
                >
                  <LogOut className="size-4" aria-hidden="true" />К проектам
                </Link>
              </DropdownMenuPrimitive.Item>
            </DropdownMenuPrimitive.Content>
          </DropdownMenuPrimitive.Portal>
        </DropdownMenuPrimitive.Root>
        <span
          className="hidden size-8 shrink-0 place-items-center rounded-full bg-[var(--surface-subtle)] text-[11px] font-semibold text-[var(--ink-secondary)] sm:grid"
          title={`Профиль: ${session.data?.user.email ?? "Автор"}`}
        >
          {initials}
        </span>
      </div>
      <EditProjectDialog project={project} open={editingProject} onOpenChange={setEditingProject} />
    </header>
  );
}
