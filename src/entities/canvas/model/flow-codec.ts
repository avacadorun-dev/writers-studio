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
  | "canvas-group";

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
      const resource = item.type === "file" ? item["x-writers-studio"].resource : undefined;
      return {
        id: item.id,
        type: flowNodeType(item),
        position: { x: item.x, y: item.y },
        initialWidth: item.width,
        initialHeight: item.height,
        style: { width: item.width, height: item.height },
        zIndex: item.type === "group" ? 0 : index + 1,
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
      Number(left.data.canvasNode.type !== "group") -
      Number(right.data.canvasNode.type !== "group");
    return groupOrder || (left.zIndex ?? 0) - (right.zIndex ?? 0);
  });
  return {
    ...base,
    nodes: orderedNodes.map((item) => ({
      ...item.data.canvasNode,
      id: item.id,
      x: Math.round(item.position.x),
      y: Math.round(item.position.y),
      width: Math.round(item.measured?.width ?? item.width ?? item.data.canvasNode.width),
      height: Math.round(item.measured?.height ?? item.height ?? item.data.canvasNode.height),
    })) as CanvasNode[],
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

export function sideToPosition(side: CanvasSide): Position {
  return sidePosition[side];
}

function flowNodeType(item: CanvasNode): CanvasFlowNodeType {
  if (item.type === "text") return "canvas-text";
  if (item.type === "group") return "canvas-group";
  return item["x-writers-studio"].resource.kind === "media" ? "canvas-image" : "canvas-document";
}

function canvasNodeAriaLabel(
  item: CanvasNode,
  documents: Map<string, CanvasDocumentPreview>,
  media: Map<string, ProjectMedia>,
): string {
  if (item.type === "text") return item.text.trim().slice(0, 120) || "Пустая заметка";
  if (item.type === "group") return `Группа ${item.label || "без названия"}`;
  const resource = item["x-writers-studio"].resource;
  if (resource.kind === "document") {
    return `Документ ${documents.get(resource.id)?.title ?? "недоступен"}`;
  }
  return `Изображение ${media.get(resource.id)?.originalName ?? "недоступно"}`;
}

function normalizeSide(value: string | null | undefined): CanvasSide | undefined {
  return value === "top" || value === "right" || value === "bottom" || value === "left"
    ? value
    : undefined;
}

export function canvasFileResource(item: CanvasFileNode) {
  return item["x-writers-studio"].resource;
}
