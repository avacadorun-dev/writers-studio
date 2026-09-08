import { type Edge, MarkerType, type Node, Position } from "@xyflow/react";
import type { ProjectMedia } from "~entities/project";
import type {
  CanvasDocument,
  CanvasDocumentPreview,
  CanvasEdge,
  CanvasFileNode,
  CanvasNode,
  CanvasSide,
} from "./types";

export type CanvasFlowNodeType =
  | "canvas-text"
  | "canvas-document"
  | "canvas-image"
  | "canvas-external-file"
  | "canvas-pdf"
  | "canvas-media"
  | "canvas-nested"
  | "canvas-link"
  | "canvas-group"
  | "canvas-stroke"
  | "canvas-shape"
  | "canvas-frame";

export interface CanvasFlowNodeData extends Record<string, unknown> {
  canvasNode: CanvasNode;
  document?: CanvasDocumentPreview;
  media?: ProjectMedia;
}

export interface CanvasFlowEdgeData extends Record<string, unknown> {
  canvasEdge: CanvasEdge;
}

export type CanvasFlowNode = Node<CanvasFlowNodeData, CanvasFlowNodeType>;
export type CanvasFlowEdge = Edge<CanvasFlowEdgeData, "canvas-edge">;

export interface CanvasFlowState {
  nodes: CanvasFlowNode[];
  edges: CanvasFlowEdge[];
}

const sidePosition: Record<CanvasSide, Position> = {
  top: Position.Top,
  right: Position.Right,
  bottom: Position.Bottom,
  left: Position.Left,
};

export function canvasToFlow(
  content: CanvasDocument,
  documents: CanvasDocumentPreview[] = [],
  media: ProjectMedia[] = [],
): CanvasFlowState {
  const documentById = new Map(documents.map((item) => [item.id, item]));
  const mediaById = new Map(media.map((item) => [item.id, item]));
  return {
    nodes: content.nodes.map((item, index) => {
      const resource = item.type === "file" ? item["x-writers-studio"]?.resource : undefined;
      return {
        id: item.id,
        type: flowNodeType(item, mediaById),
        position: { x: item.x, y: item.y },
        initialWidth: item.width,
        initialHeight: item.height,
        style: { width: item.width, height: item.height },
        zIndex: isBackgroundNode(item) ? 0 : index + 1,
        draggable: item["x-writers-studio"]?.locked !== true,
        connectable: !(
          item.type === "group" && item["x-writers-studio"]?.element?.kind === "stroke"
        ),
        data: {
          canvasNode: item,
          document: resource?.kind === "document" ? documentById.get(resource.id) : undefined,
          media: resource?.kind === "media" ? mediaById.get(resource.id) : undefined,
        },
        ariaLabel: canvasNodeAriaLabel(item, documentById, mediaById),
      } satisfies CanvasFlowNode;
    }),
    edges: content.edges.map((item) => ({
      id: item.id,
      type: "canvas-edge",
      source: item.fromNode,
      target: item.toNode,
      sourceHandle: item.fromSide,
      targetHandle: item.toSide,
      markerStart: item.fromEnd === "arrow" ? { type: MarkerType.ArrowClosed } : undefined,
      markerEnd: item.toEnd !== "none" ? { type: MarkerType.ArrowClosed } : undefined,
      zIndex: 2,
      data: { canvasEdge: item },
      ariaLabel: item.label ? `Связь: ${item.label}` : "Связь между карточками",
    })),
  };
}

export function flowToCanvas(
  nodes: CanvasFlowNode[],
  edges: CanvasFlowEdge[],
  base: CanvasDocument,
): CanvasDocument {
  const orderedNodes = [...nodes].sort((left, right) => {
    const groupOrder =
      Number(!isBackgroundNode(left.data.canvasNode)) -
      Number(!isBackgroundNode(right.data.canvasNode));
    return groupOrder || (left.zIndex ?? 0) - (right.zIndex ?? 0);
  });
  return {
    ...base,
    nodes: orderedNodes.map((item) => resizedCanvasNode(item)) as CanvasNode[],
    edges: edges.map((item) => ({
      ...(item.data?.canvasEdge ?? {
        id: item.id,
        fromNode: item.source,
        toNode: item.target,
      }),
      id: item.id,
      fromNode: item.source,
      fromSide: normalizeSide(item.sourceHandle),
      toNode: item.target,
      toSide: normalizeSide(item.targetHandle),
    })),
  };
}

