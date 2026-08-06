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

export type CanvasAlignment = "left" | "center-x" | "right" | "top" | "center-y" | "bottom";

export interface CanvasSnapGuide {
  position: number;
  start: number;
  end: number;
}

export interface CanvasSnapResult {
  position: { x: number; y: number };
  vertical?: CanvasSnapGuide;
  horizontal?: CanvasSnapGuide;
}

export function snapCanvasNodeWithGuides(
  moving: CanvasPositionedBounds,
  peers: CanvasPositionedBounds[],
  threshold = 12,
): CanvasSnapResult {
  const tolerance = Math.max(0, threshold);
  const movingX = [moving.x, moving.x + moving.width / 2, moving.x + moving.width];
  const movingY = [moving.y, moving.y + moving.height / 2, moving.y + moving.height];
  let xMatch:
    | { offset: number; peer: CanvasPositionedBounds; position: number; priority: number }
    | undefined;
  let yMatch:
    | { offset: number; peer: CanvasPositionedBounds; position: number; priority: number }
    | undefined;

  for (const peer of peers) {
    if (peer.id === moving.id) continue;
    const peerX = [peer.x, peer.x + peer.width / 2, peer.x + peer.width];
    const peerY = [peer.y, peer.y + peer.height / 2, peer.y + peer.height];
    for (const [ownIndex, own] of movingX.entries()) {
      for (const [targetIndex, target] of peerX.entries()) {
        const offset = target - own;
        const priority = ownIndex === 1 && targetIndex === 1 ? 0 : 1;
        if (
          Math.abs(offset) <= tolerance &&
          (xMatch === undefined ||
            priority < xMatch.priority ||
            (priority === xMatch.priority && Math.abs(offset) < Math.abs(xMatch.offset)))
        ) {
          xMatch = { offset, peer, position: target, priority };
        }
      }
    }
    for (const [ownIndex, own] of movingY.entries()) {
      for (const [targetIndex, target] of peerY.entries()) {
        const offset = target - own;
        const priority = ownIndex === 1 && targetIndex === 1 ? 0 : 1;
        if (
          Math.abs(offset) <= tolerance &&
          (yMatch === undefined ||
            priority < yMatch.priority ||
            (priority === yMatch.priority && Math.abs(offset) < Math.abs(yMatch.offset)))
        ) {
          yMatch = { offset, peer, position: target, priority };
        }
      }
    }
  }

  const x = moving.x + (xMatch?.offset ?? 0);
  const y = moving.y + (yMatch?.offset ?? 0);
  return {
    position: { x, y },
    ...(xMatch
      ? {
          vertical: {
            position: xMatch.position,
            start: Math.min(y, xMatch.peer.y) - 24,
            end: Math.max(y + moving.height, xMatch.peer.y + xMatch.peer.height) + 24,
          },
        }
      : {}),
    ...(yMatch
      ? {
          horizontal: {
            position: yMatch.position,
            start: Math.min(x, yMatch.peer.x) - 24,
            end: Math.max(x + moving.width, yMatch.peer.x + yMatch.peer.width) + 24,
          },
        }
      : {}),
  };
}

export function snapCanvasNodePosition(
  moving: CanvasPositionedBounds,
  peers: CanvasPositionedBounds[],
  threshold = 12,
): { x: number; y: number } {
  return snapCanvasNodeWithGuides(moving, peers, threshold).position;
}

