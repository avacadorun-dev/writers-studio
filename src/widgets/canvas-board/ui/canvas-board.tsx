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
  MiniMap,
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
  ViewportPortal,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import {
  Activity,
  AlignCenter,
  ArrowDownToLine,
  ArrowRight,
  ArrowUpToLine,
  ChevronLeft,
  ChevronRight,
  Circle,
  Columns3,
  Copy,
  Diamond,
  Download,
  Eraser,
  FileOutput,
  FilePlus2,
  Focus,
  GitBranch,
  Group,
  Hand,
  Highlighter,
  ImagePlus,
  Layers3,
  LayoutGrid,
  LayoutTemplate,
  Link2,
  ListTree,
  Maximize2,
  Minus,
  MousePointer2,
  PanelsTopLeft,
  PenLine,
  Play,
  Redo2,
  RotateCw,
  Route,
  Search,
  Square,
  StickyNote,
  Tags,
  TimerReset,
  Trash2,
  Undo2,
  Workflow,
  X,
} from "lucide-react";
import {
  type ClipboardEvent,
  type CSSProperties,
  createContext,
  type DragEvent,
  type KeyboardEvent,
  type MouseEvent,
  type PointerEvent as ReactPointerEvent,
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
  advanceCanvasPlaythrough,
  alignCanvasSelection,
  analyzeCanvasGraph,
  appendSampledPoint,
  type CanvasColor,
  type CanvasDocument,
  type CanvasDocumentPreview,
  type CanvasEdge,
  type CanvasFlowEdge,
  type CanvasFlowNode,
  type CanvasGraphDiagnostic,
  type CanvasLayoutMode,
  type CanvasNode,
  type CanvasPlaythroughProfile,
  type CanvasPlaythroughState,
  type CanvasPoint,
  type CanvasPromotion,
  type CanvasShapeElement,
  type CanvasSnapResult,
  type CanvasStrokeElement,
  type CanvasStrokePoint,
  type CanvasTypeSchema,
  type CanvasViewMode,
  canvasExportBounds,
  canvasExportDimensions,
  canvasExportFileName,
  canvasExportNodeIds,
  canvasFocusRemainingSeconds,
  canvasFocusTimerEnd,
  canvasPlaythroughChoices,
  canvasPlaythroughStarts,
  canvasTemplates,
  canvasToFlow,
  commitCanvasHistory,
  createCanvasHistory,
  createCanvasTemplate,
  createFrameAroundSelection,
  createGroupAroundSelection,
  createShapeNode,
  createStrokeNode,
  distributeCanvasSelection,
  duplicateCanvasSelection,
  eraseCanvasElements,
  fetchCanvasLinkPreviews,
  flowToCanvas,
  formatCanvasFocusTime,
  isSpatialGroup,
  listCanvasObjects,
  moveGroupContents,
  orderedCanvasFrames,
  projectCanvasView,
  redoCanvasHistory,
  removeCanvasSelection,
  requestCanvasLayout,
  rotateCanvasSelection,
  searchCanvasObjects,
  snapCanvasNodeWithGuides,
  startCanvasPlaythrough,
  strokeElementPath,
  tidyCanvasSelection,
  undoCanvasHistory,
} from "~entities/canvas";
import { type DocumentNode, type ProjectMedia, uploadProjectMedia } from "~entities/project";
import { cn } from "~shared/lib";
import { Button, useActionDialog, useToast } from "~shared/ui";
import { exportCanvasViewport } from "../lib/export-canvas";
import { useDismissiblePanel } from "../model/use-dismissible-panel";
import { useToolRailMenu } from "../model/use-tool-rail-menu";

interface CanvasBoardProps {
  projectId: string;
  title?: string;
  content: CanvasDocument;
  documents: CanvasDocumentPreview[];
  media: ProjectMedia[];
  projectDocuments: DocumentNode[];
  typeSchema: CanvasTypeSchema;
  editable?: boolean;
  promotionEnabled?: boolean;
  initialFocusNodeId?: string;
  onChange: (content: CanvasDocument) => void;
  onOpenDocument: (nodeId: string) => void;
  onOpenCanvas?: (canvasId: string) => void;
  onPromoteNode?: (nodeId: string, suggestedTitle: string) => Promise<CanvasPromotion | undefined>;
  onAssistantContextChange?: (context: {
    selectionIds: string[];
    viewport: { x: number; y: number; zoom: number };
  }) => void;
}

interface BoardContextValue {
  editable: boolean;
  typeSchema: CanvasTypeSchema;
  updateNode: (nodeId: string, update: (node: CanvasNode) => CanvasNode) => void;
  editEdgeLabel: (edgeId: string, label: string) => void;
}

type CanvasTool =
  | "select"
  | "hand"
  | "connector"
  | "note"
  | "pen"
  | "highlighter"
  | "eraser"
  | CanvasShapeElement["shape"];

type CanvasToolMenu = "create" | "draw" | "arrange" | "explore" | "selection";

interface CanvasGesture {
  pointerId: number;
  pointerType: string;
  tool: Exclude<CanvasTool, "select" | "hand" | "connector" | "note">;
  points: CanvasPoint[];
}

interface CanvasClipboard {
  source: CanvasDocument;
  nodeIds: string[];
  edgeIds: string[];
  pasteCount: number;
}

interface CanvasNodeMenu {
  nodeId: string;
  x: number;
  y: number;
}

const BoardContext = createContext<BoardContextValue | null>(null);

