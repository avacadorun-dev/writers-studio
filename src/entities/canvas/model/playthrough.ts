import type { CanvasDocument, CanvasEdge, CanvasNode } from "./types";

export type CanvasPlaythroughProfile = "dialogue" | "quest" | "state";

export interface CanvasPlaythroughChoice {
  edgeId: string;
  toNodeId: string;
  label: string;
  condition?: unknown;
}

export interface CanvasPlaythroughState {
  profile: CanvasPlaythroughProfile;
  path: string[];
  edgePath: string[];
}

export function canvasPlaythroughStarts(
  document: CanvasDocument,
  profile: CanvasPlaythroughProfile,
): CanvasNode[] {
  const nodes = document.nodes.filter((node) => node["x-writers-studio"]?.card?.typeId === profile);
  const ids = new Set(nodes.map((node) => node.id));
  if (profile === "state") {
    const explicit = nodes.filter(
      (node) => node["x-writers-studio"]?.card?.fields.initial === true,
    );
    if (explicit.length) return explicit;
  }
  const incoming = new Set(
    playthroughEdges(document, profile)
      .filter((edge) => ids.has(edge.fromNode) && ids.has(edge.toNode))
      .map((edge) => edge.toNode),
  );
  const roots = nodes.filter((node) => !incoming.has(node.id));
  return roots.length ? roots : nodes.slice(0, 1);
}

export function startCanvasPlaythrough(
  profile: CanvasPlaythroughProfile,
  nodeId: string,
): CanvasPlaythroughState {
  return { profile, path: [nodeId], edgePath: [] };
}

export function canvasPlaythroughChoices(
  document: CanvasDocument,
  state: CanvasPlaythroughState,
): CanvasPlaythroughChoice[] {
  const current = state.path.at(-1);
  if (!current) return [];
  return playthroughEdges(document, state.profile)
    .filter((edge) => edge.fromNode === current)
    .map((edge) => ({
      edgeId: edge.id,
      toNodeId: edge.toNode,
      label: edge.label?.trim() || nodeTitle(document, edge.toNode),
      condition: edge["x-writers-studio"]?.relation?.fields?.condition,
    }));
}

export function advanceCanvasPlaythrough(
  state: CanvasPlaythroughState,
  choice: CanvasPlaythroughChoice,
): CanvasPlaythroughState {
  return {
    ...state,
    path: [...state.path, choice.toNodeId],
    edgePath: [...state.edgePath, choice.edgeId],
  };
}

function playthroughEdges(
  document: CanvasDocument,
  profile: CanvasPlaythroughProfile,
): CanvasEdge[] {
  const relation = profile === "quest" ? "depends_on" : "transitions_to";
  return document.edges.filter((edge) => edge["x-writers-studio"]?.relation?.typeId === relation);
}

function nodeTitle(document: CanvasDocument, nodeId: string): string {
  const node = document.nodes.find((item) => item.id === nodeId);
  if (!node) return nodeId;
  if (node.type === "text") return node.text.split("\n", 1)[0]?.replace(/^#+\s*/u, "") || nodeId;
  if (node.type === "group") return node.label || nodeId;
  if (node.type === "link") return node["x-writers-studio"]?.preview?.title || node.url;
  return node.file;
}
