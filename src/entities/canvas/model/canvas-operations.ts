import type { CanvasColor, CanvasDocument, CanvasEdge, CanvasGroupNode, CanvasNode } from "./types";

export interface CanvasBounds {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface CanvasPositionedBounds extends CanvasBounds {
  id: string;
}

export interface DuplicateCanvasResult {
  document: CanvasDocument;
  nodeIds: string[];
  edgeIds: string[];
}

export function snapCanvasNodePosition(
  moving: CanvasPositionedBounds,
  peers: CanvasPositionedBounds[],
  threshold = 12,
): { x: number; y: number } {
  const tolerance = Math.max(0, threshold);
  const movingCenterX = moving.x + moving.width / 2;
  const movingCenterY = moving.y + moving.height / 2;
  let xOffset: number | undefined;
  let yOffset: number | undefined;

  for (const peer of peers) {
    if (peer.id === moving.id) continue;
    const candidateX = peer.x + peer.width / 2 - movingCenterX;
    if (
      Math.abs(candidateX) <= tolerance &&
      (xOffset === undefined || Math.abs(candidateX) < Math.abs(xOffset))
    ) {
      xOffset = candidateX;
    }
    const candidateY = peer.y + peer.height / 2 - movingCenterY;
    if (
      Math.abs(candidateY) <= tolerance &&
      (yOffset === undefined || Math.abs(candidateY) < Math.abs(yOffset))
    ) {
      yOffset = candidateY;
    }
  }

  return {
    x: moving.x + (xOffset ?? 0),
    y: moving.y + (yOffset ?? 0),
  };
}

export function selectionBounds(nodes: CanvasNode[]): CanvasBounds | undefined {
  if (nodes.length === 0) return undefined;
  const left = Math.min(...nodes.map((item) => item.x));
  const top = Math.min(...nodes.map((item) => item.y));
  const right = Math.max(...nodes.map((item) => item.x + item.width));
  const bottom = Math.max(...nodes.map((item) => item.y + item.height));
  return { x: left, y: top, width: right - left, height: bottom - top };
}

export function isNodeInside(node: CanvasNode, bounds: CanvasBounds): boolean {
  return (
    node.x >= bounds.x &&
    node.y >= bounds.y &&
    node.x + node.width <= bounds.x + bounds.width &&
    node.y + node.height <= bounds.y + bounds.height
  );
}

export function createGroupAroundSelection(
  document: CanvasDocument,
  selectedNodeIds: Iterable<string>,
  idFactory: () => string,
  label = "Новая группа",
  color?: CanvasColor,
): { document: CanvasDocument; groupId?: string } {
  const selected = new Set(selectedNodeIds);
  const bounds = selectionBounds(
    document.nodes.filter((item) => selected.has(item.id) && item.type !== "group"),
  );
  if (!bounds) return { document };
  const group: CanvasGroupNode = {
    id: idFactory(),
    type: "group",
    x: Math.round(bounds.x - 32),
    y: Math.round(bounds.y - 56),
    width: Math.round(bounds.width + 64),
    height: Math.round(bounds.height + 88),
    label,
    color,
  };
  return {
    document: { ...document, nodes: [group, ...document.nodes] },
    groupId: group.id,
  };
}

export function moveGroupContents(
  document: CanvasDocument,
  groupId: string,
  delta: { x: number; y: number },
): CanvasDocument {
  const group = document.nodes.find((item) => item.id === groupId && item.type === "group");
  if (!group || (delta.x === 0 && delta.y === 0)) return document;
  const enclosed = new Set(
    document.nodes
      .filter((item) => item.id !== groupId && isNodeInside(item, group))
      .map((item) => item.id),
  );
  return {
    ...document,
    nodes: document.nodes.map((item) =>
      enclosed.has(item.id)
        ? { ...item, x: Math.round(item.x + delta.x), y: Math.round(item.y + delta.y) }
        : item,
    ),
  };
}

export function duplicateCanvasSelection(
  document: CanvasDocument,
  selectedNodeIds: Iterable<string>,
  selectedEdgeIds: Iterable<string>,
  idFactory: () => string,
  offset = 40,
): DuplicateCanvasResult {
  const selectedNodes = expandedSelection(document, new Set(selectedNodeIds));
  const selectedEdges = new Set(selectedEdgeIds);
  const idMap = new Map<string, string>();
  for (const id of selectedNodes) idMap.set(id, idFactory());

  const copiedNodes = document.nodes
    .filter((item) => selectedNodes.has(item.id))
    .map((item) => ({
      ...item,
      id: idMap.get(item.id) as string,
      x: Math.round(item.x + offset),
      y: Math.round(item.y + offset),
    })) as CanvasNode[];
  const copiedEdges = document.edges
    .filter(
      (item) =>
        (selectedEdges.has(item.id) ||
          (selectedNodes.has(item.fromNode) && selectedNodes.has(item.toNode))) &&
        idMap.has(item.fromNode) &&
        idMap.has(item.toNode),
    )
    .map((item) => ({
      ...item,
      id: idFactory(),
      fromNode: idMap.get(item.fromNode) as string,
      toNode: idMap.get(item.toNode) as string,
    })) as CanvasEdge[];

  return {
    document: {
      ...document,
      nodes: [...document.nodes, ...copiedNodes],
      edges: [...document.edges, ...copiedEdges],
    },
    nodeIds: copiedNodes.map((item) => item.id),
    edgeIds: copiedEdges.map((item) => item.id),
  };
}

export function removeCanvasSelection(
  document: CanvasDocument,
  selectedNodeIds: Iterable<string>,
  selectedEdgeIds: Iterable<string>,
): CanvasDocument {
  const nodes = new Set(selectedNodeIds);
  const edges = new Set(selectedEdgeIds);
  return {
    ...document,
    nodes: document.nodes.filter((item) => !nodes.has(item.id)),
    edges: document.edges.filter(
      (item) => !edges.has(item.id) && !nodes.has(item.fromNode) && !nodes.has(item.toNode),
    ),
  };
}

function expandedSelection(document: CanvasDocument, selected: Set<string>): Set<string> {
  const result = new Set(selected);
  for (const group of document.nodes) {
    if (group.type !== "group" || !selected.has(group.id)) continue;
    for (const item of document.nodes) {
      if (item.id !== group.id && isNodeInside(item, group)) result.add(item.id);
    }
  }
  return result;
}