const nodeTypes: NodeTypes = {
  "canvas-text": CanvasTextCard,
  "canvas-document": CanvasDocumentCard,
  "canvas-image": CanvasImageCard,
  "canvas-pdf": CanvasPDFCard,
  "canvas-media": CanvasMediaCard,
  "canvas-external-file": CanvasExternalFileCard,
  "canvas-group": CanvasGroupCard,
  "canvas-stroke": CanvasStrokeCard,
  "canvas-shape": CanvasShapeCard,
  "canvas-frame": CanvasFrameCard,
  "canvas-link": CanvasLinkCard,
  "canvas-nested": CanvasNestedCard,
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

function drawingColor(color?: CanvasColor): string {
  if (!color) return "var(--ink-primary)";
  if (color.charCodeAt(0) === 35) return color;
  return (
    (
      {
        "1": "var(--danger)",
        "2": "var(--warning)",
        "3": "var(--accent)",
        "4": "var(--success)",
        "5": "var(--info)",
        "6": "var(--accent-ink)",
      } as Record<string, string>
    )[color] ?? "var(--ink-primary)"
  );
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

function CardTypeBadge({ node }: { node: CanvasNode }) {
  const { typeSchema } = useBoardContext();
  const typeId = node["x-writers-studio"]?.card?.typeId;
  const type = typeId ? typeSchema.cardTypes.find((item) => item.id === typeId) : undefined;
  if (!type) return null;
  return (
    <span
      className="pointer-events-none absolute right-2 top-2 z-10 rounded-full border border-[var(--border-subtle)] px-2 py-0.5 text-[9px] font-semibold uppercase tracking-wide"
      style={{ background: canvasColor(type.color) }}
    >
      {type.name}
    </span>
  );
}

function CardResizer({
  selected,
  group = false,
  drawing = false,
}: {
  selected: boolean;
  group?: boolean;
  drawing?: boolean;
}) {
  const { editable } = useBoardContext();
  return (
    <NodeResizer
      isVisible={editable && selected}
      minWidth={drawing ? 40 : group ? 240 : 180}
      minHeight={drawing ? 40 : group ? 160 : 120}
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
      <CardTypeBadge node={item} />
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
  const { editable } = useBoardContext();
  const item = data.canvasNode;
  if (item.type !== "file" || item["x-writers-studio"]?.resource?.kind !== "document") return null;
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
    >
      <CardResizer selected={selected} />
      <CardHandles interactive={editable} />
      <CardTypeBadge node={item} />
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
  if (item.type !== "file" || item["x-writers-studio"]?.resource?.kind !== "media") return null;
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
      <CardTypeBadge node={item} />
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

function CanvasExternalFileCard({ data, selected }: NodeProps<CanvasFlowNode>) {
  const { editable } = useBoardContext();
  const item = data.canvasNode;
  if (item.type !== "file" || item["x-writers-studio"]?.resource) return null;
  return (
    <article
      data-testid={`canvas-external-file-${item.id}`}
      className={cn(
        "size-full overflow-hidden rounded-[12px] border bg-[var(--surface-raised)] p-4 shadow-[0_5px_18px_var(--shadow-color)]",
        selected
          ? "border-[var(--accent)] ring-2 ring-[var(--accent)]/20"
          : "border-[var(--border-strong)]",
      )}
    >
      <CardResizer selected={selected} />
      <CardHandles interactive={editable} />
      <span className="flex items-center gap-2 text-xs uppercase tracking-wide text-[var(--ink-tertiary)]">
        <FilePlus2 className="size-4" aria-hidden="true" /> Внешний файл
      </span>
      <strong className="mt-3 block break-words text-sm">{item.file}</strong>
      <p className="mt-2 text-xs leading-5 text-[var(--ink-secondary)]">
        Путь сохранён из JSON Canvas. Добавьте файл в проект, чтобы открыть содержимое.
      </p>
    </article>
  );
}

function CanvasPDFCard({ data, selected }: NodeProps<CanvasFlowNode>) {
  const { editable } = useBoardContext();
  const item = data.canvasNode;
  const previewCanvas = useRef<HTMLCanvasElement>(null);
  const [previewFailed, setPreviewFailed] = useState(false);
  const media = data.media;

  useEffect(() => {
    if (item.type !== "file" || media?.mediaType !== "application/pdf" || !previewCanvas.current) {
      return;
    }
    let disposed = false;
    let loadingTask: { destroy: () => Promise<void> } | undefined;
    void Promise.all([import("pdfjs-dist"), import("pdfjs-dist/build/pdf.worker.min.mjs?url")])
      .then(async ([pdfjs, worker]) => {
        if (disposed || !previewCanvas.current) return;
        pdfjs.GlobalWorkerOptions.workerSrc = worker.default;
        const task = pdfjs.getDocument({ url: media.contentUrl, withCredentials: true });
        loadingTask = task;
        const document = await task.promise;
        const page = await document.getPage(1);
        const initial = page.getViewport({ scale: 1 });
        const viewport = page.getViewport({ scale: Math.min(2, item.width / initial.width) });
        const canvas = previewCanvas.current;
        if (!canvas || disposed) return;
        const ratio = window.devicePixelRatio || 1;
        canvas.width = Math.round(viewport.width * ratio);
        canvas.height = Math.round(viewport.height * ratio);
        canvas.style.width = `${viewport.width}px`;
        canvas.style.height = `${viewport.height}px`;
        const context = canvas.getContext("2d");
        if (!context) throw new Error("Canvas 2D is unavailable");
        await page.render({
          canvas,
          canvasContext: context,
          viewport,
          transform: [ratio, 0, 0, ratio, 0, 0],
        }).promise;
      })
      .catch(() => !disposed && setPreviewFailed(true));
    return () => {
      disposed = true;
      void loadingTask?.destroy();
    };
  }, [item, media]);

  if (item.type !== "file" || media?.mediaType !== "application/pdf") return null;
  return (
    <article
      data-testid={`canvas-pdf-${item.id}`}
      className={cn(
        "size-full overflow-hidden rounded-[12px] border bg-[var(--surface-raised)] shadow-[0_5px_18px_var(--shadow-color)]",
        selected
          ? "border-[var(--accent)] ring-2 ring-[var(--accent)]/20"
          : "border-[var(--border-strong)]",
      )}
    >
      <CardResizer selected={selected} />
      <CardHandles interactive={editable} />
      <CardTypeBadge node={item} />
      {previewFailed ? (
        <div className="grid size-full place-items-center p-5 text-center text-xs text-[var(--ink-secondary)]">
          Нажмите дважды, чтобы открыть PDF
        </div>
      ) : (
        <canvas ref={previewCanvas} className="mx-auto block max-w-full bg-[var(--ink-on-solid)]" />
      )}
      <footer className="absolute inset-x-0 bottom-0 flex items-center justify-between gap-2 bg-[var(--surface-overlay)] px-3 py-2 text-xs text-[var(--ink-on-solid)]">
        <span className="truncate">{media.originalName}</span>
        {media.pageCount ? <span className="shrink-0">{media.pageCount} стр.</span> : null}
      </footer>
    </article>
  );
}

function CanvasMediaCard({ data, selected }: NodeProps<CanvasFlowNode>) {
  const { editable } = useBoardContext();
  const item = data.canvasNode;
  const media = data.media;
  if (
    item.type !== "file" ||
    !media ||
    (!media.mediaType.startsWith("audio/") && !media.mediaType.startsWith("video/"))
  ) {
    return null;
  }
  const video = media.mediaType.startsWith("video/");
  return (
    <article
      data-testid={`canvas-media-${item.id}`}
      className={cn(
        "size-full overflow-hidden rounded-[12px] border bg-[var(--surface-raised)] shadow-[0_5px_18px_var(--shadow-color)]",
        selected
          ? "border-[var(--accent)] ring-2 ring-[var(--accent)]/20"
          : "border-[var(--border-strong)]",
      )}
    >
      <CardResizer selected={selected} />
      <CardHandles interactive={editable} />
      <CardTypeBadge node={item} />
      {video ? (
        <video
          className="nodrag size-full bg-[var(--surface-app)] object-contain"
          src={media.contentUrl}
          controls
          preload="metadata"
        >
          <track kind="captions" />
        </video>
      ) : (
        <div className="nodrag flex size-full flex-col justify-center gap-3 p-4">
          <strong className="truncate text-sm">{media.originalName}</strong>
          <audio className="w-full" src={media.contentUrl} controls preload="metadata">
            <track kind="captions" />
          </audio>
          {media.durationMs ? (
            <span className="text-[10px] text-[var(--ink-tertiary)]">
              Метаданные длительности: {Math.round(media.durationMs / 1000)} сек.
            </span>
          ) : null}
        </div>
      )}
      {video ? (
        <span className="pointer-events-none absolute inset-x-0 bottom-0 truncate bg-[var(--surface-overlay)] px-3 py-2 text-xs text-[var(--ink-on-solid)]">
          {media.originalName}
        </span>
      ) : null}
    </article>
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

function CanvasStrokeCard({ data, selected }: NodeProps<CanvasFlowNode>) {
  const item = data.canvasNode;
  const element = item.type === "group" ? item["x-writers-studio"]?.element : undefined;
  if (element?.kind !== "stroke") return null;
  return (
    <svg
      data-testid={`canvas-stroke-${item.id}`}
      viewBox={`0 0 ${item.width} ${item.height}`}
      className={cn(
        "size-full overflow-visible",
        selected && "drop-shadow-[0_0_4px_var(--accent)]",
      )}
      aria-hidden="true"
    >
      <path
        d={strokeElementPath(element)}
        fill={drawingColor(element.color)}
        fillOpacity={element.opacity}
      />
    </svg>
  );
}

function CanvasShapeCard({ data, selected }: NodeProps<CanvasFlowNode>) {
  const { editable } = useBoardContext();
  const item = data.canvasNode;
  const extension = item.type === "group" ? item["x-writers-studio"] : undefined;
  const element = extension?.element;
  if (element?.kind !== "shape") return null;
  const stroke = drawingColor(element.color);
  const fill = element.fill === "transparent" ? "transparent" : drawingColor(element.fill);
  const markerId = `canvas-arrow-${item.id}`;
  const ascending = extension?.lineDirection === "ascending";
  const common = {
    fill,
    stroke,
    strokeWidth: element.size,
    strokeOpacity: element.opacity,
    strokeDasharray:
      element.dash === "dashed" ? "10 7" : element.dash === "dotted" ? "2 7" : undefined,
    vectorEffect: "non-scaling-stroke" as const,
  };
  return (
    <div
      data-testid={`canvas-shape-${item.id}`}
      className={cn("size-full", selected && "drop-shadow-[0_0_4px_var(--accent)]")}
    >
      <CardResizer selected={selected} drawing />
      <CardHandles interactive={editable} />
      <CardTypeBadge node={item} />
      <svg viewBox={`0 0 ${item.width} ${item.height}`} className="size-full overflow-visible">
        <title>{element.label || `Фигура ${element.shape}`}</title>
        <defs>
          <marker
            id={markerId}
            markerWidth="8"
            markerHeight="8"
            refX="6"
            refY="4"
            orient="auto"
            markerUnits="strokeWidth"
          >
            <path d="M 0 0 L 8 4 L 0 8 Z" fill={stroke} fillOpacity={element.opacity} />
          </marker>
        </defs>
        <g
          transform={`rotate(${element.rotation} ${item.width / 2} ${item.height / 2})`}
          className="pointer-events-auto"
        >
          {element.shape === "rectangle" ? (
            <rect x="2" y="2" width={item.width - 4} height={item.height - 4} rx="6" {...common} />
          ) : null}
          {element.shape === "ellipse" ? (
            <ellipse
              cx={item.width / 2}
              cy={item.height / 2}
              rx={item.width / 2 - 2}
              ry={item.height / 2 - 2}
              {...common}
            />
          ) : null}
          {element.shape === "diamond" ? (
            <path
              d={`M ${item.width / 2} 2 L ${item.width - 2} ${item.height / 2} L ${item.width / 2} ${item.height - 2} L 2 ${item.height / 2} Z`}
              {...common}
            />
          ) : null}
          {element.shape === "line" || element.shape === "arrow" ? (
            <line
              x1="2"
              y1={ascending ? item.height - 2 : 2}
              x2={item.width - 2}
              y2={ascending ? 2 : item.height - 2}
              {...common}
              markerEnd={element.shape === "arrow" ? `url(#${markerId})` : undefined}
            />
          ) : null}
          {element.label ? (
            <text
              x="50%"
              y="50%"
              textAnchor="middle"
              dominantBaseline="middle"
              fill="var(--ink-primary)"
              className="text-xs"
            >
              {element.label}
            </text>
          ) : null}
        </g>
      </svg>
    </div>
  );
}

function CanvasFrameCard({ data, selected }: NodeProps<CanvasFlowNode>) {
  const { editable } = useBoardContext();
  const item = data.canvasNode;
  if (item.type !== "group") return null;
  const element = item["x-writers-studio"]?.element;
  if (element?.kind !== "frame") return null;
  return (
    <section
      data-testid={`canvas-frame-${item.id}`}
      className={cn(
        "size-full rounded-[16px] border-2 bg-transparent",
        selected ? "border-[var(--accent)]" : "border-[var(--border-strong)]",
      )}
    >
      <CardResizer selected={selected} group />
      <CardHandles interactive={editable} />
      <span className="absolute -top-8 left-2 rounded-full bg-[var(--surface-raised)] px-3 py-1 text-xs font-semibold shadow-sm">
        {element.order}. {element.label || item.label || "Фрейм"}
      </span>
    </section>
  );
}

function CanvasLinkCard({ data, selected }: NodeProps<CanvasFlowNode>) {
  const { editable } = useBoardContext();
  const item = data.canvasNode;
  if (item.type !== "link") return null;
  const preview = item["x-writers-studio"]?.preview;
  let host = item.url;
  try {
    host = new URL(item.url).hostname;
  } catch {
    // Backend validation remains authoritative; keep the raw URL visible meanwhile.
  }
  return (
    <article
      data-testid={`canvas-link-${item.id}`}
      className={cn(
        "block size-full overflow-hidden rounded-[12px] border bg-[var(--surface-raised)] p-4 shadow-[0_5px_18px_var(--shadow-color)]",
        selected
          ? "border-[var(--accent)] ring-2 ring-[var(--accent)]/20"
          : "border-[var(--border-strong)]",
      )}
    >
      <CardResizer selected={selected} />
      <CardHandles interactive={editable} />
      <CardTypeBadge node={item} />
      <span className="flex items-center gap-2 text-xs text-[var(--ink-secondary)]">
        <Link2 className="size-4" aria-hidden="true" /> {preview?.siteName || host}
      </span>
      <strong className="mt-3 line-clamp-2 block text-base">{preview?.title || item.url}</strong>
      {preview?.description ? (
        <p className="mt-2 line-clamp-3 text-xs leading-5 text-[var(--ink-secondary)]">
          {preview.description}
        </p>
      ) : null}
    </article>
  );
}

function CanvasNestedCard({ data, selected }: NodeProps<CanvasFlowNode>) {
  const { editable } = useBoardContext();
  const item = data.canvasNode;
  if (item.type !== "file" || item["x-writers-studio"]?.resource?.kind !== "canvas") return null;
  const title =
    typeof item["x-writers-studio"].nestedTitle === "string"
      ? item["x-writers-studio"].nestedTitle
      : item.file.split("/").at(-1);
  return (
    <article
      data-testid={`canvas-nested-${item.id}`}
      className={cn(
        "size-full rounded-[12px] border bg-[var(--surface-raised)] p-4 text-left shadow-[0_5px_18px_var(--shadow-color)]",
        selected
          ? "border-[var(--accent)] ring-2 ring-[var(--accent)]/20"
          : "border-[var(--border-strong)]",
      )}
    >
      <CardResizer selected={selected} />
      <CardHandles interactive={editable} />
      <CardTypeBadge node={item} />
      <span className="text-xs uppercase tracking-wide text-[var(--ink-tertiary)]">
        Другая доска
      </span>
      <strong className="mt-3 block">{title}</strong>
      <p className="mt-2 text-xs text-[var(--ink-secondary)]">
        Дважды нажмите, чтобы перейти без рекурсивного embed.
      </p>
    </article>
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

function isDrawingTool(
  tool: CanvasTool,
): tool is Exclude<CanvasTool, "select" | "hand" | "connector" | "note"> {
  return !["select", "hand", "connector", "note"].includes(tool);
}

function isShapeTool(tool: CanvasTool): tool is CanvasShapeElement["shape"] {
  return ["rectangle", "ellipse", "diamond", "line", "arrow"].includes(tool);
}

function GesturePreview({ gesture }: { gesture: CanvasGesture }) {
  const first = gesture.points[0];
  const last = gesture.points.at(-1);
  if (!first || !last) return null;
  if (gesture.tool === "eraser") {
    return (
      <div
        className="pointer-events-none absolute size-8 rounded-full border-2 border-[var(--danger)] bg-[var(--danger-soft)]/30"
        style={{ left: last.x - 16, top: last.y - 16 }}
      />
    );
  }
  if (gesture.tool === "pen" || gesture.tool === "highlighter") {
    const element: CanvasStrokeElement = {
      kind: "stroke" as const,
      points: gesture.points.map((point) =>
        point.pressure === undefined
          ? ([point.x, point.y] as CanvasStrokePoint)
          : ([point.x, point.y, point.pressure] as CanvasStrokePoint),
      ),
      brush: gesture.tool,
      color: "5" as const,
      size: gesture.tool === "highlighter" ? 18 : 4,
      opacity: gesture.tool === "highlighter" ? 0.32 : 1,
    };
    return (
      <svg
        className="pointer-events-none absolute left-0 top-0 size-px overflow-visible"
        aria-hidden="true"
      >
        <path
          d={strokeElementPath(element)}
          fill={drawingColor(element.color)}
          fillOpacity={element.opacity}
        />
      </svg>
    );
  }
  const left = Math.min(first.x, last.x);
  const top = Math.min(first.y, last.y);
  const width = Math.max(1, Math.abs(last.x - first.x));
  const height = Math.max(1, Math.abs(last.y - first.y));
  const ascending = last.x < first.x !== last.y < first.y;
  return (
    <svg
      className="pointer-events-none absolute overflow-visible"
      style={{ left, top, width, height }}
      viewBox={`0 0 ${width} ${height}`}
      aria-hidden="true"
    >
      {gesture.tool === "rectangle" ? (
        <rect
          x="1"
          y="1"
          width={Math.max(1, width - 2)}
          height={Math.max(1, height - 2)}
          rx="5"
          fill="transparent"
          stroke="var(--info)"
          strokeWidth="3"
        />
      ) : null}
      {gesture.tool === "ellipse" ? (
        <ellipse
          cx={width / 2}
          cy={height / 2}
          rx={Math.max(1, width / 2 - 1)}
          ry={Math.max(1, height / 2 - 1)}
          fill="transparent"
          stroke="var(--info)"
          strokeWidth="3"
        />
      ) : null}
      {gesture.tool === "diamond" ? (
        <path
          d={`M ${width / 2} 1 L ${width - 1} ${height / 2} L ${width / 2} ${height - 1} L 1 ${height / 2} Z`}
          fill="transparent"
          stroke="var(--info)"
          strokeWidth="3"
        />
      ) : null}
      {gesture.tool === "line" || gesture.tool === "arrow" ? (
        <line
          x1="1"
          y1={ascending ? height - 1 : 1}
          x2={width - 1}
          y2={ascending ? 1 : height - 1}
          stroke="var(--info)"
          strokeWidth="3"
        />
      ) : null}
    </svg>
  );
}

function LayoutPreview({ source, preview }: { source: CanvasDocument; preview: CanvasDocument }) {
  const sourceById = new Map(source.nodes.map((node) => [node.id, node]));
  return (
    <>
      {preview.nodes.map((node) => {
        const original = sourceById.get(node.id);
        if (!original || (original.x === node.x && original.y === node.y)) return null;
        return (
          <div
            key={node.id}
            className="pointer-events-none absolute rounded-lg border-2 border-dashed border-[var(--info)] bg-[var(--info-soft)]/30"
            style={{ left: node.x, top: node.y, width: node.width, height: node.height }}
          />
        );
      })}
    </>
  );
}

function SnapGuideOverlay({ guides }: { guides: CanvasSnapResult }) {
  return (
    <>
      {guides.vertical ? (
        <div
          className="pointer-events-none absolute w-px bg-[var(--info)] shadow-[0_0_0_1px_var(--surface-app)]"
          style={{
            left: guides.vertical.position,
            top: guides.vertical.start,
            height: guides.vertical.end - guides.vertical.start,
          }}
        />
      ) : null}
      {guides.horizontal ? (
        <div
          className="pointer-events-none absolute h-px bg-[var(--info)] shadow-[0_0_0_1px_var(--surface-app)]"
          style={{
            left: guides.horizontal.start,
            top: guides.horizontal.position,
            width: guides.horizontal.end - guides.horizontal.start,
          }}
        />
      ) : null}
    </>
  );
}

function CanvasBoardInner({
  projectId,
  title = "Canvas",
  content,
  documents,
  media,
  projectDocuments,
  typeSchema,
  editable = true,
  promotionEnabled = true,
  onChange,
  onOpenDocument,
  onOpenCanvas,
  onPromoteNode,
  initialFocusNodeId,
  onAssistantContextChange,
}: CanvasBoardProps) {
  const reactFlow = useReactFlow<CanvasFlowNode, CanvasFlowEdge>();
  const { requestInput, requestSelect } = useActionDialog();
  const { showToast } = useToast();
  const [tablet, setTablet] = useState(() => window.matchMedia("(min-width: 768px)").matches);
  const [activeView, setActiveView] = useState<CanvasViewMode>(
    () => content["x-writers-studio"]?.activeView ?? "free",
  );
  const [viewFilterTypeId, setViewFilterTypeId] = useState(
    () => content["x-writers-studio"]?.viewFilterTypeId,
  );
  const canEdit = editable && tablet && activeView === "free";
  const [history, setHistory] = useState(() => createCanvasHistory(content));
  const historyRef = useRef(history);
  const [initialFlow] = useState(() =>
    canvasToFlow(projectCanvasView(content, activeView, viewFilterTypeId), documents, media),
  );
  const [nodes, setNodes] = useState<CanvasFlowNode[]>(initialFlow.nodes);
  const [edges, setEdges] = useState<CanvasFlowEdge[]>(initialFlow.edges);
  const nodesRef = useRef(nodes);
  const edgesRef = useRef(edges);
  const [selectedNodeIds, setSelectedNodeIds] = useState<string[]>([]);
  const [selectedEdgeIds, setSelectedEdgeIds] = useState<string[]>([]);
  const [localMedia, setLocalMedia] = useState(media);
  const [tool, setTool] = useState<CanvasTool>("select");
  const toolRail = useToolRailMenu<CanvasToolMenu>();
  const [gesture, setGesture] = useState<CanvasGesture>();
  const [layoutPreview, setLayoutPreview] = useState<CanvasDocument>();
  const [layoutPending, setLayoutPending] = useState(false);
  const diagnosticsPanel = useDismissiblePanel<CanvasGraphDiagnostic[]>();
  const diagnostics = diagnosticsPanel.value;
  const navigatorPanel = useDismissiblePanel<true>();
  const nodeMenu = useDismissiblePanel<CanvasNodeMenu>();
  const [navigatorQuery, setNavigatorQuery] = useState("");
  const navigatorInput = useRef<HTMLInputElement>(null);
  const navigatorItems = useRef<Array<HTMLButtonElement | null>>([]);
  const [presentationIndex, setPresentationIndex] = useState<number>();
  const [focusEndsAt, setFocusEndsAt] = useState<number>();
  const [focusNow, setFocusNow] = useState(() => Date.now());
  const [playthrough, setPlaythrough] = useState<CanvasPlaythroughState>();
  const [exportPending, setExportPending] = useState(false);
  const [snapGuides, setSnapGuides] = useState<CanvasSnapResult>();
  const [snappingDisabled, setSnappingDisabled] = useState(false);
  const gestureRef = useRef<CanvasGesture | undefined>(undefined);
  const previousTool = useRef<CanvasTool | undefined>(undefined);
  const dragStart = useRef<{ id: string; x: number; y: number } | undefined>(undefined);
  const clipboard = useRef<CanvasClipboard | undefined>(undefined);
  const fileInput = useRef<HTMLInputElement>(null);
  const board = useRef<HTMLDivElement>(null);
  const initialFocusApplied = useRef(false);

  useEffect(() => {
    const query = window.matchMedia("(min-width: 768px)");
    const update = () => setTablet(query.matches);
    query.addEventListener("change", update);
    return () => query.removeEventListener("change", update);
  }, []);

  useEffect(() => {
    const update = (event: globalThis.KeyboardEvent) => setSnappingDisabled(event.altKey);
    const reset = () => setSnappingDisabled(false);
    window.addEventListener("keydown", update);
    window.addEventListener("keyup", update);
    window.addEventListener("blur", reset);
    return () => {
      window.removeEventListener("keydown", update);
      window.removeEventListener("keyup", update);
      window.removeEventListener("blur", reset);
    };
  }, []);

  useEffect(() => {
    const flow = canvasToFlow(
      projectCanvasView(historyRef.current.present, activeView, viewFilterTypeId),
      documents,
      localMedia,
    );
    nodesRef.current = flow.nodes;
    edgesRef.current = flow.edges;
    setNodes(flow.nodes);
    setEdges(flow.edges);
  }, [activeView, documents, localMedia, viewFilterTypeId]);

  useEffect(() => {
    if (!initialFocusNodeId || initialFocusApplied.current) return;
    const node = nodesRef.current.find((item) => item.id === initialFocusNodeId);
    if (!node) return;
    initialFocusApplied.current = true;
    setSelectedNodeIds([initialFocusNodeId]);
    setNodes((current) =>
      current.map((item) => ({ ...item, selected: item.id === initialFocusNodeId })),
    );
    window.setTimeout(
      () => void reactFlow.fitView({ nodes: [node], padding: 0.5, duration: 300, maxZoom: 1.4 }),
      0,
    );
  }, [initialFocusNodeId, reactFlow]);

  useEffect(() => {
    if (!navigatorPanel.isOpen) return;
    navigatorInput.current?.focus();
  }, [navigatorPanel.isOpen]);

  useEffect(() => {
    if (focusEndsAt === undefined) return;
    const tick = () => {
      const now = Date.now();
      setFocusNow(now);
      if (now < focusEndsAt) return;
      setFocusEndsAt(undefined);
      showToast({ title: "Фокус-сессия завершена", description: "Пора зафиксировать результат" });
    };
    tick();
    const interval = window.setInterval(tick, 1_000);
    return () => window.clearInterval(interval);
  }, [focusEndsAt, showToast]);

  const canvasObjects = useMemo(
    () => listCanvasObjects(history.present, documents, localMedia),
    [documents, history.present, localMedia],
  );
  const filteredCanvasObjects = useMemo(
    () => searchCanvasObjects(canvasObjects, navigatorQuery),
    [canvasObjects, navigatorQuery],
  );
  const presentationFrames = useMemo(() => orderedCanvasFrames(history.present), [history.present]);
  const focusRemaining = canvasFocusRemainingSeconds(focusEndsAt, focusNow);
  const currentPresentationFrame =
    presentationIndex === undefined ? undefined : presentationFrames[presentationIndex];
  const currentPresentationElement = currentPresentationFrame?.["x-writers-studio"]?.element;
  const currentPresentationLabel =
    currentPresentationElement?.kind === "frame"
      ? currentPresentationElement.label || currentPresentationFrame?.label
      : currentPresentationFrame?.label;

  const syncFlow = useCallback(
    (
      document: CanvasDocument,
      selectedIds: string[] = [],
      view: CanvasViewMode = activeView,
      filterTypeId: string | undefined = viewFilterTypeId,
    ) => {
      const selected = new Set(selectedIds);
      const flow = canvasToFlow(
        projectCanvasView(document, view, filterTypeId),
        documents,
        localMedia,
      );
      flow.nodes = flow.nodes.map((node) => ({ ...node, selected: selected.has(node.id) }));
      nodesRef.current = flow.nodes;
      edgesRef.current = flow.edges;
      setNodes(flow.nodes);
      setEdges(flow.edges);
    },
    [activeView, documents, localMedia, viewFilterTypeId],
  );

  const chooseCanvasView = useCallback(() => {
    void requestSelect({
      title: "Представление доски",
      label: "Одна модель, разные способы просмотра",
      options: [
        { value: "free", label: "Свободный Canvas · редактирование" },
        { value: "mind-map", label: "Mind map" },
        { value: "timeline", label: "Timeline по полю date" },
        { value: "swimlanes", label: "Swimlanes по типам карточек" },
        { value: "dependency", label: "Dependency graph" },
        { value: "filtered", label: "Фильтр по типу" },
      ],
      confirmLabel: "Показать",
    }).then(async (selectedView) => {
      if (!selectedView) return;
      const view = selectedView as CanvasViewMode;
      let filterTypeId = viewFilterTypeId;
      if (view === "filtered") {
        filterTypeId = await requestSelect({
          title: "Фильтр карточек",
          label: "Показать только тип",
          options: typeSchema.cardTypes.map((type) => ({ value: type.id, label: type.name })),
          confirmLabel: "Применить",
        });
        if (!filterTypeId) return;
      }
      setActiveView(view);
      setViewFilterTypeId(filterTypeId);
      const current = historyRef.current.present;
      const document: CanvasDocument = {
        ...current,
        "x-writers-studio": {
          ...current["x-writers-studio"],
          schemaVersion: 2,
          activeView: view,
          viewFilterTypeId: view === "filtered" ? filterTypeId : undefined,
        },
      };
      const next = commitCanvasHistory(historyRef.current, document);
      historyRef.current = next;
      setHistory(next);
      syncFlow(document, [], view, filterTypeId);
      onChange(document);
      setSelectedNodeIds([]);
      setSelectedEdgeIds([]);
      window.setTimeout(() => void reactFlow.fitView({ padding: 0.18, duration: 250 }), 0);
    });
  }, [onChange, reactFlow, requestSelect, syncFlow, typeSchema.cardTypes, viewFilterTypeId]);

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
      const migrated: CanvasDocument = {
        ...document,
        "x-writers-studio": {
          ...document["x-writers-studio"],
          schemaVersion: 2,
        },
      };
      const next = commitCanvasHistory(historyRef.current, migrated);
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
    (position?: { x: number; y: number }, text = "# Новая заметка\n\nЗапишите идею…") => {
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
        text,
      };
      commit(
        { ...historyRef.current.present, nodes: [...historyRef.current.present.nodes, node] },
        [node.id],
      );
      setSelectedNodeIds([node.id]);
    },
    [canEdit, commit, reactFlow],
  );

  const addLink = useCallback(
    (url: string, position?: { x: number; y: number }) => {
      if (!canEdit) return;
      let normalized: string;
      try {
        const parsed = new URL(url.trim());
        if (parsed.protocol !== "http:" && parsed.protocol !== "https:") return;
        normalized = parsed.toString();
      } catch {
        return;
      }
      const center =
        position ??
        reactFlow.screenToFlowPosition({ x: window.innerWidth / 2, y: window.innerHeight / 2 });
      const node: CanvasNode = {
        id: crypto.randomUUID(),
        type: "link",
        url: normalized,
        x: Math.round(center.x - defaultCardWidth / 2),
        y: Math.round(center.y - defaultCardHeight / 2),
        width: defaultCardWidth,
        height: defaultCardHeight,
      };
      commit(
        { ...historyRef.current.present, nodes: [...historyRef.current.present.nodes, node] },
        [node.id],
      );
      setSelectedNodeIds([node.id]);

      void fetchCanvasLinkPreviews(projectId, [normalized])
        .then(({ previews }) => {
          const preview = previews[0];
          if (!preview) return;
          const current = historyRef.current.present;
          if (!current.nodes.some((item) => item.id === node.id)) return;
          commit(
            {
              ...current,
              nodes: current.nodes.map((item) =>
                item.id === node.id && item.type === "link"
                  ? {
                      ...item,
                      url: preview.url,
                      "x-writers-studio": {
                        ...item["x-writers-studio"],
                        preview: {
                          title: preview.title,
                          description: preview.description,
                          siteName: preview.siteName,
                          fetchedAt: preview.fetchedAt,
                        },
                      },
                    }
                  : item,
              ),
            },
            [node.id],
          );
        })
        .catch(() => {
          // The URL card remains useful even when a remote site rejects safe server-side previewing.
        });
    },
    [canEdit, commit, projectId, reactFlow],
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

  const addMedia = useCallback(
    async (file: File, position?: { x: number; y: number }) => {
      if (!canEdit) return;
      try {
        const response = await uploadProjectMedia(projectId, file);
        setLocalMedia((current) => [
          ...current.filter((item) => item.id !== response.media.id),
          response.media,
        ]);
        const center =
          position ??
          reactFlow.screenToFlowPosition({ x: window.innerWidth / 2, y: window.innerHeight / 2 });
        const ratio =
          response.media.width && response.media.height
            ? Math.max(0.35, Math.min(2.5, response.media.width / response.media.height))
            : 0.78;
        const audio = response.media.mediaType.startsWith("audio/");
        const video = response.media.mediaType.startsWith("video/");
        const width = video
          ? 360
          : response.media.mediaType === "application/pdf" || audio
            ? 320
            : 300;
        const height = audio ? 130 : video ? 210 : Math.round(width / ratio);
        const node: CanvasNode = {
          id: crypto.randomUUID(),
          type: "file",
          file: `writers-studio://media/${response.media.id}`,
          x: Math.round(center.x - width / 2),
          y: Math.round(center.y - height / 2),
          width,
          height,
          "x-writers-studio": {
            resource: { kind: "media", id: response.media.id },
            mediaType: response.media.mediaType,
          },
        };
        commit(
          { ...historyRef.current.present, nodes: [...historyRef.current.present.nodes, node] },
          [node.id],
        );
        setSelectedNodeIds([node.id]);
      } catch (error) {
        showToast({
          title: "Не удалось добавить файл",
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

  const frameSelection = useCallback(() => {
    if (!canEdit || selectedNodeIds.length === 0) return;
    const result = createFrameAroundSelection(historyRef.current.present, selectedNodeIds, () =>
      crypto.randomUUID(),
    );
    if (!result.frameId) return;
    commit(result.document, [result.frameId, ...selectedNodeIds]);
    setSelectedNodeIds([result.frameId, ...selectedNodeIds]);
  }, [canEdit, commit, selectedNodeIds]);

  const alignSelection = useCallback(() => {
    if (!canEdit || selectedNodeIds.length < 2) return;
    void requestSelect({
      title: "Выровнять элементы",
      label: "Ориентир",
      options: [
        { value: "left", label: "По левому краю" },
        { value: "center-x", label: "По центру по горизонтали" },
        { value: "right", label: "По правому краю" },
        { value: "top", label: "По верхнему краю" },
        { value: "center-y", label: "По центру по вертикали" },
        { value: "bottom", label: "По нижнему краю" },
      ],
      confirmLabel: "Выровнять",
    }).then((alignment) => {
      if (!alignment) return;
      commit(
        alignCanvasSelection(
          historyRef.current.present,
          selectedNodeIds,
          alignment as Parameters<typeof alignCanvasSelection>[2],
        ),
        selectedNodeIds,
      );
    });
  }, [canEdit, commit, requestSelect, selectedNodeIds]);

  const distributeSelection = useCallback(() => {
    if (!canEdit || selectedNodeIds.length < 3) return;
    void requestSelect({
      title: "Распределить элементы",
      label: "Направление",
      options: [
        { value: "horizontal", label: "Равные интервалы по горизонтали" },
        { value: "vertical", label: "Равные интервалы по вертикали" },
      ],
      confirmLabel: "Распределить",
    }).then((axis) => {
      if (axis !== "horizontal" && axis !== "vertical") return;
      commit(
        distributeCanvasSelection(historyRef.current.present, selectedNodeIds, axis),
        selectedNodeIds,
      );
    });
  }, [canEdit, commit, requestSelect, selectedNodeIds]);

  const tidySelection = useCallback(() => {
    if (!canEdit || selectedNodeIds.length < 2) return;
    commit(tidyCanvasSelection(historyRef.current.present, selectedNodeIds), selectedNodeIds);
  }, [canEdit, commit, selectedNodeIds]);

  const previewAutomaticLayout = useCallback(() => {
    if (!canEdit) return;
    void requestSelect({
      title: "Автоматическая раскладка",
      label: "Алгоритм",
      options: [
        { value: "hierarchy-horizontal", label: "Иерархия слева направо" },
        { value: "hierarchy-vertical", label: "Иерархия сверху вниз" },
        { value: "mind-map", label: "Mind map / radial" },
        { value: "organic", label: "Органическая сеть" },
      ],
      confirmLabel: "Показать preview",
    }).then((mode) => {
      if (!mode) return;
      setLayoutPending(true);
      void requestCanvasLayout(
        historyRef.current.present,
        selectedNodeIds,
        mode as CanvasLayoutMode,
      )
        .then(setLayoutPreview)
        .catch((error: unknown) =>
          showToast({
            title: "Не удалось рассчитать раскладку",
            description: error instanceof Error ? error.message : undefined,
          }),
        )
        .finally(() => setLayoutPending(false));
    });
  }, [canEdit, requestSelect, selectedNodeIds, showToast]);

  const addTemplate = useCallback(() => {
    if (!canEdit) return;
    void requestSelect({
      title: "Шаблон доски",
      label: "Сценарий работы",
      options: canvasTemplates.map((template) => ({
        value: template.id,
        label: `${template.title} — ${template.description}`,
      })),
      confirmLabel: "Добавить",
    }).then((templateId) => {
      if (!templateId) return;
      const center = reactFlow.screenToFlowPosition({
        x: window.innerWidth / 2,
        y: window.innerHeight / 2,
      });
      const template = createCanvasTemplate(
        templateId as Parameters<typeof createCanvasTemplate>[0],
        () => crypto.randomUUID(),
        { x: Math.round(center.x - 600), y: Math.round(center.y - 280) },
      );
      if (template.nodes.length === 0) {
        showToast({ title: "Чистый шаблон не добавляет элементов" });
        return;
      }
      commit(
        {
          ...historyRef.current.present,
          nodes: [...historyRef.current.present.nodes, ...template.nodes],
          edges: [...historyRef.current.present.edges, ...template.edges],
        },
        template.nodes.map((node) => node.id),
      );
      setSelectedNodeIds(template.nodes.map((node) => node.id));
    });
  }, [canEdit, commit, reactFlow, requestSelect, showToast]);

  const runGraphAnalysis = useCallback(() => {
    if (diagnosticsPanel.isOpen) {
      diagnosticsPanel.close();
      return;
    }
    const result = analyzeCanvasGraph(historyRef.current.present, "all");
    diagnosticsPanel.open(result.diagnostics);
    if (result.diagnostics.length === 0) {
      showToast({ title: "Граф выглядит согласованно", description: "Предупреждений не найдено" });
    }
  }, [diagnosticsPanel, showToast]);

  const focusCanvasObject = useCallback(
    (nodeId: string) => {
      const selected = new Set([nodeId]);
      setSelectedNodeIds([nodeId]);
      setSelectedEdgeIds([]);
      setNodes((current) => current.map((node) => ({ ...node, selected: selected.has(node.id) })));
      const node = nodesRef.current.find((item) => item.id === nodeId);
      if (node) {
        void reactFlow.fitView({ nodes: [node], padding: 0.55, duration: 250, maxZoom: 1.4 });
      }
    },
    [reactFlow],
  );

  const startPlaythrough = useCallback(() => {
    void requestSelect({
      title: "Ручной play-through",
      label: "Тип графа",
      options: [
        { value: "dialogue", label: "Диалог" },
        { value: "quest", label: "Квесты" },
        { value: "state", label: "State machine" },
      ],
      confirmLabel: "Начать",
    }).then(async (value) => {
      if (!value) return;
      const profile = value as CanvasPlaythroughProfile;
      const starts = canvasPlaythroughStarts(historyRef.current.present, profile);
      if (starts.length === 0) {
        showToast({ title: "Для этого сценария нет типизированных карточек" });
        return;
      }
      const startId =
        starts.length === 1
          ? starts[0]?.id
          : await requestSelect({
              title: "Точка входа",
              label: "С чего начать прохождение",
              options: starts.map((node) => ({
                value: node.id,
                label:
                  listCanvasObjects({ nodes: [node], edges: [] }, documents, localMedia)[0]
                    ?.title ?? node.id,
              })),
              confirmLabel: "Старт",
            });
      if (!startId) return;
      setPlaythrough(startCanvasPlaythrough(profile, startId));
      focusCanvasObject(startId);
    });
  }, [documents, focusCanvasObject, localMedia, requestSelect, showToast]);

  const playthroughChoices = useMemo(
    () => (playthrough ? canvasPlaythroughChoices(history.present, playthrough) : []),
    [history.present, playthrough],
  );
  const currentPlaythroughObject = playthrough
    ? canvasObjects.find((item) => item.id === playthrough.path.at(-1))
    : undefined;

  const advancePlaythrough = useCallback(
    (choice: (typeof playthroughChoices)[number]) => {
      if (!playthrough) return;
      const next = advanceCanvasPlaythrough(playthrough, choice);
      setPlaythrough(next);
      focusCanvasObject(choice.toNodeId);
      setSelectedEdgeIds(next.edgePath);
      const selectedEdges = new Set(next.edgePath);
      setEdges((current) =>
        current.map((edge) => ({ ...edge, selected: selectedEdges.has(edge.id) })),
      );
    },
    [focusCanvasObject, playthrough],
  );

  const showPresentationFrame = useCallback(
    (index: number) => {
      if (presentationFrames.length === 0) {
        showToast({ title: "На доске пока нет фреймов" });
        return;
      }
      const nextIndex = Math.max(0, Math.min(presentationFrames.length - 1, Math.trunc(index)));
      setPresentationIndex(nextIndex);
      const frame = presentationFrames[nextIndex];
      const flowFrame = frame ? nodesRef.current.find((node) => node.id === frame.id) : undefined;
      if (flowFrame) {
        void reactFlow.fitView({ nodes: [flowFrame], padding: 0.08, duration: 350, maxZoom: 1.5 });
      }
    },
    [presentationFrames, reactFlow, showToast],
  );

  const startFocusTimer = useCallback(() => {
    void requestSelect({
      title: "Фокус-сессия",
      label: "Продолжительность",
      options: [5, 10, 15, 25, 45].map((minutes) => ({
        value: String(minutes),
        label: `${minutes} минут`,
      })),
      confirmLabel: "Начать",
    }).then((value) => {
      if (!value) return;
      const now = Date.now();
      setFocusNow(now);
      setFocusEndsAt(canvasFocusTimerEnd(now, Number(value)));
    });
  }, [requestSelect]);

  const setSelectionColor = useCallback(
    (color?: CanvasColor) => {
      if (!canEdit) return;
      const nodeIds = new Set(selectedNodeIds);
      const edgeIds = new Set(selectedEdgeIds);
      commit({
        ...historyRef.current.present,
        nodes: historyRef.current.present.nodes.map((item) => {
          if (!nodeIds.has(item.id)) return item;
          const extension = item["x-writers-studio"];
          const element = extension?.element;
          if (item.type === "group" && element && element.kind !== "frame" && color) {
            return {
              ...item,
              color,
              "x-writers-studio": {
                ...extension,
                element: { ...element, color },
              },
            } as CanvasNode;
          }
          return { ...item, color } as CanvasNode;
        }),
        edges: historyRef.current.present.edges.map((item) =>
          edgeIds.has(item.id) ? { ...item, color } : item,
        ),
      });
    },
    [canEdit, commit, selectedEdgeIds, selectedNodeIds],
  );

  const assignCardType = useCallback(() => {
    if (!canEdit || selectedNodeIds.length === 0) return;
    void requestSelect({
      title: "Тип карточки",
      label: "Проектный тип",
      options: typeSchema.cardTypes.map((type) => ({
        value: type.id,
        label: `${type.name} · ${type.category}`,
      })),
      confirmLabel: "Назначить",
    }).then((typeId) => {
      if (!typeId) return;
      const selected = new Set(selectedNodeIds);
      commit(
        {
          ...historyRef.current.present,
          nodes: historyRef.current.present.nodes.map((node) => {
            if (!selected.has(node.id)) return node;
            const extension = node["x-writers-studio"];
            return {
              ...node,
              "x-writers-studio": {
                ...extension,
                card: {
                  typeId,
                  fields: extension?.card?.typeId === typeId ? extension.card.fields : {},
                },
              },
            } as CanvasNode;
          }),
        },
        selectedNodeIds,
      );
    });
  }, [canEdit, commit, requestSelect, selectedNodeIds, typeSchema.cardTypes]);

  const assignRelationType = useCallback(() => {
    if (!canEdit || selectedEdgeIds.length === 0) return;
    void requestSelect({
      title: "Тип связи",
      label: "Семантика",
      options: typeSchema.relationTypes.map((type) => ({ value: type.id, label: type.name })),
      confirmLabel: "Назначить",
    }).then((typeId) => {
      const type = typeSchema.relationTypes.find((item) => item.id === typeId);
      if (!type) return;
      const selected = new Set(selectedEdgeIds);
      commit({
        ...historyRef.current.present,
        edges: historyRef.current.present.edges.map((edge) =>
          selected.has(edge.id)
            ? {
                ...edge,
                color: type.color,
                toEnd: type.directed ? "arrow" : "none",
                "x-writers-studio": {
                  ...edge["x-writers-studio"],
                  relation: { typeId: type.id },
                },
              }
            : edge,
        ),
      });
    });
  }, [canEdit, commit, requestSelect, selectedEdgeIds, typeSchema.relationTypes]);

  const promoteSelection = useCallback(async () => {
    if (!canEdit || !promotionEnabled || !onPromoteNode || selectedNodeIds.length !== 1) return;
    const node = historyRef.current.present.nodes.find((item) => item.id === selectedNodeIds[0]);
    if (
      !node ||
      node.type === "file" ||
      (node.type === "group" && node["x-writers-studio"]?.element)
    )
      return;
    const source =
      node.type === "text"
        ? node.text
        : node.type === "link"
          ? (node["x-writers-studio"]?.preview?.title ?? node.url)
          : (node.label ?? "Новый документ");
    const suggestedTitle =
      source
        .split("\n", 1)[0]
        ?.replace(/^#{1,6}\s*/u, "")
        .trim()
        .slice(0, 120) || "Новый документ";
    const result = await onPromoteNode(node.id, suggestedTitle);
    if (!result) return;
    const nextHistory = createCanvasHistory(result.revision.content);
    historyRef.current = nextHistory;
    setHistory(nextHistory);
    const promotedPreview: CanvasDocumentPreview = {
      id: result.document.id,
      title: result.document.title,
      kind: result.document.kind,
      status: result.document.status,
      currentRevision: result.document.currentRevision,
      plainText: result.document.plainText ?? "",
      trashedAt: result.document.trashedAt,
    };
    const nextFlow = canvasToFlow(
      projectCanvasView(result.revision.content, activeView, viewFilterTypeId),
      [...documents.filter((item) => item.id !== promotedPreview.id), promotedPreview],
      localMedia,
    );
    nodesRef.current = nextFlow.nodes;
    edgesRef.current = nextFlow.edges;
    setNodes(nextFlow.nodes);
    setEdges(nextFlow.edges);
    setSelectedNodeIds([node.id]);
    setSelectedEdgeIds([]);
  }, [
    activeView,
    canEdit,
    documents,
    localMedia,
    onPromoteNode,
    promotionEnabled,
    selectedNodeIds,
    viewFilterTypeId,
  ]);

  const changeZOrder = useCallback(
    (front: boolean) => {
      if (!canEdit || selectedNodeIds.length === 0) return;
      const selected = new Set(selectedNodeIds);
      const groups = historyRef.current.present.nodes.filter(isSpatialGroup);
      const cards = historyRef.current.present.nodes.filter((item) => !isSpatialGroup(item));
      const reorder = (items: CanvasNode[]) => [
        ...items.filter((item) => (front ? !selected.has(item.id) : selected.has(item.id))),
        ...items.filter((item) => (front ? selected.has(item.id) : !selected.has(item.id))),
      ];
      commit({ ...historyRef.current.present, nodes: [...reorder(groups), ...reorder(cards)] });
    },
    [canEdit, commit, selectedNodeIds],
  );

  const rotateSelection = useCallback(() => {
    if (!canEdit || selectedNodeIds.length === 0) return;
    void requestInput({
      title: "Повернуть фигуры",
      label: "Угол в градусах",
      defaultValue: "15",
      confirmLabel: "Повернуть",
    }).then((raw) => {
      if (raw === undefined) return;
      const degrees = Number(raw.replace(",", "."));
      if (!Number.isFinite(degrees)) {
        showToast({ title: "Введите корректный угол" });
        return;
      }
      commit(rotateCanvasSelection(historyRef.current.present, selectedNodeIds, degrees));
    });
  }, [canEdit, commit, requestInput, selectedNodeIds, showToast]);

  const handleUndo = useCallback(() => {
    const next = undoCanvasHistory(historyRef.current);
    if (next !== historyRef.current) applyHistory(next);
  }, [applyHistory]);
  const handleRedo = useCallback(() => {
    const next = redoCanvasHistory(historyRef.current);
    if (next !== historyRef.current) applyHistory(next);
  }, [applyHistory]);

  const beginGesture = useCallback(
    (event: ReactPointerEvent<HTMLDivElement>) => {
      if (!canEdit || !isDrawingTool(tool) || event.button !== 0) return;
      if (
        !(event.target instanceof HTMLElement) ||
        !event.target.classList.contains("react-flow__pane")
      ) {
        return;
      }
      if (event.pointerType === "touch") return;
      if (gestureRef.current) return;
      const position = reactFlow.screenToFlowPosition({ x: event.clientX, y: event.clientY });
      const next: CanvasGesture = {
        pointerId: event.pointerId,
        pointerType: event.pointerType,
        tool,
        points: [
          {
            ...position,
            ...(event.pointerType === "pen" ? { pressure: event.pressure } : {}),
          },
        ],
      };
      event.preventDefault();
      event.currentTarget.setPointerCapture(event.pointerId);
      gestureRef.current = next;
      setGesture(next);
    },
    [canEdit, reactFlow, tool],
  );

  const moveGesture = useCallback(
    (event: ReactPointerEvent<HTMLDivElement>) => {
      const current = gestureRef.current;
      if (!current || current.pointerId !== event.pointerId) return;
      if (current.pointerType === "pen" && event.pointerType === "touch") return;
      const coalesced = event.nativeEvent.getCoalescedEvents?.() ?? [event.nativeEvent];
      let points = current.points;
      for (const pointer of coalesced) {
        const position = reactFlow.screenToFlowPosition({ x: pointer.clientX, y: pointer.clientY });
        points = appendSampledPoint(points, {
          ...position,
          ...(pointer.pointerType === "pen" ? { pressure: pointer.pressure } : {}),
        });
      }
      if (points === current.points) return;
      const next = { ...current, points };
      gestureRef.current = next;
      setGesture(next);
    },
    [reactFlow],
  );

  const finishGesture = useCallback(
    (event: ReactPointerEvent<HTMLDivElement>) => {
      const current = gestureRef.current;
      if (!current || current.pointerId !== event.pointerId) return;
      const position = reactFlow.screenToFlowPosition({ x: event.clientX, y: event.clientY });
      const points = appendSampledPoint(current.points, {
        ...position,
        ...(event.pointerType === "pen" ? { pressure: event.pressure } : {}),
      });
      gestureRef.current = undefined;
      setGesture(undefined);
      if (event.currentTarget.hasPointerCapture(event.pointerId)) {
        event.currentTarget.releasePointerCapture(event.pointerId);
      }
      if (current.tool === "eraser") {
        commit(
          eraseCanvasElements(historyRef.current.present, points, 16, () => crypto.randomUUID()),
          [],
        );
        return;
      }
      const node =
        current.tool === "pen" || current.tool === "highlighter"
          ? createStrokeNode(crypto.randomUUID(), points, current.tool, {
              color: "5",
              size: current.tool === "highlighter" ? 18 : 4,
              opacity: current.tool === "highlighter" ? 0.32 : 1,
            })
          : isShapeTool(current.tool)
            ? createShapeNode(
                crypto.randomUUID(),
                points[0] ?? position,
                points.at(-1) ?? position,
                current.tool,
                { color: "5", size: 3, opacity: 1 },
              )
            : undefined;
      if (!node) return;
      commit(
        { ...historyRef.current.present, nodes: [...historyRef.current.present.nodes, node] },
        [node.id],
      );
      setSelectedNodeIds([node.id]);
    },
    [commit, reactFlow],
  );

  const cancelGesture = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    if (gestureRef.current?.pointerId !== event.pointerId) return;
    gestureRef.current = undefined;
    setGesture(undefined);
  }, []);

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
      if (clipboard.current) {
        event.preventDefault();
        pasteSelection();
      }
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
    } else if (event.code === "Space" && !event.repeat) {
      event.preventDefault();
      previousTool.current = tool;
      setTool("hand");
    } else if (!command) {
      const shortcut: Partial<Record<string, CanvasTool>> = {
        v: "select",
        h: "hand",
        n: "note",
        c: "connector",
        p: "pen",
        m: "highlighter",
        e: "eraser",
        r: "rectangle",
        o: "ellipse",
        d: "diamond",
        l: "line",
        a: "arrow",
      };
      const next = shortcut[event.key.toLowerCase()];
      if (next) {
        event.preventDefault();
        setTool(next);
      }
    }
  };

  const onKeyUp = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.code !== "Space" || !previousTool.current) return;
    setTool(previousTool.current);
    previousTool.current = undefined;
  };

  const onPaste = (event: ClipboardEvent<HTMLDivElement>) => {
    if (!canEdit || isFormTarget(event.target) || clipboard.current) return;
    const text = event.clipboardData.getData("text/plain").trim();
    if (!text) return;
    event.preventDefault();
    const position = reactFlow.screenToFlowPosition({
      x: window.innerWidth / 2,
      y: window.innerHeight / 2,
    });
    if (/^https?:\/\//i.test(text)) addLink(text, position);
    else addText(position, text);
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
    const droppedURL = event.dataTransfer.getData("text/uri-list").split("\n")[0]?.trim();
    if (droppedURL && /^https?:\/\//i.test(droppedURL)) {
      addLink(droppedURL, position);
      return;
    }
    const mediaFile = [...event.dataTransfer.files].find(
      (file) => file.type.startsWith("image/") || file.type === "application/pdf",
    );
    if (mediaFile) void addMedia(mediaFile, position);
  };

  const contextValue = useMemo<BoardContextValue>(
    () => ({
      editable: canEdit,
      typeSchema,
      updateNode,
      editEdgeLabel,
    }),
    [canEdit, editEdgeLabel, typeSchema, updateNode],
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
      onAssistantContextChange?.({
        selectionIds: nextNodeIds,
        viewport: reactFlow.getViewport(),
      });
    },
    [onAssistantContextChange, reactFlow],
  );

  const exportCanvas = useCallback(() => {
    const current = historyRef.current.present;
    const selectedFrames = current.nodes.filter(
      (node) =>
        selectedNodeIds.includes(node.id) && node["x-writers-studio"]?.element?.kind === "frame",
    );
    const scopeOptions = [
      { value: "all", label: "Вся доска" },
      ...(selectedNodeIds.length > 0
        ? [{ value: "selection", label: `Выделение (${selectedNodeIds.length})` }]
        : []),
      ...selectedFrames.map((frame) => ({
        value: `frame:${frame.id}`,
        label: `Фрейм: ${frame.label || "Без названия"}`,
      })),
    ];

    void requestSelect({
      title: "Экспорт доски",
      label: "Область",
      options: scopeOptions,
      confirmLabel: "Далее",
    }).then(async (scopeValue) => {
      if (!scopeValue) return;
      const format = await requestSelect({
        title: "Экспорт доски",
        label: "Формат",
        options: [
          { value: "png", label: "PNG — изображение" },
          { value: "svg", label: "SVG — вектор" },
          { value: "pdf", label: "PDF — документ" },
          { value: "canvas", label: ".canvas — редактируемый JSON Canvas" },
        ],
        confirmLabel: "Экспортировать",
      });
      if (format !== "png" && format !== "svg" && format !== "pdf" && format !== "canvas") return;
      if (format === "canvas") {
        const blob = new Blob([JSON.stringify(current, null, 2)], { type: "application/json" });
        const url = URL.createObjectURL(blob);
        const anchor = document.createElement("a");
        anchor.href = url;
        anchor.download = canvasExportFileName(title, format);
        anchor.click();
        URL.revokeObjectURL(url);
        showToast({ title: "JSON Canvas экспортирован" });
        return;
      }

      const frameId = scopeValue.startsWith("frame:") ? scopeValue.slice(6) : undefined;
      const scope = frameId ? "frame" : scopeValue === "selection" ? "selection" : "all";
      const nodeIds = canvasExportNodeIds(current, { scope, selectedNodeIds, frameId });
      const bounds = canvasExportBounds(current, nodeIds);
      const viewport = board.current?.querySelector<HTMLElement>(".react-flow__viewport");
      if (!bounds || !viewport || !board.current) {
        showToast({ title: "В выбранной области нет элементов" });
        return;
      }

      setExportPending(true);
      try {
        const dimensions = canvasExportDimensions(bounds);
        await exportCanvasViewport({
          viewport,
          bounds,
          ...dimensions,
          backgroundColor: getComputedStyle(board.current).backgroundColor,
          fileName: canvasExportFileName(title, format),
          format,
        });
        showToast({ title: `Доска экспортирована в ${format.toUpperCase()}` });
      } catch (error) {
        showToast({
          title: "Не удалось экспортировать доску",
          description: error instanceof Error ? error.message : undefined,
        });
      } finally {
        setExportPending(false);
      }
    });
  }, [requestSelect, selectedNodeIds, showToast, title]);

  return (
    <BoardContext.Provider value={contextValue}>
      <div
        ref={board}
        data-testid="canvas-board"
        role="application"
        aria-label="Canvas-доска"
        className="relative size-full min-h-0 bg-[var(--surface-app)] outline-none"
        onKeyDown={onKeyDown}
        onKeyUp={onKeyUp}
        onPaste={onPaste}
        onPointerDown={(event) => {
          if (
            canEdit &&
            tool === "note" &&
            event.target instanceof HTMLElement &&
            event.target.classList.contains("react-flow__pane")
          ) {
            addText(reactFlow.screenToFlowPosition({ x: event.clientX, y: event.clientY }));
            setTool("select");
            return;
          }
          beginGesture(event);
        }}
        onPointerMove={moveGesture}
        onPointerUp={finishGesture}
        onPointerCancel={cancelGesture}
        onDoubleClick={(event) => {
          if (
            canEdit &&
            tool === "select" &&
            event.target instanceof HTMLElement &&
            event.target.classList.contains("react-flow__pane")
          ) {
            addText(reactFlow.screenToFlowPosition({ x: event.clientX, y: event.clientY }));
          }
        }}
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
          snapToGrid={!snappingDisabled}
          snapGrid={[20, 20]}
          connectionMode={ConnectionMode.Loose}
          minZoom={0.12}
          maxZoom={3}
          fitView
          fitViewOptions={{ padding: 0.2 }}
          nodesDraggable={canEdit && tool === "select" && presentationIndex === undefined}
          nodesConnectable={canEdit && (tool === "select" || tool === "connector")}
          nodesFocusable
          edgesFocusable
          elementsSelectable={
            presentationIndex === undefined && (tool === "select" || tool === "connector")
          }
          selectionOnDrag={canEdit && tool === "select"}
          elevateNodesOnSelect={false}
          panOnDrag={tool === "hand" || !canEdit ? true : [1, 2]}
          deleteKeyCode={null}
          onNodesChange={(changes: NodeChange<CanvasFlowNode>[]) => {
            const positionChanges = changes.filter(
              (change) => change.type === "position" && change.position,
            );
            const adjustedChanges =
              positionChanges.length === 1 && !snappingDisabled
                ? changes.map((change) => {
                    if (change.type !== "position" || !change.position) return change;
                    const moving = nodesRef.current.find((node) => node.id === change.id);
                    const movingItem = moving?.data.canvasNode;
                    if (!moving || !movingItem || isSpatialGroup(movingItem)) return change;
                    const guides = snapCanvasNodeWithGuides(
                      {
                        id: moving.id,
                        x: change.position.x,
                        y: change.position.y,
                        width: movingItem.width,
                        height: movingItem.height,
                      },
                      nodesRef.current
                        .filter(
                          (node) => node.id !== moving.id && !isSpatialGroup(node.data.canvasNode),
                        )
                        .map((node) => ({
                          id: node.id,
                          x: node.position.x,
                          y: node.position.y,
                          width: node.data.canvasNode.width,
                          height: node.data.canvasNode.height,
                        })),
                    );
                    setSnapGuides(guides);
                    return { ...change, position: guides.position };
                  })
                : changes;
            if (positionChanges.length !== 1 || snappingDisabled) setSnapGuides(undefined);
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
            setSnapGuides(undefined);
            dragStart.current = { id: node.id, x: node.position.x, y: node.position.y };
          }}
          onNodeDragStop={(_, node) => {
            setSnapGuides(undefined);
            const original = historyRef.current.present.nodes.find((item) => item.id === node.id);
            if (original && isSpatialGroup(original) && dragStart.current?.id === node.id) {
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
          onMoveEnd={() =>
            onAssistantContextChange?.({
              selectionIds: selectedNodeIds,
              viewport: reactFlow.getViewport(),
            })
          }
          onNodeClick={(_, node) => {
            // A phone has no practical double-click gesture and remains a
            // read-only viewer. Tablet and desktop always reserve one click
            // for selection and open resources only on double-click.
            if (tablet) return;
            const item = node.data.canvasNode;
            if (item.type === "file" && item["x-writers-studio"]?.resource?.kind === "document") {
              onOpenDocument(item["x-writers-studio"].resource.id);
            } else if (
              item.type === "file" &&
              item["x-writers-studio"]?.resource?.kind === "canvas"
            ) {
              onOpenCanvas?.(item["x-writers-studio"].resource.id);
            } else if (item.type === "link") {
              window.open(item.url, "_blank", "noopener,noreferrer");
            } else if (item.type === "file" && node.data.media?.mediaType === "application/pdf") {
              window.open(node.data.media.contentUrl, "_blank", "noopener,noreferrer");
            }
          }}
          onNodeContextMenu={(event, node) => {
            if (!canEdit) return;
            event.preventDefault();
            event.stopPropagation();
            const nextNodes = nodesRef.current.map((item) => ({
              ...item,
              selected: item.id === node.id,
            }));
            const nextEdges = edgesRef.current.map((item) => ({ ...item, selected: false }));
            nodesRef.current = nextNodes;
            edgesRef.current = nextEdges;
            setNodes(nextNodes);
            setEdges(nextEdges);
            setSelectedNodeIds([node.id]);
            setSelectedEdgeIds([]);
            nodeMenu.open({
              nodeId: node.id,
              x: Math.max(8, Math.min(event.clientX, window.innerWidth - 232)),
              y: Math.max(8, Math.min(event.clientY, window.innerHeight - 204)),
            });
          }}
          onNodeDoubleClick={(event: MouseEvent, node) => {
            event.preventDefault();
            event.stopPropagation();
            const item = node.data.canvasNode;
            if (item.type === "file" && item["x-writers-studio"]?.resource?.kind === "document") {
              onOpenDocument(item["x-writers-studio"].resource.id);
            } else if (
              item.type === "file" &&
              item["x-writers-studio"]?.resource?.kind === "canvas"
            ) {
              onOpenCanvas?.(item["x-writers-studio"].resource.id);
            } else if (item.type === "link") {
              window.open(item.url, "_blank", "noopener,noreferrer");
            } else if (item.type === "file" && node.data.media?.mediaType === "application/pdf") {
              window.open(node.data.media.contentUrl, "_blank", "noopener,noreferrer");
            }
          }}
        >
          {gesture ? (
            <ViewportPortal>
              <GesturePreview gesture={gesture} />
            </ViewportPortal>
          ) : null}
          {layoutPreview ? (
            <ViewportPortal>
              <LayoutPreview source={history.present} preview={layoutPreview} />
            </ViewportPortal>
          ) : null}
          {snapGuides ? (
            <ViewportPortal>
              <SnapGuideOverlay guides={snapGuides} />
            </ViewportPortal>
          ) : null}
          <Background gap={20} size={1} color="var(--border-subtle)" />
          <MiniMap
            pannable
            zoomable
            position="bottom-left"
            className="!border !border-[var(--border-subtle)] !bg-[var(--surface-raised)]"
            maskColor="color-mix(in srgb, var(--surface-app) 70%, transparent)"
            nodeColor="var(--accent-soft)"
          />
          <Controls
            showInteractive={false}
            position="bottom-right"
            className="!overflow-hidden !rounded-[var(--radius-sm)] !border !border-[var(--border-subtle)]"
          />
          {layoutPreview ? (
            <Panel
              position="top-right"
              className="!m-3 flex items-center gap-2 rounded-[12px] border border-[var(--info)] bg-[var(--surface-raised)] p-2 shadow-lg"
            >
              <span className="px-2 text-xs text-[var(--ink-secondary)]">Preview раскладки</span>
              <Button
                type="button"
                variant="ghost"
                size="compact"
                onClick={() => setLayoutPreview(undefined)}
              >
                Отмена
              </Button>
              <Button
                type="button"
                size="compact"
                onClick={() => {
                  commit(layoutPreview, selectedNodeIds);
                  setLayoutPreview(undefined);
                }}
              >
                Применить
              </Button>
            </Panel>
          ) : null}
          {playthrough && !layoutPreview ? (
            <Panel
              position="top-center"
              className="!mt-16 w-[min(420px,calc(100vw-32px))] rounded-[12px] border border-[var(--border-subtle)] bg-[var(--surface-raised)] p-3 shadow-lg"
            >
              <div role="dialog" aria-label="Ручной play-through">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <span className="text-[10px] font-semibold uppercase tracking-wide text-[var(--ink-tertiary)]">
                      {playthrough.profile} · шаг {playthrough.path.length}
                    </span>
                    <strong className="mt-1 block truncate text-sm">
                      {currentPlaythroughObject?.title ?? playthrough.path.at(-1)}
                    </strong>
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="size-8"
                    aria-label="Закрыть play-through"
                    onClick={() => setPlaythrough(undefined)}
                  >
                    <X className="size-4" aria-hidden="true" />
                  </Button>
                </div>
                {playthroughChoices.length ? (
                  <div className="mt-3 grid gap-2">
                    {playthroughChoices.map((choice) => (
                      <Button
                        key={choice.edgeId}
                        type="button"
                        variant="secondary"
                        className="h-auto min-h-9 justify-start py-2 text-left"
                        onClick={() => advancePlaythrough(choice)}
                      >
                        <Route className="size-3.5 shrink-0" aria-hidden="true" />
                        <span>
                          {choice.label}
                          {choice.condition ? (
                            <small className="ml-2 text-[var(--ink-tertiary)]">
                              если {String(choice.condition)}
                            </small>
                          ) : null}
                        </span>
                      </Button>
                    ))}
                  </div>
                ) : (
                  <p className="mt-3 rounded-lg bg-[var(--success-soft)] px-3 py-2 text-xs text-[var(--success)]">
                    Конечная точка сценария
                  </p>
                )}
              </div>
            </Panel>
          ) : null}
          {diagnostics && !layoutPreview ? (
            <Panel
              position="top-right"
              className="!mx-3 !mb-3 !mt-16 max-h-[min(70vh,520px)] w-80 overflow-auto rounded-[12px] border border-[var(--border-subtle)] bg-[var(--surface-raised)] p-0 shadow-lg"
            >
              <div
                ref={diagnosticsPanel.panelRef}
                role="dialog"
                aria-label="Диагностика графа"
                className="p-3"
              >
                <div className="mb-2 flex items-center justify-between gap-2">
                  <strong className="text-sm">Диагностика графа</strong>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="size-8"
                    aria-label="Закрыть диагностику"
                    title="Закрыть"
                    onClick={diagnosticsPanel.close}
                  >
                    <X className="size-4" aria-hidden="true" />
                  </Button>
                </div>
                {diagnostics.length === 0 ? (
                  <p className="text-xs text-[var(--ink-secondary)]">Предупреждений нет.</p>
                ) : (
                  <ul className="space-y-1">
                    {diagnostics.map((diagnostic) => (
                      <li
                        key={`${diagnostic.code}:${diagnostic.nodeIds.join(",")}:${diagnostic.edgeIds.join(",")}`}
                      >
                        <button
                          type="button"
                          className="w-full rounded-lg px-3 py-2 text-left text-xs hover:bg-[var(--surface-subtle)]"
                          onClick={() => {
                            const selected = new Set(diagnostic.nodeIds);
                            setSelectedNodeIds(diagnostic.nodeIds);
                            setSelectedEdgeIds(diagnostic.edgeIds);
                            setNodes((current) =>
                              current.map((node) => ({ ...node, selected: selected.has(node.id) })),
                            );
                            void reactFlow.fitView({
                              nodes: nodesRef.current.filter((node) => selected.has(node.id)),
                              padding: 0.4,
                              duration: 250,
                            });
                          }}
                        >
                          <span className="font-semibold">{diagnostic.message}</span>
                          <span className="mt-1 block text-[var(--ink-tertiary)]">
                            {diagnostic.nodeIds.length} элементов
                          </span>
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </Panel>
          ) : null}
          {navigatorPanel.isOpen && !layoutPreview ? (
            <Panel
              position="top-left"
              className="!mx-3 !mb-3 !mt-16 w-80 rounded-[12px] border border-[var(--border-subtle)] bg-[var(--surface-raised)] p-0 shadow-lg"
            >
              <div
                ref={navigatorPanel.panelRef}
                role="dialog"
                aria-label="Объекты доски"
                className="p-3"
              >
                <div className="mb-2 flex items-center justify-between gap-2">
                  <strong className="text-sm">Объекты доски</strong>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="size-8"
                    aria-label="Закрыть список объектов"
                    onClick={navigatorPanel.close}
                  >
                    <X className="size-4" aria-hidden="true" />
                  </Button>
                </div>
                <label className="relative block">
                  <Search
                    className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-[var(--ink-tertiary)]"
                    aria-hidden="true"
                  />
                  <input
                    ref={navigatorInput}
                    value={navigatorQuery}
                    onChange={(event) => setNavigatorQuery(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key !== "ArrowDown") return;
                      event.preventDefault();
                      navigatorItems.current[0]?.focus();
                    }}
                    className="h-9 w-full rounded-lg border border-[var(--border-subtle)] bg-[var(--surface-app)] pl-9 pr-3 text-sm outline-none focus:border-[var(--accent)]"
                    placeholder="Название, текст или тип…"
                    aria-label="Поиск по доске"
                  />
                </label>
                <p className="my-2 text-[11px] text-[var(--ink-tertiary)]">
                  {filteredCanvasObjects.length} из {canvasObjects.length}
                </p>
                <ul className="max-h-[min(58vh,440px)] space-y-1 overflow-auto">
                  {filteredCanvasObjects.map((object, index) => (
                    <li key={object.id}>
                      <button
                        ref={(element) => {
                          navigatorItems.current[index] = element;
                        }}
                        type="button"
                        className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-xs hover:bg-[var(--surface-subtle)] focus:bg-[var(--surface-subtle)] focus:outline-none"
                        onClick={() => focusCanvasObject(object.id)}
                        onKeyDown={(event) => {
                          if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;
                          event.preventDefault();
                          const direction = event.key === "ArrowDown" ? 1 : -1;
                          const next = Math.max(
                            0,
                            Math.min(filteredCanvasObjects.length - 1, index + direction),
                          );
                          navigatorItems.current[next]?.focus();
                        }}
                      >
                        <span className="min-w-0 flex-1 truncate font-semibold">
                          {object.title}
                        </span>
                        <span className="shrink-0 text-[10px] uppercase text-[var(--ink-tertiary)]">
                          {object.kind}
                        </span>
                      </button>
                    </li>
                  ))}
                </ul>
              </div>
            </Panel>
          ) : null}
          {presentationIndex !== undefined && currentPresentationFrame ? (
            <Panel
              position="top-center"
              className="!mt-16 flex items-center gap-1 rounded-full border border-[var(--border-subtle)] bg-[var(--surface-raised)] p-1.5 shadow-lg"
            >
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label="Предыдущий фрейм"
                disabled={presentationIndex === 0}
                onClick={() => showPresentationFrame(presentationIndex - 1)}
              >
                <ChevronLeft className="size-4" aria-hidden="true" />
              </Button>
              <span className="max-w-64 truncate px-2 text-xs font-semibold">
                {presentationIndex + 1}/{presentationFrames.length} · {currentPresentationLabel}
              </span>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label="Следующий фрейм"
                disabled={presentationIndex === presentationFrames.length - 1}
                onClick={() => showPresentationFrame(presentationIndex + 1)}
              >
                <ChevronRight className="size-4" aria-hidden="true" />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label="Завершить презентацию"
                onClick={() => setPresentationIndex(undefined)}
              >
                <X className="size-4" aria-hidden="true" />
              </Button>
            </Panel>
          ) : null}
          {focusEndsAt !== undefined ? (
            <Panel
              position="bottom-center"
              className="flex items-center gap-2 rounded-full border border-[var(--accent)] bg-[var(--surface-raised)] px-3 py-1.5 shadow-lg"
            >
              <TimerReset className="size-4 text-[var(--accent-ink)]" aria-hidden="true" />
              <span className="font-mono text-sm font-semibold">
                {formatCanvasFocusTime(focusRemaining)}
              </span>
              <Button
                type="button"
                variant="ghost"
                size="compact"
                onClick={() => setFocusEndsAt(undefined)}
              >
                Завершить
              </Button>
            </Panel>
          ) : null}
          <Panel position="top-left" className="!m-3 !overflow-visible">
            <div
              ref={toolRail.railRef}
              aria-label="Инструменты Canvas"
              aria-orientation="vertical"
              className="relative flex max-h-[calc(100vh-104px)] w-11 flex-col items-center gap-1 overflow-visible rounded-[12px] border border-[var(--border-subtle)] bg-[var(--surface-raised)] p-1.5 shadow-lg"
              role="toolbar"
            >
              {canEdit ? (
                <>
                  <ToolButton
                    icon={MousePointer2}
                    label="Выбор (V)"
                    active={tool === "select"}
                    onClick={() => setTool("select")}
                  />
                  <ToolButton
                    icon={Hand}
                    label="Рука (H)"
                    active={tool === "hand"}
                    onClick={() => setTool("hand")}
                  />
                  <span className="my-1 h-px w-6 shrink-0 bg-[var(--border-subtle)]" />
                  <ToolRailGroupButton
                    icon={StickyNote}
                    label="Добавить"
                    expanded={toolRail.openMenu === "create"}
                    active={tool === "connector" || tool === "note"}
                    onClick={() => toolRail.toggleMenu("create")}
                  />
                  {toolRail.openMenu === "create" ? (
                    <ToolFlyout label="Добавить на доску">
                      <ToolButton
                        icon={Link2}
                        label="Связь (C)"
                        active={tool === "connector"}
                        onClick={() => {
                          setTool("connector");
                          toolRail.closeMenu();
                        }}
                      />
                      <ToolButton
                        icon={StickyNote}
                        label="Заметка (N)"
                        active={tool === "note"}
                        onClick={() => {
                          setTool("note");
                          toolRail.closeMenu();
                        }}
                      />
                      <ToolButton
                        icon={LayoutTemplate}
                        label="Шаблон"
                        onClick={() => {
                          toolRail.closeMenu();
                          addTemplate();
                        }}
                      />
                      <ToolButton
                        icon={FilePlus2}
                        label="Документ"
                        onClick={() => {
                          toolRail.closeMenu();
                          addDocument();
                        }}
                      />
                      <ToolButton
                        icon={ImagePlus}
                        label="Изображение, PDF, аудио или видео"
                        onClick={() => {
                          toolRail.closeMenu();
                          fileInput.current?.click();
                        }}
                      />
                    </ToolFlyout>
                  ) : null}
                  <ToolRailGroupButton
                    icon={PenLine}
                    label="Рисование"
                    expanded={toolRail.openMenu === "draw"}
                    active={isDrawingTool(tool)}
                    onClick={() => toolRail.toggleMenu("draw")}
                  />
                  {toolRail.openMenu === "draw" ? (
                    <ToolFlyout label="Рисование и фигуры">
                      <ToolButton
                        icon={PenLine}
                        label="Перо (P)"
                        active={tool === "pen"}
                        onClick={() => setTool("pen")}
                      />
                      <ToolButton
                        icon={Highlighter}
                        label="Маркер (M)"
                        active={tool === "highlighter"}
                        onClick={() => setTool("highlighter")}
                      />
                      <ToolButton
                        icon={Eraser}
                        label="Ластик (E)"
                        active={tool === "eraser"}
                        onClick={() => setTool("eraser")}
                      />
                      <ToolButton
                        icon={Square}
                        label="Прямоугольник (R)"
                        active={tool === "rectangle"}
                        onClick={() => setTool("rectangle")}
                      />
                      <ToolButton
                        icon={Circle}
                        label="Эллипс (O)"
                        active={tool === "ellipse"}
                        onClick={() => setTool("ellipse")}
                      />
                      <ToolButton
                        icon={Diamond}
                        label="Ромб (D)"
                        active={tool === "diamond"}
                        onClick={() => setTool("diamond")}
                      />
                      <ToolButton
                        icon={Minus}
                        label="Линия (L)"
                        active={tool === "line"}
                        onClick={() => setTool("line")}
                      />
                      <ToolButton
                        icon={ArrowRight}
                        label="Стрелка (A)"
                        active={tool === "arrow"}
                        onClick={() => setTool("arrow")}
                      />
                    </ToolFlyout>
                  ) : null}
                  <span className="my-1 h-px w-6 shrink-0 bg-[var(--border-subtle)]" />
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
                  <ToolRailGroupButton
                    icon={LayoutGrid}
                    label="Компоновка"
                    expanded={toolRail.openMenu === "arrange"}
                    onClick={() => toolRail.toggleMenu("arrange")}
                  />
                  {toolRail.openMenu === "arrange" ? (
                    <ToolFlyout label="Компоновка и порядок">
                      <ToolButton
                        icon={Group}
                        label="Группа"
                        onClick={groupSelection}
                        disabled={selectedNodeIds.length === 0}
                      />
                      <ToolButton
                        icon={PanelsTopLeft}
                        label="Фрейм"
                        onClick={frameSelection}
                        disabled={selectedNodeIds.length === 0}
                      />
                      <ToolButton
                        icon={AlignCenter}
                        label="Выровнять"
                        onClick={alignSelection}
                        disabled={selectedNodeIds.length < 2}
                      />
                      <ToolButton
                        icon={RotateCw}
                        label="Повернуть"
                        onClick={rotateSelection}
                        disabled={
                          !history.present.nodes.some(
                            (node) =>
                              selectedNodeIds.includes(node.id) &&
                              node["x-writers-studio"]?.element?.kind === "shape",
                          )
                        }
                      />
                      <ToolButton
                        icon={Columns3}
                        label="Интервалы"
                        onClick={distributeSelection}
                        disabled={selectedNodeIds.length < 3}
                      />
                      <ToolButton
                        icon={LayoutGrid}
                        label="Упорядочить"
                        onClick={tidySelection}
                        disabled={selectedNodeIds.length < 2}
                      />
                      <ToolButton
                        icon={Workflow}
                        label={layoutPending ? "Раскладка…" : "Авто-раскладка"}
                        onClick={previewAutomaticLayout}
                        disabled={layoutPending || history.present.nodes.length < 2}
                      />
                    </ToolFlyout>
                  ) : null}
                  <ToolRailGroupButton
                    icon={Layers3}
                    label="Режимы и анализ"
                    expanded={toolRail.openMenu === "explore"}
                    active={
                      activeView !== "free" ||
                      diagnosticsPanel.isOpen ||
                      navigatorPanel.isOpen ||
                      presentationIndex !== undefined ||
                      playthrough !== undefined ||
                      focusEndsAt !== undefined
                    }
                    onClick={() => toolRail.toggleMenu("explore")}
                  />
                  {toolRail.openMenu === "explore" ? (
                    <ToolFlyout label="Режимы, поиск и анализ">
                      <ToolButton
                        icon={Layers3}
                        label={`Вид: ${activeView}`}
                        active={activeView !== "free"}
                        onClick={chooseCanvasView}
                      />
                      <span ref={diagnosticsPanel.triggerRef} className="contents">
                        <ToolButton
                          icon={Activity}
                          label="Диагностика"
                          active={diagnosticsPanel.isOpen}
                          onClick={runGraphAnalysis}
                        />
                      </span>
                      <span ref={navigatorPanel.triggerRef} className="contents">
                        <ToolButton
                          icon={ListTree}
                          label="Объекты и поиск"
                          active={navigatorPanel.isOpen}
                          onClick={() =>
                            navigatorPanel.isOpen
                              ? navigatorPanel.close()
                              : navigatorPanel.open(true)
                          }
                        />
                      </span>
                      <ToolButton
                        icon={Play}
                        label="Презентация"
                        active={presentationIndex !== undefined}
                        disabled={presentationFrames.length === 0}
                        onClick={() =>
                          presentationIndex === undefined
                            ? showPresentationFrame(0)
                            : setPresentationIndex(undefined)
                        }
                      />
                      <ToolButton
                        icon={Route}
                        label="Play-through"
                        active={playthrough !== undefined}
                        onClick={() =>
                          playthrough === undefined ? startPlaythrough() : setPlaythrough(undefined)
                        }
                      />
                      <ToolButton
                        icon={TimerReset}
                        label="Фокус-таймер"
                        active={focusEndsAt !== undefined}
                        onClick={() =>
                          focusEndsAt === undefined ? startFocusTimer() : setFocusEndsAt(undefined)
                        }
                      />
                    </ToolFlyout>
                  ) : null}
                  <ToolRailGroupButton
                    icon={Tags}
                    label="Действия с выделением"
                    expanded={toolRail.openMenu === "selection"}
                    active={selectedNodeIds.length > 0 || selectedEdgeIds.length > 0}
                    onClick={() => toolRail.toggleMenu("selection")}
                  />
                  {toolRail.openMenu === "selection" ? (
                    <ToolFlyout label="Действия с выделением">
                      <ToolButton
                        icon={Tags}
                        label="Тип карточки"
                        onClick={assignCardType}
                        disabled={selectedNodeIds.length === 0}
                      />
                      <ToolButton
                        icon={FileOutput}
                        label={promotionEnabled ? "В документ" : "Сначала сохраните доску"}
                        onClick={() => void promoteSelection()}
                        disabled={
                          !promotionEnabled ||
                          selectedNodeIds.length !== 1 ||
                          !history.present.nodes.some(
                            (node) =>
                              node.id === selectedNodeIds[0] &&
                              node.type !== "file" &&
                              !(node.type === "group" && node["x-writers-studio"]?.element),
                          )
                        }
                      />
                      <ToolButton
                        icon={GitBranch}
                        label="Тип связи"
                        onClick={assignRelationType}
                        disabled={selectedEdgeIds.length === 0}
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
                        onClick={() => {
                          changeZOrder(true);
                          toolRail.closeMenu();
                        }}
                        disabled={selectedNodeIds.length === 0}
                      />
                      <ToolButton
                        icon={ArrowDownToLine}
                        label="На задний план"
                        onClick={() => {
                          changeZOrder(false);
                          toolRail.closeMenu();
                        }}
                        disabled={selectedNodeIds.length === 0}
                      />
                      {(selectedNodeIds.length > 0 || selectedEdgeIds.length > 0) && (
                        <fieldset className="mt-1 grid grid-cols-7 gap-1 border-t border-[var(--border-subtle)] pt-2">
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
                    </ToolFlyout>
                  ) : null}
                </>
              ) : editable && tablet ? (
                <>
                  <span className="sr-only">Проекционный вид · координаты Canvas не меняются</span>
                  <ToolButton
                    icon={Layers3}
                    label={`Вид: ${activeView}`}
                    active
                    onClick={chooseCanvasView}
                  />
                  <ToolButton
                    icon={Activity}
                    label="Диагностика"
                    active={diagnosticsPanel.isOpen}
                    onClick={runGraphAnalysis}
                  />
                  <ToolButton
                    icon={Route}
                    label="Play-through"
                    active={playthrough !== undefined}
                    onClick={() =>
                      playthrough === undefined ? startPlaythrough() : setPlaythrough(undefined)
                    }
                  />
                </>
              ) : (
                <span className="sr-only">На телефоне доступен просмотр доски</span>
              )}
              <span className="my-1 h-px w-6 shrink-0 bg-[var(--border-subtle)]" />
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
              <ToolButton
                icon={Download}
                label={exportPending ? "Экспорт…" : "Экспорт"}
                disabled={exportPending || history.present.nodes.length === 0}
                onClick={exportCanvas}
              />
            </div>
          </Panel>
        </ReactFlow>
        {nodeMenu.value ? (
          <div
            ref={nodeMenu.panelRef}
            role="menu"
            aria-label="Действия с карточкой"
            className="fixed z-[120] grid w-56 gap-1 rounded-[10px] border border-[var(--border-strong)] bg-[var(--surface-raised)] p-1.5 shadow-[0_16px_36px_var(--shadow-color)]"
            style={{ left: nodeMenu.value.x, top: nodeMenu.value.y }}
          >
            <Button
              type="button"
              role="menuitem"
              variant="ghost"
              size="compact"
              className="w-full justify-start"
              onClick={() => {
                duplicateSelection();
                nodeMenu.close();
              }}
            >
              <Copy className="size-4" aria-hidden="true" />
              Дублировать
            </Button>
            <Button
              type="button"
              role="menuitem"
              variant="ghost"
              size="compact"
              className="w-full justify-start"
              onClick={() => {
                changeZOrder(true);
                nodeMenu.close();
              }}
            >
              <ArrowUpToLine className="size-4" aria-hidden="true" />
              На передний план
            </Button>
            <Button
              type="button"
              role="menuitem"
              variant="ghost"
              size="compact"
              className="w-full justify-start"
              onClick={() => {
                changeZOrder(false);
                nodeMenu.close();
              }}
            >
              <ArrowDownToLine className="size-4" aria-hidden="true" />
              На задний план
            </Button>
            <div className="my-0.5 h-px bg-[var(--border-subtle)]" aria-hidden="true" />
            <Button
              type="button"
              role="menuitem"
              variant="ghost"
              size="compact"
              className="w-full justify-start text-[var(--danger)]"
              onClick={() => {
                removeSelection();
                nodeMenu.close();
              }}
            >
              <Trash2 className="size-4" aria-hidden="true" />
              Удалить
            </Button>
          </div>
        ) : null}
        <input
          ref={fileInput}
          type="file"
          accept="image/png,image/jpeg,image/gif,application/pdf,audio/mpeg,audio/mp4,audio/ogg,audio/wav,video/mp4,video/webm"
          className="hidden"
          onChange={(event) => {
            const file = event.target.files?.[0];
            if (file) void addMedia(file);
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
  active,
}: {
  icon: typeof StickyNote;
  label: string;
  onClick: () => void;
  disabled?: boolean;
  active?: boolean;
}) {
  return (
    <Button
      type="button"
      variant="ghost"
      size="compact"
      className={cn(
        "size-8 min-h-8 shrink-0 px-0",
        active && "bg-[var(--accent-soft)] text-[var(--accent-ink)]",
      )}
      title={label}
      aria-label={label}
      tooltip={label}
      tooltipSide="right"
      disabled={disabled}
      aria-pressed={active}
      onClick={onClick}
    >
      <Icon className="size-4" aria-hidden="true" />
      <span data-tool-label className="hidden min-w-0 flex-1 truncate text-left">
        {label}
      </span>
    </Button>
  );
}

function ToolRailGroupButton({
  icon: Icon,
  label,
  expanded,
  onClick,
  active,
}: {
  icon: typeof StickyNote;
  label: string;
  expanded: boolean;
  onClick: () => void;
  active?: boolean;
}) {
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      className={cn(
        "relative size-8 shrink-0",
        (active || expanded) && "bg-[var(--accent-soft)] text-[var(--accent-ink)]",
      )}
      title={label}
      aria-label={label}
      tooltip={label}
      tooltipSide="right"
      aria-haspopup="true"
      aria-expanded={expanded}
      onClick={onClick}
    >
      <Icon className="size-4" aria-hidden="true" />
      <ChevronRight
        className={cn(
          "absolute bottom-0.5 right-0.5 size-2.5 transition-transform",
          expanded && "rotate-180",
        )}
        aria-hidden="true"
      />
    </Button>
  );
}

function ToolFlyout({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <fieldset className="absolute left-[calc(100%+8px)] top-0 z-30 flex max-h-[calc(100vh-104px)] w-64 min-w-0 flex-col gap-1 overflow-x-hidden overflow-y-auto rounded-[12px] border border-[var(--border-subtle)] bg-[var(--surface-raised)] p-2 shadow-xl [&_[data-tool-label]]:block [&_button]:w-full [&_button]:min-w-0 [&_button]:justify-start">
      <legend className="px-2 pb-1 pt-0.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--ink-tertiary)]">
        {label}
      </legend>
      {children}
    </fieldset>
  );
}

export function CanvasBoard(props: CanvasBoardProps) {
  return (
    <ReactFlowProvider>
      <CanvasBoardInner {...props} />
    </ReactFlowProvider>
  );
}
