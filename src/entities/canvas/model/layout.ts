import type { CanvasDocument, CanvasNode } from "./types";

export type CanvasLayoutMode =
  | "hierarchy-horizontal"
  | "hierarchy-vertical"
  | "mind-map"
  | "organic";

export interface CanvasLayoutNode {
  id: string;
  width: number;
  height: number;
  x?: number;
  y?: number;
}

export interface CanvasLayoutGraph {
  id: string;
  layoutOptions: Record<string, string>;
  children: CanvasLayoutNode[];
  edges: Array<{ id: string; sources: string[]; targets: string[] }>;
}

export interface CanvasLayoutResult {
  children?: CanvasLayoutNode[];
}

export function buildCanvasLayoutGraph(
  document: CanvasDocument,
  nodeIds: Iterable<string>,
  mode: CanvasLayoutMode,
): CanvasLayoutGraph {
  const requested = new Set(nodeIds);
  const included = new Set(
    document.nodes
      .filter((node) => (requested.size === 0 || requested.has(node.id)) && isLayoutNode(node))
      .map((node) => node.id),
  );
  return {
    id: "canvas-layout",
    layoutOptions: layoutOptions(mode),
    children: document.nodes
      .filter((node) => included.has(node.id))
      .map((node) => ({ id: node.id, width: node.width, height: node.height })),
    edges: document.edges
      .filter((edge) => included.has(edge.fromNode) && included.has(edge.toNode))
      .map((edge) => ({ id: edge.id, sources: [edge.fromNode], targets: [edge.toNode] })),
  };
}

export function applyCanvasLayout(
  document: CanvasDocument,
  result: CanvasLayoutResult,
): CanvasDocument {
  const positions = new Map(
    (result.children ?? [])
      .filter((node) => Number.isFinite(node.x) && Number.isFinite(node.y))
      .map((node) => [node.id, { x: Math.round(node.x ?? 0), y: Math.round(node.y ?? 0) }]),
  );
  if (positions.size === 0) return document;
  return {
    ...document,
    nodes: document.nodes.map((node) => {
      if (node["x-writers-studio"]?.locked) return node;
      const position = positions.get(node.id);
      return position ? { ...node, ...position } : node;
    }) as CanvasNode[],
  };
}

export function requestCanvasLayout(
  document: CanvasDocument,
  nodeIds: Iterable<string>,
  mode: CanvasLayoutMode,
): Promise<CanvasDocument> {
  const graph = buildCanvasLayoutGraph(document, nodeIds, mode);
  if (graph.children.length < 2) return Promise.resolve(document);
  return new Promise((resolve, reject) => {
    const worker = new Worker(new URL("./layout.worker.ts", import.meta.url), { type: "module" });
    const timeout = globalThis.setTimeout(() => {
      worker.terminate();
      reject(new Error("Раскладка не успела завершиться"));
    }, 15_000);
    worker.addEventListener(
      "message",
      (event: MessageEvent<{ result?: CanvasLayoutResult; error?: string }>) => {
        globalThis.clearTimeout(timeout);
        worker.terminate();
        if (event.data.error) reject(new Error(event.data.error));
        else resolve(applyCanvasLayout(document, event.data.result ?? {}));
      },
      { once: true },
    );
    worker.addEventListener(
      "error",
      () => {
        globalThis.clearTimeout(timeout);
        worker.terminate();
        reject(new Error("Не удалось запустить алгоритм раскладки"));
      },
      { once: true },
    );
    worker.postMessage(graph);
  });
}

function isLayoutNode(node: CanvasNode): boolean {
  if (node.type !== "group") return true;
  const kind = node["x-writers-studio"]?.element?.kind;
  return kind === "shape";
}

function layoutOptions(mode: CanvasLayoutMode): Record<string, string> {
  if (mode === "mind-map") {
    return { "elk.algorithm": "radial", "elk.spacing.nodeNode": "70" };
  }
  if (mode === "organic") {
    return { "elk.algorithm": "force", "elk.spacing.nodeNode": "80" };
  }
  return {
    "elk.algorithm": "layered",
    "elk.direction": mode === "hierarchy-horizontal" ? "RIGHT" : "DOWN",
    "elk.spacing.nodeNode": "60",
    "elk.layered.spacing.nodeNodeBetweenLayers": "100",
  };
}