function resizedCanvasNode(item: CanvasFlowNode): CanvasNode {
  const source = item.data.canvasNode;
  const width = Math.round(item.measured?.width ?? item.width ?? source.width);
  const height = Math.round(item.measured?.height ?? item.height ?? source.height);
  const extension = source["x-writers-studio"];
  const element = extension?.element;
  if (source.type === "group" && element?.kind === "stroke") {
    const scaleX = width / source.width;
    const scaleY = height / source.height;
    return {
      ...source,
      id: item.id,
      x: Math.round(item.position.x),
      y: Math.round(item.position.y),
      width,
      height,
      "x-writers-studio": {
        ...extension,
        element: {
          ...element,
          points: element.points.map((point) =>
            point[2] === undefined
              ? [
                  Math.round(point[0] * scaleX * 100) / 100,
                  Math.round(point[1] * scaleY * 100) / 100,
                ]
              : [
                  Math.round(point[0] * scaleX * 100) / 100,
                  Math.round(point[1] * scaleY * 100) / 100,
                  point[2],
                ],
          ),
        },
      },
    };
  }
  return {
    ...source,
    id: item.id,
    x: Math.round(item.position.x),
    y: Math.round(item.position.y),
    width,
    height,
  } as CanvasNode;
}

export function sideToPosition(side: CanvasSide): Position {
  return sidePosition[side];
}

function flowNodeType(item: CanvasNode, media: Map<string, ProjectMedia>): CanvasFlowNodeType {
  if (item.type === "text") return "canvas-text";
  if (item.type === "link") return "canvas-link";
  if (item.type === "group") {
    const element = item["x-writers-studio"]?.element;
    if (element?.kind === "stroke") return "canvas-stroke";
    if (element?.kind === "shape") return "canvas-shape";
    if (element?.kind === "frame") return "canvas-frame";
    return "canvas-group";
  }
  const resource = item["x-writers-studio"]?.resource;
  if (!resource) return "canvas-external-file";
  if (resource.kind === "document") return "canvas-document";
  if (resource.kind === "canvas") return "canvas-nested";
  const mediaType =
    (resource.kind === "media" ? media.get(resource.id)?.mediaType : undefined) ??
    (item["x-writers-studio"]?.mediaType as string | undefined) ??
    "";
  if (mediaType === "application/pdf") return "canvas-pdf";
  if (mediaType.startsWith("audio/") || mediaType.startsWith("video/")) return "canvas-media";
  return "canvas-image";
}

function isBackgroundNode(item: CanvasNode): boolean {
  if (item.type !== "group") return false;
  const kind = item["x-writers-studio"]?.element?.kind;
  return kind === undefined || kind === "frame";
}

function canvasNodeAriaLabel(
  item: CanvasNode,
  documents: Map<string, CanvasDocumentPreview>,
  media: Map<string, ProjectMedia>,
): string {
  if (item.type === "text") return item.text.trim().slice(0, 120) || "Пустая заметка";
  if (item.type === "link") {
    return `Ссылка ${item["x-writers-studio"]?.preview?.title ?? item.url}`;
  }
  if (item.type === "group") {
    const element = item["x-writers-studio"]?.element;
    if (element?.kind === "stroke") return `${element.brush === "pen" ? "Штрих" : "Маркер"}`;
    if (element?.kind === "shape") return `Фигура ${element.label || element.shape}`;
    if (element?.kind === "frame") return `Фрейм ${element.label || item.label || element.order}`;
    return `Группа ${item.label || "без названия"}`;
  }
  const resource = item["x-writers-studio"]?.resource;
  if (!resource) return `Файл ${item.file}`;
  if (resource.kind === "document") {
    return `Документ ${documents.get(resource.id)?.title ?? "недоступен"}`;
  }
  if (resource.kind === "canvas") return "Вложенная доска";
  const asset = media.get(resource.id);
  return `${asset?.mediaType === "application/pdf" ? "PDF" : "Изображение"} ${asset?.originalName ?? "недоступно"}`;
}

function normalizeSide(value: string | null | undefined): CanvasSide | undefined {
  return value === "top" || value === "right" || value === "bottom" || value === "left"
    ? value
    : undefined;
}

export function canvasFileResource(item: CanvasFileNode) {
  return item["x-writers-studio"]?.resource;
}
