import {
  applyEdgeChanges,
  applyNodeChanges,
  Background,
  BaseEdge,
  ConnectionMode,
  Controls,
  EdgeLabelRenderer,
  type EdgeProps,
  type EdgeTypes,
  getSmoothStepPath,
  Handle,
  type NodeChange,
  type NodeProps,
  NodeResizer,
  type NodeTypes,
  type OnSelectionChangeParams,
  Panel,
  Position,
  ReactFlow,
  ReactFlowProvider,
  reconnectEdge,
  useReactFlow,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import {
  ArrowDownToLine,
  ArrowUpToLine,
  Copy,
  FilePlus2,
  Focus,
  Group,
  ImagePlus,
  Maximize2,
  Redo2,
  StickyNote,
  Trash2,
  Undo2,
} from "lucide-react";
import {
  type CSSProperties,
  createContext,
  type DragEvent,
  type KeyboardEvent,
  type MouseEvent,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

import {
  type CanvasColor,
  type CanvasDocument,
  type CanvasDocumentPreview,
  type CanvasEdge,
  type CanvasFlowEdge,
  type CanvasFlowNode,
  type CanvasNode,
  canvasToFlow,
  commitCanvasHistory,
  createCanvasHistory,
  createGroupAroundSelection,
  duplicateCanvasSelection,
  flowToCanvas,
  moveGroupContents,
  redoCanvasHistory,
  removeCanvasSelection,
  snapCanvasNodePosition,
  undoCanvasHistory,
} from "~entities/canvas";
import { type DocumentNode, type ProjectMedia, uploadProjectImage } from "~entities/project";
import { cn } from "~shared/lib";
import { Button, useActionDialog, useToast } from "~shared/ui";

interface CanvasBoardProps {
  projectId: string;
  content: CanvasDocument;
  documents: CanvasDocumentPreview[];
  media: ProjectMedia[];
  projectDocuments: DocumentNode[];
  editable?: boolean;
  onChange: (content: CanvasDocument) => void;
  onOpenDocument: (nodeId: string) => void;
}

interface BoardContextValue {
  editable: boolean;
  updateNode: (nodeId: string, update: (node: CanvasNode) => CanvasNode) => void;
  editEdgeLabel: (edgeId: string, label: string) => void;
  openDocument: (nodeId: string) => void;
}

interface CanvasClipboard {
  source: CanvasDocument;
  nodeIds: string[];
  edgeIds: string[];
  pasteCount: number;
}

const BoardContext = createContext<BoardContextValue | null>(null);

const nodeTypes: NodeTypes = {
  "canvas-text": CanvasTextCard,
  "canvas-document": CanvasDocumentCard,
  "canvas-image": CanvasImageCard,
  "canvas-group": CanvasGroupCard,
};

const edgeTypes: EdgeTypes = { "canvas-edge": CanvasConnection };

const canvasFlowStyle = {
  "--xy-controls-button-background-color": "var(--surface-raised)",
  "--xy-controls-button-background-color-hover": "var(--surface-subtle)",
  "--xy-controls-button-color": "var(--ink-secondary)",
  "--xy-controls-button-color-hover": "var(--ink-primary)",
  "--xy-controls-button-border-color": "var(--border-subtle)",
  "--xy-controls-box-shadow": "0 5px 18px var(--shadow-color)",
  "--xy-attribution-background-color": "var(--surface-raised)",
} as CSSProperties;

const defaultCardWidth = 280;
const defaultCardHeight = 180;

const palette: Array<{ value?: CanvasColor; color: string; label: string }> = [
  { color: "var(--surface-raised)", label: "Без цвета" },
  { value: "1", color: "var(--danger-soft)", label: "Красный" },
  { value: "2", color: "var(--warning-soft)", label: "Оранжевый" },
  { value: "3", color: "var(--diff-delete)", label: "Жёлтый" },
  { value: "4", color: "var(--success-soft)", label: "Зелёный" },
  { value: "5", color: "var(--info-soft)", label: "Синий" },
  { value: "6", color: "var(--creative-soft)", label: "Фиолетовый" },
];

const colorValue: Record<string, string> = Object.fromEntries(
  palette.filter((item) => item.value).map((item) => [item.value as string, item.color]),
);

function canvasColor(color?: CanvasColor): string {
  if (!color) return "var(--surface-raised)";
  return color.charCodeAt(0) === 35 ? color : (colorValue[color] ?? "var(--surface-raised)");
}

function useBoardContext(): BoardContextValue {
  const context = useContext(BoardContext);
  if (!context) throw new Error("Canvas cards must be rendered inside CanvasBoard");
  return context;
}

function CardHandles({ interactive = true }: { interactive?: boolean }) {
  const handleClassName = cn(
    "!size-2.5 !bg-[var(--accent)]",
    !interactive && "!pointer-events-none !opacity-0",
  );
  return (
    <>
      <Handle id="top" type="source" position={Position.Top} className={handleClassName} />
      <Handle id="right" type="source" position={Position.Right} className={handleClassName} />
      <Handle id="bottom" type="source" position={Position.Bottom} className={handleClassName} />
      <Handle id="left" type="source" position={Position.Left} className={handleClassName} />
    </>
  );
}

function CardResizer({ selected, group = false }: { selected: boolean; group?: boolean }) {
  const { editable } = useBoardContext();
  return (
    <NodeResizer
      isVisible={editable && selected}
      minWidth={group ? 240 : 180}
      minHeight={group ? 160 : 120}
      lineClassName="!border-[var(--accent)]"
      handleClassName="!size-2.5 !border-2 !border-[var(--surface-raised)] !bg-[var(--accent)]"
    />
  );
}

function CanvasTextCard({ data, selected }: NodeProps<CanvasFlowNode>) {
  const { editable, updateNode } = useBoardContext();
  const item = data.canvasNode;
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(item.type === "text" ? item.text : "");
  const editorRef = useRef<HTMLTextAreaElement>(null);
  useEffect(() => {
    if (item.type === "text" && !editing) setDraft(item.text);
  }, [editing, item]);
  useEffect(() => {
    if (editing) editorRef.current?.focus();
  }, [editing]);
  if (item.type !== "text") return null;

  const save = () => {
    setEditing(false);
    if (draft !== item.text) {
      updateNode(item.id, (node) => (node.type === "text" ? { ...node, text: draft } : node));
    }
  };

  return (
    <article
      data-testid={`canvas-text-${item.id}`}
      className={cn(
        "size-full overflow-hidden rounded-[12px] border bg-[var(--surface-raised)] shadow-[0_5px_18px_var(--shadow-color)]",
        selected
          ? "border-[var(--accent)] ring-2 ring-[var(--accent)]/20"
          : "border-[var(--border-strong)]",
      )}
      style={{ background: canvasColor(item.color) }}
      onDoubleClick={() => editable && setEditing(true)}
    >
      <CardResizer selected={selected} />
      <CardHandles interactive={editable} />
      {editing ? (
        <textarea
          ref={editorRef}
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          onBlur={save}
          onKeyDown={(event) => {
            if (event.key === "Escape") {
              setDraft(item.text);
              setEditing(false);
            }
            if ((event.metaKey || event.ctrlKey) && event.key === "Enter") save();
          }}
          className="nodrag nowheel size-full resize-none bg-transparent p-4 font-mono text-[13px] leading-5 outline-none"
          aria-label="Markdown заметки"
        />
      ) : (
        <div className="nowheel size-full overflow-auto p-4 text-[13px] leading-5">
          {item.text.trim() ? (
            <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              components={{
                h1: ({ children }) => <h3 className="mb-2 text-lg font-semibold">{children}</h3>,
                h2: ({ children }) => <h4 className="mb-2 text-base font-semibold">{children}</h4>,
                p: ({ children }) => <p className="my-2 first:mt-0 last:mb-0">{children}</p>,
                ul: ({ children }) => <ul className="my-2 list-disc pl-5">{children}</ul>,
                ol: ({ children }) => <ol className="my-2 list-decimal pl-5">{children}</ol>,
                a: ({ children, href }) => (
                  <a href={href} target="_blank" rel="noreferrer" className="underline">
                    {children}
                  </a>
                ),
                img: ({ alt }) => (
                  <span className="text-[var(--ink-secondary)]">[{alt || "изображение"}]</span>
                ),
              }}
            >
              {item.text}
            </ReactMarkdown>
          ) : (
            <p className="text-[var(--ink-tertiary)]">Дважды нажмите, чтобы написать заметку…</p>
          )}
        </div>
      )}
    </article>
  );
}

function CanvasDocumentCard({ data, selected }: NodeProps<CanvasFlowNode>) {
  const { editable, openDocument } = useBoardContext();
  const item = data.canvasNode;
  if (item.type !== "file" || item["x-writers-studio"].resource.kind !== "document") return null;
  const document = data.document;
  const status = document ? statusLabel(document.status) : "Недоступен";
  return (
    <article
      data-testid={`canvas-document-${item.id}`}
      className={cn(
        "size-full overflow-hidden rounded-[12px] border bg-[var(--surface-raised)] shadow-[0_5px_18px_var(--shadow-color)]",
        selected
          ? "border-[var(--accent)] ring-2 ring-[var(--accent)]/20"
          : "border-[var(--border-strong)]",
      )}
      style={{ background: canvasColor(item.color) }}
      onDoubleClick={() => document && openDocument(document.id)}
    >
      <CardResizer selected={selected} />
      <CardHandles interactive={editable} />
      <div className="flex h-10 items-center gap-2 border-b border-[var(--border-subtle)] px-3">
        <FilePlus2 className="size-4 shrink-0" aria-hidden="true" />
        <strong className="truncate text-sm">{document?.title ?? "Документ недоступен"}</strong>
      </div>
      <div className="p-3">
        <span className="rounded-full bg-[var(--surface-on-solid)] px-2 py-1 text-[10px] font-semibold uppercase tracking-wide">
          {status}
        </span>
        <p className="mt-3 line-clamp-5 text-[12px] leading-5 text-[var(--ink-secondary)]">
          {document?.plainText.trim() ||
            (editable
              ? "Пустой документ. Дважды нажмите, чтобы открыть Studio."
              : "Пустой документ. Нажмите, чтобы открыть Studio.")}
        </p>
      </div>
    </article>
  );
}

function CanvasImageCard({ data, selected }: NodeProps<CanvasFlowNode>) {
  const { editable } = useBoardContext();
  const item = data.canvasNode;
  if (item.type !== "file" || item["x-writers-studio"].resource.kind !== "media") return null;
  return (
    <figure
      data-testid={`canvas-image-${item.id}`}
      className={cn(
        "size-full overflow-hidden rounded-[12px] border bg-[var(--surface-raised)] shadow-[0_5px_18px_var(--shadow-color)]",
        selected
          ? "border-[var(--accent)] ring-2 ring-[var(--accent)]/20"
          : "border-[var(--border-strong)]",
      )}
      style={{ background: canvasColor(item.color) }}
    >
      <CardResizer selected={selected} />
      <CardHandles interactive={editable} />
      {data.media ? (
        <img
          src={data.media.contentUrl}
          alt={data.media.originalName}
          draggable={false}
          className="size-full object-cover"
        />
      ) : (
        <div className="grid size-full place-items-center p-4 text-center text-xs text-[var(--ink-secondary)]">
          Изображение недоступно
        </div>
      )}
      {data.media ? (
        <figcaption className="absolute inset-x-0 bottom-0 truncate bg-[var(--surface-overlay)] px-3 py-2 text-xs text-[var(--ink-on-solid)]">
          {data.media.originalName}
        </figcaption>
      ) : null}
    </figure>
  );
}

function CanvasGroupCard({ data, selected }: NodeProps<CanvasFlowNode>) {
  const { editable, updateNode } = useBoardContext();
  const { requestInput } = useActionDialog();
  const item = data.canvasNode;
  if (item.type !== "group") return null;
  const rename = () => {
    if (!editable) return;
    void requestInput({
      title: "Название группы",
      label: "Название",
      defaultValue: item.label ?? "",
      confirmLabel: "Сохранить",
    }).then((label) => {
      if (label !== undefined) {
        updateNode(item.id, (node) =>
          node.type === "group" ? { ...node, label: label.trim() } : node,
        );
      }
    });
  };
  return (
    <section
      data-testid={`canvas-group-${item.id}`}
      className={cn(
        "size-full rounded-[16px] border-2 border-dashed bg-transparent",
        selected ? "border-[var(--accent)]" : "border-[var(--border-strong)]",
      )}
      style={{ backgroundColor: `color-mix(in srgb, ${canvasColor(item.color)} 35%, transparent)` }}
    >
      <CardResizer selected={selected} group />
      <CardHandles interactive={editable} />
      <button
        type="button"
        disabled={!editable}
        className="nodrag absolute -top-8 left-2 max-w-[calc(100%-16px)] truncate rounded-full bg-[var(--surface-raised)] px-3 py-1 text-xs font-semibold shadow-sm disabled:cursor-default"
        onDoubleClick={rename}
        onKeyDown={(event) => {
          if (event.key === "Enter") rename();
        }}
        aria-label={`Переименовать группу «${item.label || "Группа"}»`}
      >
        {item.label || "Группа"}
      </button>
    </section>
  );
}

function CanvasConnection(props: EdgeProps<CanvasFlowEdge>) {
  const { editable, editEdgeLabel } = useBoardContext();
  const edge = props.data?.canvasEdge;
  const [path, labelX, labelY] = getSmoothStepPath(props);
  const stroke = canvasColor(edge?.color);
  return (
    <>
      <BaseEdge
        path={path}
        markerStart={props.markerStart}
        markerEnd={props.markerEnd}
        style={{
          stroke: edge?.color ? stroke : "var(--ink-secondary)",
          strokeWidth: props.selected ? 3 : 2,
        }}
      />
      {(edge?.label || (editable && props.selected)) && (
        <EdgeLabelRenderer>
          <button
            type="button"
            className="nodrag nopan absolute max-w-40 -translate-x-1/2 -translate-y-1/2 truncate rounded-full border border-[var(--border-subtle)] bg-[var(--surface-raised)] px-2.5 py-1 text-[11px] shadow-sm"
            style={{ transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)` }}
            onDoubleClick={() => editable && editEdgeLabel(props.id, edge?.label ?? "")}
          >
            {edge?.label || "Добавить подпись"}
          </button>
        </EdgeLabelRenderer>
      )}
    </>
  );
}

function statusLabel(status: CanvasDocumentPreview["status"]): string {
  return { idea: "Идея", draft: "Черновик", editing: "Редактура", ready: "Готово" }[status];
}

function isFormTarget(target: EventTarget | null): boolean {
  return (
    target instanceof HTMLInputElement ||
    target instanceof HTMLTextAreaElement ||
    (target instanceof HTMLElement && target.isContentEditable)
  );
}

function CanvasBoardInner({
  projectId,
  content,
  documents,
  media,
  projectDocuments,
  editable = true,
  onChange,
  onOpenDocument,
}: CanvasBoardProps) {
  const reactFlow = useReactFlow<CanvasFlowNode, CanvasFlowEdge>();
  const { requestInput, requestSelect } = useActionDialog();
  const { showToast } = useToast();
  const [desktop, setDesktop] = useState(() => window.matchMedia("(min-width: 1024px)").matches);
  const canEdit = editable && desktop;
  const [history, setHistory] = useState(() => createCanvasHistory(content));
  const historyRef = useRef(history);
  const [initialFlow] = useState(() => canvasToFlow(content, documents, media));
  const [nodes, setNodes] = useState<CanvasFlowNode[]>(initialFlow.nodes);
  const [edges, setEdges] = useState<CanvasFlowEdge[]>(initialFlow.edges);
  const nodesRef = useRef(nodes);
  const edgesRef = useRef(edges);
  const [selectedNodeIds, setSelectedNodeIds] = useState<string[]>([]);
  const [selectedEdgeIds, setSelectedEdgeIds] = useState<string[]>([]);
  const [localMedia, setLocalMedia] = useState(media);
  const dragStart = useRef<{ id: string; x: number; y: number } | undefined>(undefined);
  const clipboard = useRef<CanvasClipboard | undefined>(undefined);
  const fileInput = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const query = window.matchMedia("(min-width: 1024px)");
    const update = () => setDesktop(query.matches);
    query.addEventListener("change", update);
    return () => query.removeEventListener("change", update);
  }, []);

  useEffect(() => {
    const flow = canvasToFlow(historyRef.current.present, documents, localMedia);
    nodesRef.current = flow.nodes;
    edgesRef.current = flow.edges;
    setNodes(flow.nodes);
    setEdges(flow.edges);
  }, [documents, localMedia]);

  const syncFlow = useCallback(
    (document: CanvasDocument, selectedIds: string[] = []) => {
      const selected = new Set(selectedIds);
      const flow = canvasToFlow(document, documents, localMedia);
      flow.nodes = flow.nodes.map((node) => ({ ...node, selected: selected.has(node.id) }));
      nodesRef.current = flow.nodes;
      edgesRef.current = flow.edges;
      setNodes(flow.nodes);
      setEdges(flow.edges);
    },
    [documents, localMedia],
  );

  const applyHistory = useCallback(
    (next: typeof history) => {
      historyRef.current = next;
      setHistory(next);
      syncFlow(next.present);
      onChange(next.present);
    },
    [onChange, syncFlow],
  );

  const commit = useCallback(
    (document: CanvasDocument, selectedIds: string[] = selectedNodeIds) => {
      const next = commitCanvasHistory(historyRef.current, document);
      if (next === historyRef.current) return;
      historyRef.current = next;
      setHistory(next);
      syncFlow(next.present, selectedIds);
      onChange(next.present);
    },
    [onChange, selectedNodeIds, syncFlow],
  );

  const commitFlow = useCallback(
    (nextNodes = nodesRef.current, nextEdges = edgesRef.current) => {
      commit(flowToCanvas(nextNodes, nextEdges, historyRef.current.present));
    },
    [commit],
  );

  const updateNode = useCallback(
    (nodeId: string, update: (node: CanvasNode) => CanvasNode) => {
      commit(
        {
          ...historyRef.current.present,
          nodes: historyRef.current.present.nodes.map((node) =>
            node.id === nodeId ? update(node) : node,
          ),
        },
        [nodeId],
      );
    },
    [commit],
  );

  const editEdgeLabel = useCallback(
    (edgeId: string, current: string) => {
      void requestInput({
        title: "Подпись связи",
        label: "Подпись",
        defaultValue: current,
        confirmLabel: "Сохранить",
      }).then((label) => {
        if (label === undefined) return;
        commit({
          ...historyRef.current.present,
          edges: historyRef.current.present.edges.map((edge) =>
            edge.id === edgeId ? { ...edge, label: label.trim() || undefined } : edge,
          ),
        });
      });
    },
    [commit, requestInput],
  );

  const addText = useCallback(
    (position?: { x: number; y: number }) => {
      if (!canEdit) return;
      const center =
        position ??
        reactFlow.screenToFlowPosition({ x: window.innerWidth / 2, y: window.innerHeight / 2 });
      const node: CanvasNode = {
        id: crypto.randomUUID(),
        type: "text",
        x: Math.round(center.x - defaultCardWidth / 2),
        y: Math.round(center.y - defaultCardHeight / 2),
        width: defaultCardWidth,
        height: defaultCardHeight,
        text: "# Новая заметка\n\nЗапишите идею…",
      };
      commit(
        { ...historyRef.current.present, nodes: [...historyRef.current.present.nodes, node] },
        [node.id],
      );
      setSelectedNodeIds([node.id]);
    },
    [canEdit, commit, reactFlow],
  );

  const addDocument = useCallback(() => {
    if (!canEdit) return;
    const available = projectDocuments.filter((item) => !item.trashedAt);
    if (available.length === 0) {
      showToast({ title: "В проекте пока нет документов" });
      return;
    }
    void requestSelect({
      title: "Добавить документ",
      label: "Документ проекта",
      options: available.map((item) => ({ value: item.id, label: item.title })),
      confirmLabel: "Добавить",
    }).then((nodeId) => {
      const document = available.find((item) => item.id === nodeId);
      if (!document) return;
      const center = reactFlow.screenToFlowPosition({
        x: window.innerWidth / 2,
        y: window.innerHeight / 2,
      });
      const node: CanvasNode = {
        id: crypto.randomUUID(),
        type: "file",
        file: `writers-studio://documents/${document.id}`,
        x: Math.round(center.x - defaultCardWidth / 2),
        y: Math.round(center.y - defaultCardHeight / 2),
        width: defaultCardWidth,
        height: defaultCardHeight,
        "x-writers-studio": { resource: { kind: "document", id: document.id } },
      };
      commit(
        { ...historyRef.current.present, nodes: [...historyRef.current.present.nodes, node] },
        [node.id],
      );
      setSelectedNodeIds([node.id]);
    });
  }, [canEdit, commit, projectDocuments, reactFlow, requestSelect, showToast]);

  const addDocumentAt = useCallback(
    (nodeId: string, position: { x: number; y: number }) => {
      const document = projectDocuments.find((item) => item.id === nodeId && !item.trashedAt);
      if (!document || !canEdit) return;
      const node: CanvasNode = {
        id: crypto.randomUUID(),
        type: "file",
        file: `writers-studio://documents/${document.id}`,
        x: Math.round(position.x - defaultCardWidth / 2),
        y: Math.round(position.y - defaultCardHeight / 2),
        width: defaultCardWidth,
        height: defaultCardHeight,
        "x-writers-studio": { resource: { kind: "document", id: document.id } },
      };
      commit(
        { ...historyRef.current.present, nodes: [...historyRef.current.present.nodes, node] },
        [node.id],
      );
      setSelectedNodeIds([node.id]);
    },
    [canEdit, commit, projectDocuments],
  );

  const addImage = useCallback(
    async (file: File, position?: { x: number; y: number }) => {
      if (!canEdit) return;
      try {
        const response = await uploadProjectImage(projectId, file);
        setLocalMedia((current) => [
          ...current.filter((item) => item.id !== response.media.id),
          response.media,
        ]);
        const center =
          position ??
          reactFlow.screenToFlowPosition({ x: window.innerWidth / 2, y: window.innerHeight / 2 });
        const ratio = Math.max(0.35, Math.min(2.5, response.media.width / response.media.height));
        const width = 300;
        const node: CanvasNode = {
          id: crypto.randomUUID(),
          type: "file",
          file: `writers-studio://media/${response.media.id}`,
          x: Math.round(center.x - width / 2),
          y: Math.round(center.y - width / ratio / 2),
          width,
          height: Math.round(width / ratio),
          "x-writers-studio": { resource: { kind: "media", id: response.media.id } },
        };
        commit(
          { ...historyRef.current.present, nodes: [...historyRef.current.present.nodes, node] },
          [node.id],
        );
        setSelectedNodeIds([node.id]);
      } catch (error) {
        showToast({
          title: "Не удалось добавить изображение",
          description: error instanceof Error ? error.message : undefined,
        });
      }
    },
    [canEdit, commit, projectId, reactFlow, showToast],
  );

  const removeSelection = useCallback(() => {
    if (!canEdit || (selectedNodeIds.length === 0 && selectedEdgeIds.length === 0)) return;
    commit(removeCanvasSelection(historyRef.current.present, selectedNodeIds, selectedEdgeIds), []);
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
  }, [canEdit, commit, selectedEdgeIds, selectedNodeIds]);

  const copySelection = useCallback(
    (cut = false) => {
      if (selectedNodeIds.length === 0 && selectedEdgeIds.length === 0) return;
      clipboard.current = {
        source: historyRef.current.present,
        nodeIds: selectedNodeIds,
        edgeIds: selectedEdgeIds,
        pasteCount: 0,
      };
      if (cut && canEdit) removeSelection();
    },
    [canEdit, removeSelection, selectedEdgeIds, selectedNodeIds],
  );

  const pasteSelection = useCallback(() => {
    if (!canEdit || !clipboard.current) return;
    clipboard.current.pasteCount += 1;
    const source = clipboard.current;
    const duplicated = duplicateCanvasSelection(
      source.source,
      source.nodeIds,
      source.edgeIds,
      () => crypto.randomUUID(),
      40 * source.pasteCount,
    );
    const pastedNodes = duplicated.document.nodes.slice(source.source.nodes.length);
    const pastedEdges = duplicated.document.edges.slice(source.source.edges.length);
    const next = {
      ...historyRef.current.present,
      nodes: [...historyRef.current.present.nodes, ...pastedNodes],
      edges: [...historyRef.current.present.edges, ...pastedEdges],
    };
    commit(
      next,
      pastedNodes.map((item) => item.id),
    );
    setSelectedNodeIds(pastedNodes.map((item) => item.id));
    setSelectedEdgeIds(pastedEdges.map((item) => item.id));
  }, [canEdit, commit]);

  const duplicateSelection = useCallback(() => {
    if (!canEdit || selectedNodeIds.length === 0) return;
    const result = duplicateCanvasSelection(
      historyRef.current.present,
      selectedNodeIds,
      selectedEdgeIds,
      () => crypto.randomUUID(),
    );
    commit(result.document, result.nodeIds);
    setSelectedNodeIds(result.nodeIds);
    setSelectedEdgeIds(result.edgeIds);
  }, [canEdit, commit, selectedEdgeIds, selectedNodeIds]);

  const groupSelection = useCallback(() => {
    if (!canEdit || selectedNodeIds.length === 0) return;
    const result = createGroupAroundSelection(historyRef.current.present, selectedNodeIds, () =>
      crypto.randomUUID(),
    );
    if (!result.groupId) return;
    commit(result.document, [result.groupId, ...selectedNodeIds]);
    setSelectedNodeIds([result.groupId, ...selectedNodeIds]);
  }, [canEdit, commit, selectedNodeIds]);

  const setSelectionColor = useCallback(
    (color?: CanvasColor) => {
      if (!canEdit) return;
      const nodeIds = new Set(selectedNodeIds);
      const edgeIds = new Set(selectedEdgeIds);
      commit({
        ...historyRef.current.present,
        nodes: historyRef.current.present.nodes.map((item) =>
          nodeIds.has(item.id) ? { ...item, color } : item,
        ) as CanvasNode[],
        edges: historyRef.current.present.edges.map((item) =>
          edgeIds.has(item.id) ? { ...item, color } : item,
        ),
      });
    },
    [canEdit, commit, selectedEdgeIds, selectedNodeIds],
  );

  const changeZOrder = useCallback(
    (front: boolean) => {
      if (!canEdit || selectedNodeIds.length === 0) return;
      const selected = new Set(selectedNodeIds);
      const groups = historyRef.current.present.nodes.filter((item) => item.type === "group");
      const cards = historyRef.current.present.nodes.filter((item) => item.type !== "group");
      const reorder = (items: CanvasNode[]) => [
        ...items.filter((item) => (front ? !selected.has(item.id) : selected.has(item.id))),
        ...items.filter((item) => (front ? selected.has(item.id) : !selected.has(item.id))),
      ];
      commit({ ...historyRef.current.present, nodes: [...reorder(groups), ...reorder(cards)] });
    },
    [canEdit, commit, selectedNodeIds],
  );

  const handleUndo = useCallback(() => {
    const next = undoCanvasHistory(historyRef.current);
    if (next !== historyRef.current) applyHistory(next);
  }, [applyHistory]);
  const handleRedo = useCallback(() => {
    const next = redoCanvasHistory(historyRef.current);
    if (next !== historyRef.current) applyHistory(next);
  }, [applyHistory]);

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (isFormTarget(event.target)) return;
    const command = event.metaKey || event.ctrlKey;
    if (command && event.key.toLowerCase() === "z") {
      event.preventDefault();
      event.shiftKey ? handleRedo() : handleUndo();
    } else if (command && event.key.toLowerCase() === "c") {
      event.preventDefault();
      copySelection();
    } else if (command && event.key.toLowerCase() === "x") {
      event.preventDefault();
      copySelection(true);
    } else if (command && event.key.toLowerCase() === "v") {
      event.preventDefault();
      pasteSelection();
    } else if (command && event.key.toLowerCase() === "d") {
      event.preventDefault();
      duplicateSelection();
    } else if (command && event.key.toLowerCase() === "a") {
      event.preventDefault();
      setSelectedNodeIds(nodesRef.current.map((item) => item.id));
      setNodes((current) => current.map((item) => ({ ...item, selected: true })));
    } else if (event.key === "Delete" || event.key === "Backspace") {
      event.preventDefault();
      removeSelection();
    }
  };

  const onDrop = (event: DragEvent<HTMLDivElement>) => {
    if (!canEdit) return;
    event.preventDefault();
    const position = reactFlow.screenToFlowPosition({ x: event.clientX, y: event.clientY });
    const rawDocument = event.dataTransfer.getData("application/x-writers-studio-document");
    if (rawDocument) {
      try {
        const payload = JSON.parse(rawDocument) as { nodeId?: string };
        if (payload.nodeId) addDocumentAt(payload.nodeId, position);
      } catch {
        showToast({ title: "Не удалось прочитать перетаскиваемый документ" });
      }
      return;
    }
    const image = [...event.dataTransfer.files].find((file) => file.type.startsWith("image/"));
    if (image) void addImage(image, position);
  };

  const contextValue = useMemo<BoardContextValue>(
    () => ({
      editable: canEdit,
      updateNode,
      editEdgeLabel,
      openDocument: onOpenDocument,
    }),
    [canEdit, editEdgeLabel, onOpenDocument, updateNode],
  );
  const handleSelectionChange = useCallback(
    ({
      nodes: selectedNodes,
      edges: selectedEdges,
    }: OnSelectionChangeParams<CanvasFlowNode, CanvasFlowEdge>) => {
      const nextNodeIds = selectedNodes.map((item) => item.id);
      const nextEdgeIds = selectedEdges.map((item) => item.id);
      setSelectedNodeIds((current) =>
        current.length === nextNodeIds.length &&
        current.every((id, index) => id === nextNodeIds[index])
          ? current
          : nextNodeIds,
      );
      setSelectedEdgeIds((current) =>
        current.length === nextEdgeIds.length &&
        current.every((id, index) => id === nextEdgeIds[index])
          ? current
          : nextEdgeIds,
      );
    },
    [],
  );

  return (
    <BoardContext.Provider value={contextValue}>
      <div
        data-testid="canvas-board"
        role="application"
        aria-label="Canvas-доска"
        className="relative size-full min-h-0 bg-[var(--surface-app)] outline-none"
        onKeyDown={onKeyDown}
        onDragOver={(event) => canEdit && event.preventDefault()}
        onDrop={onDrop}
      >
        <ReactFlow<CanvasFlowNode, CanvasFlowEdge>
          className="canvas-flow"
          style={canvasFlowStyle}
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          edgeTypes={edgeTypes}
          onlyRenderVisibleElements
          snapToGrid
          snapGrid={[20, 20]}
          connectionMode={ConnectionMode.Loose}
          minZoom={0.12}
          maxZoom={3}
          fitView
          fitViewOptions={{ padding: 0.2 }}
          nodesDraggable={canEdit}
          nodesConnectable={canEdit}
          nodesFocusable
          edgesFocusable
          elementsSelectable
          selectionOnDrag={canEdit}
          panOnDrag={canEdit ? [1, 2] : true}
          deleteKeyCode={null}
          onNodesChange={(changes: NodeChange<CanvasFlowNode>[]) => {
            const positionChanges = changes.filter(
              (change) => change.type === "position" && change.position,
            );
            const adjustedChanges =
              positionChanges.length === 1
                ? changes.map((change) => {
                    if (change.type !== "position" || !change.position) return change;
                    const moving = nodesRef.current.find((node) => node.id === change.id);
                    const movingItem = moving?.data.canvasNode;
                    if (!moving || !movingItem || movingItem.type === "group") return change;
                    const position = snapCanvasNodePosition(
                      {
                        id: moving.id,
                        x: change.position.x,
                        y: change.position.y,
                        width: movingItem.width,
                        height: movingItem.height,
                      },
                      nodesRef.current
                        .filter(
                          (node) => node.id !== moving.id && node.data.canvasNode.type !== "group",
                        )
                        .map((node) => ({
                          id: node.id,
                          x: node.position.x,
                          y: node.position.y,
                          width: node.data.canvasNode.width,
                          height: node.data.canvasNode.height,
                        })),
                    );
                    return { ...change, position };
                  })
                : changes;
            const next = applyNodeChanges(adjustedChanges, nodesRef.current);
            nodesRef.current = next;
            setNodes(next);
            if (
              changes.some(
                (change) =>
                  change.type === "dimensions" && "resizing" in change && change.resizing === false,
              )
            ) {
              commitFlow(next, edgesRef.current);
            }
          }}
          onEdgesChange={(changes) => {
            const next = applyEdgeChanges(changes, edgesRef.current);
            edgesRef.current = next;
            setEdges(next);
          }}
          onNodeDragStart={(_, node) => {
            dragStart.current = { id: node.id, x: node.position.x, y: node.position.y };
          }}
          onNodeDragStop={(_, node) => {
            const original = historyRef.current.present.nodes.find((item) => item.id === node.id);
            if (original?.type === "group" && dragStart.current?.id === node.id) {
              const delta = {
                x: node.position.x - dragStart.current.x,
                y: node.position.y - dragStart.current.y,
              };
              const moved = moveGroupContents(historyRef.current.present, node.id, delta);
              const flowDocument = flowToCanvas(
                nodesRef.current,
                edgesRef.current,
                historyRef.current.present,
              );
              const movedGroup = flowDocument.nodes.find((item) => item.id === node.id);
              commit({
                ...moved,
                edges: flowDocument.edges,
                nodes: moved.nodes.map((item) =>
                  item.id === node.id && movedGroup ? movedGroup : item,
                ),
              });
            } else {
              commitFlow();
            }
            dragStart.current = undefined;
          }}
          onConnect={(connection) => {
            if (!canEdit || !connection.source || !connection.target) return;
            const id = crypto.randomUUID();
            const canvasEdge: CanvasEdge = {
              id,
              fromNode: connection.source,
              fromSide: connection.sourceHandle as CanvasEdge["fromSide"],
              fromEnd: "none",
              toNode: connection.target,
              toSide: connection.targetHandle as CanvasEdge["toSide"],
              toEnd: "arrow",
            };
            const next = [
              ...edgesRef.current,
              {
                id,
                type: "canvas-edge" as const,
                source: connection.source,
                target: connection.target,
                sourceHandle: connection.sourceHandle,
                targetHandle: connection.targetHandle,
                markerEnd: { type: "arrowclosed" as const },
                data: { canvasEdge },
              },
            ];
            edgesRef.current = next;
            setEdges(next);
            commitFlow(nodesRef.current, next);
          }}
          onReconnect={(_, connection) => {
            const oldEdge = edgesRef.current.find((item) => item.id === _.id);
            if (!oldEdge) return;
            const next = reconnectEdge(oldEdge, connection, edgesRef.current);
            edgesRef.current = next;
            setEdges(next);
            commitFlow(nodesRef.current, next);
          }}
          onSelectionChange={handleSelectionChange}
          onNodeClick={(_event: MouseEvent, node) => {
            if (canEdit) return;
            const item = node.data.canvasNode;
            if (item.type === "file" && item["x-writers-studio"].resource.kind === "document") {
              onOpenDocument(item["x-writers-studio"].resource.id);
            }
          }}
          onNodeDoubleClick={(_event: MouseEvent, node) => {
            const item = node.data.canvasNode;
            if (item.type === "file" && item["x-writers-studio"].resource.kind === "document") {
              onOpenDocument(item["x-writers-studio"].resource.id);
            }
          }}
        >
          <Background gap={20} size={1} color="var(--border-subtle)" />
          <Controls
            showInteractive={false}
            position="bottom-right"
            className="!overflow-hidden !rounded-[var(--radius-sm)] !border !border-[var(--border-subtle)]"
          />
          <Panel
            position="top-left"
            className="!m-3 flex max-w-[calc(100vw-32px)] items-center gap-1 overflow-x-auto rounded-[12px] border border-[var(--border-subtle)] bg-[var(--surface-raised)] p-1.5 shadow-lg lg:max-w-[calc(100%-180px)]"
          >
            {canEdit ? (
              <>
                <ToolButton icon={StickyNote} label="Заметка" onClick={() => addText()} />
                <ToolButton icon={FilePlus2} label="Документ" onClick={addDocument} />
                <ToolButton
                  icon={ImagePlus}
                  label="Изображение"
                  onClick={() => fileInput.current?.click()}
                />
                <span className="mx-1 h-6 w-px shrink-0 bg-[var(--border-subtle)]" />
                <ToolButton
                  icon={Undo2}
                  label="Отменить"
                  onClick={handleUndo}
                  disabled={history.past.length === 0}
                />
                <ToolButton
                  icon={Redo2}
                  label="Повторить"
                  onClick={handleRedo}
                  disabled={history.future.length === 0}
                />
                <ToolButton
                  icon={Group}
                  label="Группа"
                  onClick={groupSelection}
                  disabled={selectedNodeIds.length === 0}
                />
                <ToolButton
                  icon={Copy}
                  label="Дублировать"
                  onClick={duplicateSelection}
                  disabled={selectedNodeIds.length === 0}
                />
                <ToolButton
                  icon={Trash2}
                  label="Удалить"
                  onClick={removeSelection}
                  disabled={selectedNodeIds.length === 0 && selectedEdgeIds.length === 0}
                />
                <ToolButton
                  icon={ArrowUpToLine}
                  label="На передний план"
                  onClick={() => changeZOrder(true)}
                  disabled={selectedNodeIds.length === 0}
                />
                <ToolButton
                  icon={ArrowDownToLine}
                  label="На задний план"
                  onClick={() => changeZOrder(false)}
                  disabled={selectedNodeIds.length === 0}
                />
                {(selectedNodeIds.length > 0 || selectedEdgeIds.length > 0) && (
                  <fieldset className="ml-1 flex items-center gap-1 border-l border-[var(--border-subtle)] pl-2">
                    <legend className="sr-only">Цвет выделения</legend>
                    {palette.map((item) => (
                      <button
                        key={item.label}
                        type="button"
                        title={item.label}
                        aria-label={item.label}
                        className="size-5 shrink-0 rounded-full border border-[var(--border-strong)] transition-transform hover:scale-110"
                        style={{ backgroundColor: item.color }}
                        onClick={() => setSelectionColor(item.value)}
                      />
                    ))}
                  </fieldset>
                )}
              </>
            ) : (
              <span className="px-2 text-xs text-[var(--ink-secondary)]">
                На мобильном доступен просмотр доски
              </span>
            )}
            <span className="mx-1 h-6 w-px shrink-0 bg-[var(--border-subtle)]" />
            <ToolButton
              icon={Maximize2}
              label="Показать всё"
              onClick={() => void reactFlow.fitView({ padding: 0.2, duration: 250 })}
            />
            <ToolButton
              icon={Focus}
              label="К выделению"
              disabled={selectedNodeIds.length === 0}
              onClick={() =>
                void reactFlow.fitView({
                  nodes: nodesRef.current.filter((item) => selectedNodeIds.includes(item.id)),
                  padding: 0.35,
                  duration: 250,
                })
              }
            />
          </Panel>
        </ReactFlow>
        <input
          ref={fileInput}
          type="file"
          accept="image/png,image/jpeg,image/gif"
          className="hidden"
          onChange={(event) => {
            const file = event.target.files?.[0];
            if (file) void addImage(file);
            event.target.value = "";
          }}
        />
      </div>
    </BoardContext.Provider>
  );
}

function ToolButton({
  icon: Icon,
  label,
  onClick,
  disabled,
}: {
  icon: typeof StickyNote;
  label: string;
  onClick: () => void;
  disabled?: boolean;
}) {
  return (
    <Button
      type="button"
      variant="ghost"
      size="compact"
      className="min-h-8 shrink-0 px-2"
      title={label}
      aria-label={label}
      disabled={disabled}
      onClick={onClick}
    >
      <Icon className="size-4" aria-hidden="true" />
      <span className="hidden 2xl:inline">{label}</span>
    </Button>
  );
}

export function CanvasBoard(props: CanvasBoardProps) {
  return (
    <ReactFlowProvider>
      <CanvasBoardInner {...props} />
    </ReactFlowProvider>
  );
}
