import {
  closestCenter,
  DndContext,
  type DragEndEvent,
  type DragStartEvent,
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
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import {
  Blocks,
  BookOpen,
  Boxes,
  Brain,
  ChevronDown,
  ChevronRight,
  Clapperboard,
  Copy,
  FileAudio,
  FileText,
  FolderOpen,
  Gamepad2,
  GripVertical,
  History,
  LifeBuoy,
  ListTree,
  MessagesSquare,
  MoreHorizontal,
  Move,
  NotebookPen,
  PanelLeftClose,
  PanelLeftOpen,
  Pencil,
  Plus,
  RotateCcw,
  SpellCheck2,
  StickyNote,
  Trash2,
  Workflow,
} from "lucide-react";
import { type ReactNode, useEffect, useMemo, useRef, useState } from "react";

import {
  createNode,
  type DocumentNode,
  duplicateNode,
  moveNode,
  type ProjectStructure,
  type ProjectWorkspace,
  restoreNode,
  trashNode,
  updateNode,
} from "~entities/project";
import { cn } from "~shared/lib";
import {
  Button,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  useActionDialog,
  useToast,
} from "~shared/ui";

import {
  buildProjectOutline,
  canMoveNodeToParent,
  canPlaceNodeAtParent,
  getValidParentNodes,
  moveNodeInOutline,
  nodeTypeLabel,
  orderedSiblings,
  type ProjectOutlineNode,
  siblingIndex,
} from "../model/project-outline";

function flattenProjectOutline(items: ProjectOutlineNode[]): ProjectOutlineNode[] {
  const result: ProjectOutlineNode[] = [];
  const pending = [...items].reverse();
  while (pending.length > 0) {
    const item = pending.pop();
    if (!item) continue;
    result.push(item);
    for (let index = item.children.length - 1; index >= 0; index -= 1) {
      const child = item.children[index];
      if (child) pending.push(child);
    }
  }
  return result;
}

interface VisibleOutlineItem {
  item: ProjectOutlineNode;
  depth: number;
}

function flattenVisibleOutline(
  items: ProjectOutlineNode[],
  collapsed: Set<string>,
): VisibleOutlineItem[] {
  const result: VisibleOutlineItem[] = [];
  const pending = items.map((item) => ({ item, depth: 0 })).reverse();
  while (pending.length > 0) {
    const current = pending.pop();
    if (!current) continue;
    result.push(current);
    if (collapsed.has(current.item.node.id)) continue;
    for (let index = current.item.children.length - 1; index >= 0; index -= 1) {
      const child = current.item.children[index];
      if (child) pending.push({ item: child, depth: current.depth + 1 });
    }
  }
  return result;
}

interface ProjectTreeProps {
  projectId: string;
  activeNodeId: string;
  nodes: DocumentNode[];
  structure: ProjectStructure;
  collapsed: boolean;
  onToggle: () => void;
  integratedWorkspace?: boolean;
}

const iconByName = {
  folder: FolderOpen,
  "book-open": BookOpen,
  "file-text": FileText,
  "notebook-pen": NotebookPen,
  "sticky-note": StickyNote,
  clapperboard: Clapperboard,
  "list-tree": ListTree,
  workflow: Workflow,
  "gamepad-2": Gamepad2,
  boxes: Boxes,
} as const;

function nodeIcon(structure: ProjectStructure, kind: string) {
  const name = structure.nodeTypes.find((type) => type.key === kind)?.icon;
  return iconByName[name as keyof typeof iconByName] ?? FileText;
}

export function ProjectTree({
  projectId,
  activeNodeId,
  nodes,
  structure,
  collapsed,
  onToggle,
  integratedWorkspace = false,
}: ProjectTreeProps) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { showToast } = useToast();
  const { requestInput, requestSelect } = useActionDialog();
  const [actionsOpen, setActionsOpen] = useState(false);
  const [draggedNodeId, setDraggedNodeId] = useState<string>();
  const [dropTargetId, setDropTargetId] = useState<string>();
  const contentNodes = useMemo(() => nodes.filter((node) => !node.trashedAt), [nodes]);
  const trashedNodes = useMemo(() => nodes.filter((node) => node.trashedAt), [nodes]);
  const activeNode = contentNodes.find((node) => node.id === activeNodeId);
  const outline = useMemo(() => buildProjectOutline(nodes), [nodes]);
  const [collapsedNodeIds, setCollapsedNodeIds] = useState<Set<string>>(() => new Set());
  const [treeScrollTop, setTreeScrollTop] = useState(0);
  const treeViewportRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const byId = new Map(contentNodes.map((node) => [node.id, node]));
    const expanded = new Set<string>();
    let current = byId.get(activeNodeId);
    while (current?.parentId && !expanded.has(current.parentId)) {
      expanded.add(current.parentId);
      current = byId.get(current.parentId);
    }
    setCollapsedNodeIds((previous) => {
      const next = new Set(previous);
      for (const id of expanded) next.delete(id);
      for (const item of flattenProjectOutline(outline.roots)) {
        if (
          item.children.length > 0 &&
          !expanded.has(item.node.id) &&
          item.node.id !== activeNodeId
        ) {
          next.add(item.node.id);
        }
      }
      return next;
    });
  }, [activeNodeId, contentNodes, outline]);
  const visibleOutline = useMemo(
    () => flattenVisibleOutline(outline.roots, collapsedNodeIds),
    [collapsedNodeIds, outline],
  );
  const treeRowHeight = 36;
  const treeViewportHeight = 560;
  const treeVirtualStart = Math.max(0, Math.floor(treeScrollTop / treeRowHeight) - 10);
  const treeVirtualEnd = Math.min(
    visibleOutline.length,
    Math.ceil((treeScrollTop + treeViewportHeight) / treeRowHeight) + 10,
  );
  const virtualOutline = visibleOutline.slice(treeVirtualStart, treeVirtualEnd);
  const sortableIds = useMemo(
    () => virtualOutline.map(({ item }) => item.node.id),
    [virtualOutline],
  );
  useEffect(() => {
    const index = visibleOutline.findIndex(({ item }) => item.node.id === activeNodeId);
    if (index < 0 || !treeViewportRef.current) return;
    const top = Math.max(0, index * treeRowHeight - treeViewportHeight / 3);
    treeViewportRef.current.scrollTop = top;
    setTreeScrollTop(top);
  }, [activeNodeId, visibleOutline]);
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 180, tolerance: 8 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
  const invalidate = () => queryClient.invalidateQueries({ queryKey: ["project", projectId] });

  const createMutation = useMutation({
    mutationFn: (input: { kind: string; title: string; parentId?: string }) =>
      createNode(projectId, input),
    onSuccess: async ({ node }) => {
      await invalidate();
      showToast({
        title: `Материал создан: ${nodeTypeLabel(structure, node.kind)}`,
      });
      if (structure.nodeTypes.find((item) => item.key === node.kind)?.contentBearing) {
        void navigate({
          to: "/projects/$projectId/studio/$nodeId",
          params: { projectId, nodeId: node.id },
        });
      }
    },
    onError: (error) =>
      showToast({
        title: "Не удалось создать материал",
        description: error.message,
      }),
  });
  const updateMutation = useMutation({
    mutationFn: (input: { title?: string; status?: DocumentNode["status"] }) =>
      updateNode(activeNodeId, input),
    onSuccess: async () => {
      setActionsOpen(false);
      await invalidate();
      showToast({ title: "Свойства сохранены" });
    },
  });
  const duplicateMutation = useMutation({
    mutationFn: () => duplicateNode(activeNodeId),
    onSuccess: async () => {
      setActionsOpen(false);
      await invalidate();
      showToast({ title: "Создана копия" });
    },
  });
  const moveMutation = useMutation({
    mutationFn: (input: {
      nodeId: string;
      parentId?: string;
      targetIndex: number;
      previousParentId?: string;
      previousIndex: number;
    }) =>
      moveNode(input.nodeId, {
        parentId: input.parentId,
        targetIndex: input.targetIndex,
      }),
    onMutate: async (input) => {
      await queryClient.cancelQueries({ queryKey: ["project", projectId] });
      const previous = queryClient.getQueryData<ProjectWorkspace>(["project", projectId]);
      queryClient.setQueryData<ProjectWorkspace>(["project", projectId], (current) =>
        current
          ? {
              ...current,
              nodes: moveNodeInOutline(
                current.nodes,
                input.nodeId,
                input.parentId,
                input.targetIndex,
              ),
            }
          : current,
      );
      return { previous };
    },
    onSuccess: async (_response, input) => {
      setActionsOpen(false);
      setDraggedNodeId(undefined);
      setDropTargetId(undefined);
      await invalidate();
      showToast({
        title: "Материал перемещён",
        actionLabel: "Отменить",
        onAction: () => {
          void moveNode(input.nodeId, {
            parentId: input.previousParentId,
            targetIndex: input.previousIndex,
          }).then(invalidate);
        },
      });
    },
    onError: (error, _input, context) => {
      if (context?.previous) {
        queryClient.setQueryData(["project", projectId], context.previous);
      }
      setDraggedNodeId(undefined);
      setDropTargetId(undefined);
      showToast({
        title: "Не удалось переместить",
        description: error.message,
      });
    },
  });
  const trashMutation = useMutation({
    mutationFn: () => trashNode(activeNodeId),
    onSuccess: async () => {
      setActionsOpen(false);
      await invalidate();
      showToast({
        title: "Материал в корзине",
        actionLabel: "Отменить",
        onAction: () => void restoreNode(activeNodeId).then(invalidate),
      });
    },
  });
  const restoreMutation = useMutation({
    mutationFn: restoreNode,
    onSuccess: invalidate,
  });

  const addNode = (kind: string, parentId?: string) => {
    const type = structure.nodeTypes.find((item) => item.key === kind);
    if (!type) return;
    const defaultTitle = `${type.label} без названия`;
    void requestInput({
      title: `Создать: ${type.label.toLocaleLowerCase()}`,
      description: parentId
        ? "Материал появится внутри выбранного раздела."
        : `Материал появится на верхнем уровне «${structure.rootLabel}».`,
      label: "Название",
      defaultValue: defaultTitle,
      icon: nodeIcon(structure, kind),
      confirmLabel: "Создать",
    }).then((title) => {
      if (title?.trim()) {
        createMutation.mutate({ kind, title: title.trim(), parentId });
      }
    });
  };

  const chooseNodeType = (parentId?: string) => {
    const parent = parentId ? contentNodes.find((node) => node.id === parentId) : undefined;
    const allowed = structure.nodeTypes.filter((type) =>
      parent
        ? structure.nodeTypes
            .find((candidate) => candidate.key === parent.kind)
            ?.allowedChildren?.includes(type.key)
        : type.canBeRoot,
    );
    if (allowed.length === 0) {
      showToast({ title: "В этот раздел нельзя добавить вложенный материал" });
      return;
    }
    void requestSelect({
      title: parent ? `Добавить в «${parent.title}»` : `Добавить в «${structure.rootLabel}»`,
      description: "Типы и допустимая вложенность определяются структурой этого проекта.",
      label: "Тип материала",
      defaultValue: allowed[0]?.key ?? "",
      icon: Plus,
      confirmLabel: "Продолжить",
      options: allowed.map((type) => ({ value: type.key, label: type.label })),
    }).then((kind) => {
      if (kind) addNode(kind, parentId);
    });
  };

  const moveDocument = (nodeId: string, parentId?: string) => {
    const node = contentNodes.find((candidate) => candidate.id === nodeId);
    if (!node || !canPlaceNodeAtParent(contentNodes, nodeId, parentId, structure)) return;
    moveMutation.mutate({
      nodeId,
      parentId,
      targetIndex: orderedSiblings(contentNodes, parentId).filter((item) => item.id !== nodeId)
        .length,
      previousParentId: node.parentId,
      previousIndex: siblingIndex(contentNodes, nodeId),
    });
  };

  const moveDocumentToIndex = (
    nodeId: string,
    parentId: string | undefined,
    targetIndex: number,
  ) => {
    const node = contentNodes.find((candidate) => candidate.id === nodeId);
    if (!node || !canPlaceNodeAtParent(contentNodes, nodeId, parentId, structure)) return;
    const previousIndex = siblingIndex(contentNodes, nodeId);
    if (node.parentId === parentId && previousIndex === targetIndex) return;
    moveMutation.mutate({
      nodeId,
      parentId,
      targetIndex,
      previousParentId: node.parentId,
      previousIndex,
    });
  };

  const handleDragStart = (event: DragStartEvent) => setDraggedNodeId(String(event.active.id));

  const handleDragEnd = (event: DragEndEvent) => {
    setDraggedNodeId(undefined);
    setDropTargetId(undefined);
    if (!event.over || event.active.id === event.over.id) return;
    const sourceId = String(event.active.id);
    const targetId = String(event.over.id);
    const source = contentNodes.find((node) => node.id === sourceId);
    const target = contentNodes.find((node) => node.id === targetId);
    if (!source || !target) return;
    const activeRect = event.active.rect.current.translated;
    const wantsInside =
      activeRect !== null &&
      activeRect.left > event.over.rect.left + 36 &&
      canMoveNodeToParent(contentNodes, sourceId, targetId, structure);
    if (wantsInside) {
      moveDocumentToIndex(
        sourceId,
        targetId,
        orderedSiblings(contentNodes, targetId).filter((node) => node.id !== sourceId).length,
      );
      return;
    }
    const parentId = target.parentId;
    const siblings = orderedSiblings(contentNodes, parentId).filter((node) => node.id !== sourceId);
    const targetIndex = siblings.findIndex((node) => node.id === targetId);
    if (targetIndex < 0) return;
    const after =
      activeRect !== null &&
      activeRect.top + activeRect.height / 2 > event.over.rect.top + event.over.rect.height / 2;
    moveDocumentToIndex(sourceId, parentId, targetIndex + (after ? 1 : 0));
  };

  const renderOutlineRow = ({ item: { node, children }, depth }: VisibleOutlineItem) => {
    const Icon = nodeIcon(structure, node.kind);
    const nodeType = structure.nodeTypes.find((item) => item.key === node.kind);
    const canAcceptDrop =
      Boolean(draggedNodeId) &&
      canMoveNodeToParent(contentNodes, draggedNodeId ?? "", node.id, structure);
    return (
      <div key={node.id}>
        <SortableTreeRow
          id={node.id}
          title={node.title}
          level={depth + 1}
          selected={node.id === activeNodeId}
          disabled={moveMutation.isPending}
          kind={node.kind}
          canAcceptDrop={canAcceptDrop}
          className={cn(
            node.id === activeNodeId &&
              "bg-[var(--surface-selected)] font-medium text-[var(--accent-ink)]",
            dropTargetId === node.id && "outline-2 outline-offset-[-2px] outline-[var(--accent)]",
          )}
          paddingLeft={Math.min(depth, 5) * 14 + 2}
        >
          {children.length > 0 ? (
            <button
              type="button"
              className="grid size-6 shrink-0 place-items-center rounded hover:bg-[var(--surface-subtle)]"
              aria-label={
                collapsedNodeIds.has(node.id)
                  ? `Развернуть «${node.title}»`
                  : `Свернуть «${node.title}»`
              }
              aria-expanded={!collapsedNodeIds.has(node.id)}
              onClick={() =>
                setCollapsedNodeIds((current) => {
                  const next = new Set(current);
                  if (next.has(node.id)) next.delete(node.id);
                  else next.add(node.id);
                  return next;
                })
              }
            >
              {collapsedNodeIds.has(node.id) ? (
                <ChevronRight className="size-3.5" aria-hidden="true" />
              ) : (
                <ChevronDown className="size-3.5" aria-hidden="true" />
              )}
            </button>
          ) : (
            <span className="block size-6 shrink-0" />
          )}
          <Link
            to="/projects/$projectId/studio/$nodeId"
            params={{ projectId, nodeId: node.id }}
            className="flex min-w-0 flex-1 items-center gap-2 py-2"
          >
            <Icon className="size-4 shrink-0" aria-hidden="true" />
            <span className="truncate">{node.title}</span>
          </Link>
          {(nodeType?.allowedChildren?.length ?? 0) > 0 ? (
            <button
              type="button"
              className="mr-1 grid size-7 place-items-center rounded opacity-0 hover:bg-[var(--surface-subtle)] group-hover/tree-row:opacity-100 focus:opacity-100"
              aria-label={`Добавить внутрь «${node.title}»`}
              onClick={() => chooseNodeType(node.id)}
            >
              <Plus className="size-3.5" aria-hidden="true" />
            </button>
          ) : null}
        </SortableTreeRow>
      </div>
    );
  };

  if (collapsed) {
    return (
      <nav
        className={cn(
          "hidden w-16 shrink-0 flex-col items-center border-r border-[var(--border-subtle)] bg-[var(--surface-panel)] lg:flex",
          !integratedWorkspace && "py-1",
        )}
        aria-label="Структура проекта свёрнута"
      >
        {integratedWorkspace ? (
          <div
            data-testid="project-tree-header"
            className="flex h-14 w-full shrink-0 items-center justify-center border-b border-[var(--border-subtle)]"
          >
            <Button
              variant="ghost"
              size="icon"
              aria-label="Развернуть структуру"
              onClick={onToggle}
            >
              <PanelLeftOpen className="size-4" aria-hidden="true" />
            </Button>
          </div>
        ) : (
          <Button variant="ghost" size="icon" aria-label="Развернуть структуру" onClick={onToggle}>
            <PanelLeftOpen className="size-4" aria-hidden="true" />
          </Button>
        )}
        <CompactStudioLink
          projectId={projectId}
          node={activeNode ?? contentNodes[0]}
          icon={Blocks}
          label={structure.rootLabel}
        />
        <div className="mt-auto grid gap-1">
          <UtilityLinks projectId={projectId} nodeId={activeNodeId} compact />
        </div>
      </nav>
    );
  }

  return (
    <nav
      className="hidden w-[280px] shrink-0 flex-col border-r border-[var(--border-subtle)] bg-[var(--surface-panel)] lg:flex"
      aria-label="Структура проекта"
    >
      <div
        data-testid="project-tree-header"
        className={cn(
          "flex shrink-0 items-center px-4",
          integratedWorkspace ? "h-14 border-b border-[var(--border-subtle)]" : "h-11",
        )}
      >
        <span className="text-[11px] font-semibold tracking-[0.14em] text-[var(--ink-tertiary)] uppercase">
          Структура
        </span>
        <div className="ml-auto flex">
          <Button variant="ghost" size="icon" aria-label="Свернуть структуру" onClick={onToggle}>
            <PanelLeftClose className="size-4" aria-hidden="true" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            aria-label="Добавить материал"
            onClick={() => chooseNodeType()}
          >
            <Plus className="size-4" aria-hidden="true" />
          </Button>
          <DropdownMenu open={actionsOpen} onOpenChange={setActionsOpen}>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                aria-label="Действия с материалом"
                disabled={!activeNode}
              >
                <MoreHorizontal className="size-4" aria-hidden="true" />
              </Button>
            </DropdownMenuTrigger>
            {activeNode ? (
              <DropdownMenuContent align="end">
                <TreeAction
                  icon={Pencil}
                  label="Переименовать"
                  onClick={() =>
                    void requestInput({
                      title: `Переименовать «${activeNode.title}»`,
                      description: "Содержимое и история не изменятся.",
                      label: "Новое название",
                      defaultValue: activeNode.title,
                      icon: Pencil,
                      confirmLabel: "Сохранить",
                    }).then((title) => {
                      if (title?.trim()) updateMutation.mutate({ title: title.trim() });
                    })
                  }
                />
                <TreeAction
                  icon={RotateCcw}
                  label="Изменить статус"
                  onClick={() =>
                    void requestSelect({
                      title: "Изменить статус",
                      description: "Статус помогает ориентироваться и не меняет содержимое.",
                      label: "Статус",
                      defaultValue: activeNode.status,
                      icon: RotateCcw,
                      confirmLabel: "Сохранить",
                      options: [
                        { value: "idea", label: "Идея" },
                        { value: "draft", label: "Черновик" },
                        { value: "editing", label: "Редактура" },
                        { value: "ready", label: "Готово" },
                      ],
                    }).then((status) => {
                      if (status)
                        updateMutation.mutate({
                          status: status as DocumentNode["status"],
                        });
                    })
                  }
                />
                <TreeAction
                  icon={Move}
                  label="Переместить"
                  onClick={() =>
                    void requestSelect({
                      title: `Переместить «${activeNode.title}»`,
                      description: "Показываются только допустимые родители по схеме проекта.",
                      label: "Новое расположение",
                      defaultValue: activeNode.parentId ?? "",
                      icon: Move,
                      confirmLabel: "Переместить",
                      options: [
                        { value: "", label: structure.rootLabel },
                        ...getValidParentNodes(contentNodes, activeNode.id, structure).map(
                          (node) => ({
                            value: node.id,
                            label: `${nodeTypeLabel(structure, node.kind)} · ${node.title}`,
                          }),
                        ),
                      ],
                    }).then((parentId) => {
                      if (parentId !== undefined)
                        moveDocument(activeNode.id, parentId || undefined);
                    })
                  }
                />
                <TreeAction
                  icon={Copy}
                  label="Дублировать"
                  onClick={() => duplicateMutation.mutate()}
                />
                <TreeAction
                  icon={Trash2}
                  label="В корзину"
                  destructive
                  onClick={() => trashMutation.mutate()}
                />
              </DropdownMenuContent>
            ) : null}
          </DropdownMenu>
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto px-2 pb-4">
        <div className="flex min-h-9 items-center gap-2 px-2 text-xs font-medium text-[var(--ink-secondary)]">
          <ListTree className="size-4" aria-hidden="true" />
          <span>{structure.rootLabel}</span>
          <span className="ml-auto text-[10px] font-normal text-[var(--ink-tertiary)]">
            схема r{structure.schemaVersion}
          </span>
        </div>
        <DndContext
          sensors={sensors}
          collisionDetection={closestCenter}
          onDragStart={handleDragStart}
          onDragOver={(event) => setDropTargetId(event.over ? String(event.over.id) : undefined)}
          onDragCancel={() => {
            setDraggedNodeId(undefined);
            setDropTargetId(undefined);
          }}
          onDragEnd={handleDragEnd}
          accessibility={{
            screenReaderInstructions: {
              draggable:
                "Нажмите пробел или Enter, затем используйте стрелки для перемещения материала.",
            },
          }}
        >
          <SortableContext items={sortableIds} strategy={verticalListSortingStrategy}>
            <div
              ref={treeViewportRef}
              role="tree"
              aria-label={structure.rootLabel}
              className="overflow-y-auto"
              data-testid="virtualized-project-tree"
              style={{
                height: Math.min(
                  treeViewportHeight,
                  Math.max(treeRowHeight, visibleOutline.length * treeRowHeight),
                ),
              }}
              onScroll={(event) => setTreeScrollTop(event.currentTarget.scrollTop)}
            >
              <div style={{ height: treeVirtualStart * treeRowHeight }} aria-hidden="true" />
              {virtualOutline.map(renderOutlineRow)}
              <div
                style={{ height: (visibleOutline.length - treeVirtualEnd) * treeRowHeight }}
                aria-hidden="true"
              />
            </div>
          </SortableContext>
        </DndContext>
        <Link
          to="/recovery"
          className="mt-3 flex min-h-9 items-center gap-2 border-t border-[var(--border-subtle)] px-3 pt-3 text-[13px] text-[var(--ink-secondary)]"
        >
          <FileAudio className="size-4" aria-hidden="true" />
          Голосовые сессии
        </Link>
        {trashedNodes.length > 0 ? (
          <details className="mt-3 border-t border-[var(--border-subtle)] pt-2">
            <summary className="min-h-9 cursor-pointer px-3 py-2 text-xs text-[var(--ink-tertiary)]">
              Корзина · {trashedNodes.length}
            </summary>
            {trashedNodes.map((node) => (
              <div key={node.id} className="flex min-h-9 items-center gap-2 px-3 text-xs">
                <span className="min-w-0 flex-1 truncate">{node.title}</span>
                <button type="button" onClick={() => restoreMutation.mutate(node.id)}>
                  Вернуть
                </button>
              </div>
            ))}
          </details>
        ) : null}
      </div>
      <div className="border-t border-[var(--border-subtle)] p-2">
        <UtilityLinks projectId={projectId} nodeId={activeNodeId} />
      </div>
    </nav>
  );
}