export function rotateCanvasSelection(
  document: CanvasDocument,
  selectedNodeIds: Iterable<string>,
  degrees: number,
): CanvasDocument {
  if (!Number.isFinite(degrees)) return document;
  const selected = new Set(selectedNodeIds);
  return {
    ...document,
    nodes: document.nodes.map((node) => {
      const extension = node["x-writers-studio"];
      const element = extension?.element;
      if (!selected.has(node.id) || node.type !== "group" || element?.kind !== "shape") {
        return node;
      }
      const rotation = ((((element.rotation + degrees + 180) % 360) + 360) % 360) - 180;
      return {
        ...node,
        "x-writers-studio": { ...extension, element: { ...element, rotation } },
      };
    }),
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
    document.nodes.filter((item) => selected.has(item.id) && !isSpatialGroup(item)),
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

export function createFrameAroundSelection(
  document: CanvasDocument,
  selectedNodeIds: Iterable<string>,
  idFactory: () => string,
  label = "Новый фрейм",
): { document: CanvasDocument; frameId?: string } {
  const selected = new Set(selectedNodeIds);
  const bounds = selectionBounds(document.nodes.filter((item) => selected.has(item.id)));
  if (!bounds) return { document };
  const order =
    document.nodes.filter(
      (node) => node.type === "group" && node["x-writers-studio"]?.element?.kind === "frame",
    ).length + 1;
  const frame: CanvasGroupNode = {
    id: idFactory(),
    type: "group",
    x: Math.round(bounds.x - 48),
    y: Math.round(bounds.y - 64),
    width: Math.round(bounds.width + 96),
    height: Math.round(bounds.height + 112),
    label,
    "x-writers-studio": { element: { kind: "frame", order, label } },
  };
  return { document: { ...document, nodes: [frame, ...document.nodes] }, frameId: frame.id };
}

export function moveGroupContents(
  document: CanvasDocument,
  groupId: string,
  delta: { x: number; y: number },
): CanvasDocument {
  const group = document.nodes.find((item) => item.id === groupId && isSpatialGroup(item));
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

export function alignCanvasSelection(
  document: CanvasDocument,
  selectedNodeIds: Iterable<string>,
  alignment: CanvasAlignment,
): CanvasDocument {
  const selected = new Set(selectedNodeIds);
  const nodes = document.nodes.filter((node) => selected.has(node.id));
  const bounds = selectionBounds(nodes);
  if (!bounds || nodes.length < 2) return document;
  return {
    ...document,
    nodes: document.nodes.map((node) => {
      if (!selected.has(node.id)) return node;
      if (alignment === "left") return { ...node, x: bounds.x };
      if (alignment === "right") return { ...node, x: bounds.x + bounds.width - node.width };
      if (alignment === "center-x") {
        return { ...node, x: Math.round(bounds.x + (bounds.width - node.width) / 2) };
      }
      if (alignment === "top") return { ...node, y: bounds.y };
      if (alignment === "bottom") return { ...node, y: bounds.y + bounds.height - node.height };
      return { ...node, y: Math.round(bounds.y + (bounds.height - node.height) / 2) };
    }) as CanvasNode[],
  };
}

export function distributeCanvasSelection(
  document: CanvasDocument,
  selectedNodeIds: Iterable<string>,
  axis: "horizontal" | "vertical",
): CanvasDocument {
  const selected = new Set(selectedNodeIds);
  const nodes = document.nodes
    .filter((node) => selected.has(node.id))
    .sort((left, right) => (axis === "horizontal" ? left.x - right.x : left.y - right.y));
  if (nodes.length < 3) return document;
  const first = nodes[0];
  const last = nodes.at(-1);
  if (!first || !last) return document;
  const occupied = nodes.reduce(
    (total, node) => total + (axis === "horizontal" ? node.width : node.height),
    0,
  );
  const span =
    axis === "horizontal" ? last.x + last.width - first.x : last.y + last.height - first.y;
  const gap = (span - occupied) / (nodes.length - 1);
  const positions = new Map<string, number>();
  let cursor = axis === "horizontal" ? first.x : first.y;
  for (const node of nodes) {
    positions.set(node.id, Math.round(cursor));
    cursor += (axis === "horizontal" ? node.width : node.height) + gap;
  }
  return {
    ...document,
    nodes: document.nodes.map((node) => {
      const position = positions.get(node.id);
      if (position === undefined) return node;
      return axis === "horizontal" ? { ...node, x: position } : { ...node, y: position };
    }) as CanvasNode[],
  };
}

export function tidyCanvasSelection(
  document: CanvasDocument,
  selectedNodeIds: Iterable<string>,
  gap = 40,
): CanvasDocument {
  const selected = new Set(selectedNodeIds);
  const nodes = document.nodes.filter((node) => selected.has(node.id));
  const bounds = selectionBounds(nodes);
  if (!bounds || nodes.length < 2) return document;
  const columns = Math.ceil(Math.sqrt(nodes.length));
  const columnWidth = Math.max(...nodes.map((node) => node.width));
  const rowHeight = Math.max(...nodes.map((node) => node.height));
  const positions = new Map(
    nodes.map((node, index) => [
      node.id,
      {
        x: Math.round(bounds.x + (index % columns) * (columnWidth + gap)),
        y: Math.round(bounds.y + Math.floor(index / columns) * (rowHeight + gap)),
      },
    ]),
  );
  return {
    ...document,
    nodes: document.nodes.map((node) => {
      const position = positions.get(node.id);
      return position ? { ...node, ...position } : node;
    }) as CanvasNode[],
  };
}

function expandedSelection(document: CanvasDocument, selected: Set<string>): Set<string> {
  const result = new Set(selected);
  for (const group of document.nodes) {
    if (!isSpatialGroup(group) || !selected.has(group.id)) continue;
    for (const item of document.nodes) {
      if (item.id !== group.id && isNodeInside(item, group)) result.add(item.id);
    }
  }
  return result;
}

export function isSpatialGroup(node: CanvasNode): node is CanvasGroupNode {
  return node.type === "group" && node["x-writers-studio"]?.element === undefined;
}
