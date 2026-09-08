import type { CanvasDocument, CanvasNode } from "./types";

export type CanvasViewMode =
  | "free"
  | "mind-map"
  | "timeline"
  | "swimlanes"
  | "dependency"
  | "filtered";

export function projectCanvasView(
  document: CanvasDocument,
  mode: CanvasViewMode,
  filterTypeId?: string,
): CanvasDocument {
  if (mode === "free") return document;
  if (mode === "filtered") {
    if (!filterTypeId) return document;
    const visible = new Set(
      document.nodes
        .filter((node) => node["x-writers-studio"]?.card?.typeId === filterTypeId)
        .map((node) => node.id),
    );
    return {
      ...document,
      nodes: document.nodes.filter((node) => visible.has(node.id)),
      edges: document.edges.filter(
        (edge) => visible.has(edge.fromNode) && visible.has(edge.toNode),
      ),
    };
  }

  const positions =
    mode === "timeline"
      ? timelinePositions(document.nodes)
      : mode === "swimlanes"
        ? swimlanePositions(document.nodes)
        : graphPositions(document, mode === "mind-map");
  return {
    ...document,
    nodes: document.nodes.map((node) => {
      const position = positions.get(node.id);
      return position ? ({ ...node, ...position } as CanvasNode) : node;
    }),
  };
}

function timelinePositions(nodes: CanvasNode[]): Map<string, { x: number; y: number }> {
  const sorted = [...nodes].sort((left, right) => {
    const leftDate = typedDate(left);
    const rightDate = typedDate(right);
    if (leftDate !== rightDate) return leftDate - rightDate;
    return left.id.localeCompare(right.id);
  });
  return new Map(sorted.map((node, index) => [node.id, { x: index * 340, y: 0 }]));
}

function swimlanePositions(nodes: CanvasNode[]): Map<string, { x: number; y: number }> {
  const lanes = new Map<string, CanvasNode[]>();
  for (const node of nodes) {
    const lane = node["x-writers-studio"]?.card?.typeId ?? "untyped";
    const items = lanes.get(lane) ?? [];
    items.push(node);
    lanes.set(lane, items);
  }
  const result = new Map<string, { x: number; y: number }>();
  [...lanes.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .forEach(([, items], laneIndex) => {
      items
        .sort(
          (left, right) => left.x - right.x || left.y - right.y || left.id.localeCompare(right.id),
        )
        .forEach((node, index) => {
          result.set(node.id, { x: index * 340, y: laneIndex * 280 });
        });
    });
  return result;
}

function graphPositions(
  document: CanvasDocument,
  mindMap: boolean,
): Map<string, { x: number; y: number }> {
  const ids = new Set(document.nodes.map((node) => node.id));
  const incoming = new Map([...ids].map((id) => [id, 0]));
  const outgoing = new Map([...ids].map((id) => [id, [] as string[]]));
  for (const edge of document.edges) {
    if (!ids.has(edge.fromNode) || !ids.has(edge.toNode)) continue;
    outgoing.get(edge.fromNode)?.push(edge.toNode);
    incoming.set(edge.toNode, (incoming.get(edge.toNode) ?? 0) + 1);
  }
  const roots = [...ids].filter((id) => incoming.get(id) === 0);
  const queue = roots.map((id) => ({ id, depth: 0 }));
  const depths = new Map<string, number>();
  while (queue.length) {
    const current = queue.shift();
    if (!current) break;
    if ((depths.get(current.id) ?? -1) >= current.depth) continue;
    depths.set(current.id, current.depth);
    for (const next of outgoing.get(current.id) ?? []) {
      queue.push({ id: next, depth: current.depth + 1 });
    }
  }
  for (const id of ids) if (!depths.has(id)) depths.set(id, 0);
  const layers = new Map<number, string[]>();
  for (const [id, depth] of depths) {
    const layer = layers.get(depth) ?? [];
    layer.push(id);
    layers.set(depth, layer);
  }
  const result = new Map<string, { x: number; y: number }>();
  for (const [depth, layer] of layers) {
    layer.sort();
    layer.forEach((id, index) => {
      if (mindMap && depth > 0) {
        const side = index % 2 === 0 ? 1 : -1;
        result.set(id, { x: side * depth * 380, y: Math.floor(index / 2) * 260 });
      } else {
        result.set(id, { x: depth * 380, y: index * 260 });
      }
    });
  }
  return result;
}

function typedDate(node: CanvasNode): number {
  const value = node["x-writers-studio"]?.card?.fields.date;
  if (typeof value !== "string") return Number.MAX_SAFE_INTEGER;
  const timestamp = Date.parse(value);
  return Number.isNaN(timestamp) ? Number.MAX_SAFE_INTEGER : timestamp;
}