function SortableTreeRow({
  id,
  title,
  level,
  selected,
  disabled,
  kind,
  canAcceptDrop,
  className,
  paddingLeft,
  children,
}: {
  id: string;
  title: string;
  level: number;
  selected: boolean;
  disabled: boolean;
  kind: string;
  canAcceptDrop: boolean;
  className?: string;
  paddingLeft: number;
  children: ReactNode;
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    setActivatorNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id, disabled });
  return (
    <div
      ref={setNodeRef}
      role="treeitem"
      tabIndex={0}
      aria-level={level}
      aria-selected={selected}
      data-tree-node-kind={kind}
      data-drop-target={canAcceptDrop ? "available" : undefined}
      className={cn(
        "group/tree-row flex min-h-9 items-center rounded-[var(--radius-sm)] text-[13px] text-[var(--ink-secondary)]",
        isDragging && "z-10 opacity-60",
        className,
      )}
      style={{
        paddingLeft: `${paddingLeft}px`,
        transform: CSS.Transform.toString(transform),
        transition,
      }}
    >
      <button
        ref={setActivatorNodeRef}
        type="button"
        className="grid size-7 shrink-0 cursor-grab touch-none place-items-center rounded opacity-0 hover:bg-[var(--surface-subtle)] focus:opacity-100 focus-visible:outline-2 group-hover/tree-row:opacity-100 active:cursor-grabbing"
        aria-label={`Переместить «${title}»`}
        {...attributes}
        {...listeners}
      >
        <GripVertical className="size-3.5" aria-hidden="true" />
      </button>
      {children}
    </div>
  );
}

function UtilityLinks({
  projectId,
  nodeId,
  compact = false,
}: {
  projectId: string;
  nodeId: string;
  compact?: boolean;
}) {
  const links = [
    {
      label: "Версии",
      icon: History,
      to: "/nodes/$nodeId/versions" as const,
      params: { nodeId },
    },
    {
      label: "Словарь",
      icon: SpellCheck2,
      to: "/projects/$projectId/dictionary" as const,
      params: { projectId },
    },
    {
      label: "Правки в тексте",
      icon: MessagesSquare,
      to: "/projects/$projectId/annotations" as const,
      params: { projectId },
    },
    {
      label: "Контекст ассистента",
      icon: Brain,
      to: "/projects/$projectId/assistant-context" as const,
      params: { projectId },
    },
    {
      label: "Восстановление",
      icon: LifeBuoy,
      to: "/recovery" as const,
      params: undefined,
    },
  ];
  return links.map((item) => (
    <Button
      key={item.label}
      asChild
      variant="ghost"
      size={compact ? "icon" : "default"}
      className={compact ? "size-10" : "w-full justify-start"}
    >
      <Link to={item.to} params={item.params as never} aria-label={item.label} title={item.label}>
        <item.icon className="size-4" aria-hidden="true" />
        {!compact ? item.label : null}
      </Link>
    </Button>
  ));
}

function CompactStudioLink({
  projectId,
  node,
  icon: Icon,
  label,
}: {
  projectId: string;
  node?: DocumentNode;
  icon: typeof FileText;
  label: string;
}) {
  if (!node) return null;
  return (
    <Button asChild variant="ghost" size="icon" className="size-10">
      <Link
        to="/projects/$projectId/studio/$nodeId"
        params={{ projectId, nodeId: node.id }}
        aria-label={label}
        title={label}
      >
        <Icon className="size-4" aria-hidden="true" />
      </Link>
    </Button>
  );
}

function TreeAction({
  icon: Icon,
  label,
  destructive = false,
  onClick,
}: {
  icon: typeof FileText;
  label: string;
  destructive?: boolean;
  onClick: () => void;
}) {
  return (
    <DropdownMenuItem destructive={destructive} onSelect={onClick}>
      <Icon className="size-4" aria-hidden="true" />
      {label}
    </DropdownMenuItem>
  );
}
