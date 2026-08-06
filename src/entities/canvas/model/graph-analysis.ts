import type { CanvasDocument, CanvasEdge, CanvasNode } from "./types";

export type CanvasGraphProfile = "generic" | "quest" | "dialogue" | "state" | "economy" | "all";

export interface CanvasGraphDiagnostic {
  code: string;
  severity: "info" | "warning";
  message: string;
  nodeIds: string[];
  edgeIds: string[];
}

export interface CanvasGraphAnalysis {
  components: string[][];
  orphans: string[];
  roots: string[];
  leaves: string[];
  cycles: string[][];
  criticalPath: string[];
  diagnostics: CanvasGraphDiagnostic[];
}

export function analyzeCanvasGraph(
  document: CanvasDocument,
  profile: CanvasGraphProfile = "all",
): CanvasGraphAnalysis {
  const nodeIds = new Set(document.nodes.filter(isGraphNode).map((node) => node.id));
  const edges = document.edges.filter(
    (edge) => nodeIds.has(edge.fromNode) && nodeIds.has(edge.toNode),
  );
  const outgoing = adjacency(nodeIds, edges, "outgoing");
  const incoming = adjacency(nodeIds, edges, "incoming");
  const components = connectedComponents(nodeIds, edges);
  const orphans = [...nodeIds].filter(
    (id) => (outgoing.get(id)?.length ?? 0) === 0 && (incoming.get(id)?.length ?? 0) === 0,
  );
  const roots = [...nodeIds].filter((id) => (incoming.get(id)?.length ?? 0) === 0);
  const leaves = [...nodeIds].filter((id) => (outgoing.get(id)?.length ?? 0) === 0);
  const cycles = findCycles(nodeIds, outgoing);
  const diagnostics: CanvasGraphDiagnostic[] = [];
  if (components.length > 1) {
    diagnostics.push(
      issue("disconnected", "Доска содержит несвязанные компоненты", components.flat()),
    );
  }
  for (const id of orphans) diagnostics.push(issue("orphan", "Элемент ни с чем не связан", [id]));
  for (const cycle of cycles)
    diagnostics.push(issue("cycle", "Обнаружен направленный цикл", cycle));

  if (profile === "quest" || profile === "all") validateQuests(document, diagnostics);
  if (profile === "dialogue" || profile === "all") validateDialogue(document, diagnostics);
  if (profile === "state" || profile === "all") validateStates(document, diagnostics);
  if (profile === "economy" || profile === "all") validateEconomy(document, diagnostics);

  return {
    components,
    orphans,
    roots,
    leaves,
    cycles,
    criticalPath: cycles.length === 0 ? longestPath(nodeIds, outgoing, incoming) : [],
    diagnostics,
  };
}

export function shortestCanvasPath(
  document: CanvasDocument,
  fromNode: string,
  toNode: string,
): string[] {
  if (fromNode === toNode && document.nodes.some((node) => node.id === fromNode)) return [fromNode];
  const nodeIds = new Set(document.nodes.filter(isGraphNode).map((node) => node.id));
  if (!nodeIds.has(fromNode) || !nodeIds.has(toNode)) return [];
  const outgoing = adjacency(nodeIds, document.edges, "outgoing");
  const queue: string[] = [fromNode];
  const previous = new Map<string, string>();
  const visited = new Set(queue);
  while (queue.length) {
    const current = queue.shift();
    if (!current) break;
    for (const next of outgoing.get(current) ?? []) {
      if (visited.has(next)) continue;
      previous.set(next, current);
      if (next === toNode) return reconstructPath(previous, fromNode, toNode);
      visited.add(next);
      queue.push(next);
    }
  }
  return [];
}

function validateQuests(document: CanvasDocument, diagnostics: CanvasGraphDiagnostic[]) {
  const quests = typedNodes(document, "quest");
  const questIds = new Set(quests.map((node) => node.id));
  const dependencyEdges = document.edges.filter(
    (edge) =>
      relationType(edge) === "depends_on" &&
      questIds.has(edge.fromNode) &&
      questIds.has(edge.toNode),
  );
  const cycles = findCycles(questIds, adjacency(questIds, dependencyEdges, "outgoing"));
  for (const cycle of cycles) {
    diagnostics.push(issue("quest-cycle", "Циклическая зависимость квестов", cycle));
  }
  for (const quest of quests) {
    const prerequisites = quest["x-writers-studio"]?.card?.fields.prerequisites;
    if (!Array.isArray(prerequisites)) continue;
    const missing = prerequisites.filter(
      (id): id is string => typeof id === "string" && !questIds.has(id),
    );
    if (missing.length) {
      diagnostics.push(
        issue("quest-missing-prerequisite", `Не найдены prerequisites: ${missing.join(", ")}`, [
          quest.id,
        ]),
      );
    }
  }
}

function validateDialogue(document: CanvasDocument, diagnostics: CanvasGraphDiagnostic[]) {
  const nodes = typedNodes(document, "dialogue");
  if (nodes.length === 0) return;
  const ids = new Set(nodes.map((node) => node.id));
  const edges = document.edges.filter(
    (edge) =>
      relationType(edge) === "transitions_to" && ids.has(edge.fromNode) && ids.has(edge.toNode),
  );
  const outgoing = adjacency(ids, edges, "outgoing");
  const incoming = adjacency(ids, edges, "incoming");
  const entries = [...ids].filter((id) => (incoming.get(id)?.length ?? 0) === 0);
  if (entries.length === 0)
    diagnostics.push(issue("dialogue-no-entry", "У диалога нет точки входа", [...ids]));
  const reachable = reachableFrom(entries, outgoing);
  const unreachable = [...ids].filter((id) => !reachable.has(id));
  if (unreachable.length)
    diagnostics.push(issue("dialogue-unreachable", "Есть недостижимые реплики", unreachable));
  for (const node of nodes) {
    const terminal = node["x-writers-studio"]?.card?.fields.terminal === true;
    if (!terminal && (outgoing.get(node.id)?.length ?? 0) === 0) {
      diagnostics.push(
        issue("dialogue-dead-end", "Нетерминальная реплика не имеет продолжения", [node.id]),
      );
    }
  }
  const unconditional = edges.filter(
    (edge) => !edge["x-writers-studio"]?.relation?.fields?.condition,
  );
  for (const cycle of findCycles(ids, adjacency(ids, unconditional, "outgoing"))) {
    diagnostics.push(issue("dialogue-unconditional-cycle", "Бесконечный безусловный цикл", cycle));
  }
}

function validateStates(document: CanvasDocument, diagnostics: CanvasGraphDiagnostic[]) {
  const nodes = typedNodes(document, "state");
  if (nodes.length === 0) return;
  const ids = new Set(nodes.map((node) => node.id));
  const edges = document.edges.filter(
    (edge) =>
      relationType(edge) === "transitions_to" && ids.has(edge.fromNode) && ids.has(edge.toNode),
  );
  const outgoing = adjacency(ids, edges, "outgoing");
  const incoming = adjacency(ids, edges, "incoming");
  const initial = nodes
    .filter((node) => node["x-writers-studio"]?.card?.fields.initial === true)
    .map((node) => node.id);
  const entries = initial.length
    ? initial
    : [...ids].filter((id) => (incoming.get(id)?.length ?? 0) === 0);
  const unreachable = [...ids].filter((id) => !reachableFrom(entries, outgoing).has(id));
  if (unreachable.length)
    diagnostics.push(issue("state-unreachable", "Есть недостижимые состояния", unreachable));
  for (const node of nodes) {
    const terminal = node["x-writers-studio"]?.card?.fields.terminal === true;
    if (!terminal && (outgoing.get(node.id)?.length ?? 0) === 0) {
      diagnostics.push(issue("state-no-exit", "Состояние не имеет выхода", [node.id]));
    }
  }
}

function validateEconomy(document: CanvasDocument, diagnostics: CanvasGraphDiagnostic[]) {
  const resources = typedNodes(document, "resource");
  for (const resource of resources) {
    const produced = document.edges.some(
      (edge) => edge.toNode === resource.id && relationType(edge) === "produces",
    );
    const consumed = document.edges.some(
      (edge) => edge.fromNode === resource.id && relationType(edge) === "consumes",
    );
    if (!produced)
      diagnostics.push(issue("resource-no-producer", "У ресурса нет producer", [resource.id]));
    if (!consumed)
      diagnostics.push(issue("resource-no-consumer", "У ресурса нет consumer", [resource.id]));
  }
}

function typedNodes(document: CanvasDocument, typeId: string): CanvasNode[] {
  return document.nodes.filter((node) => node["x-writers-studio"]?.card?.typeId === typeId);
}

function relationType(edge: CanvasEdge): string {
  return edge["x-writers-studio"]?.relation?.typeId ?? "relates";
}

function isGraphNode(node: CanvasNode): boolean {
  if (node.type !== "group") return true;
  return node["x-writers-studio"]?.element?.kind === "shape";
}

function adjacency(
  nodeIds: Set<string>,
  edges: CanvasEdge[],
  direction: "outgoing" | "incoming",
): Map<string, string[]> {
  const result = new Map([...nodeIds].map((id) => [id, [] as string[]]));
  for (const edge of edges) {
    if (!nodeIds.has(edge.fromNode) || !nodeIds.has(edge.toNode)) continue;
    const from = direction === "outgoing" ? edge.fromNode : edge.toNode;
    const to = direction === "outgoing" ? edge.toNode : edge.fromNode;
    result.get(from)?.push(to);
  }
  return result;
}

function connectedComponents(nodeIds: Set<string>, edges: CanvasEdge[]): string[][] {
  const undirected = new Map([...nodeIds].map((id) => [id, [] as string[]]));
  for (const edge of edges) {
    undirected.get(edge.fromNode)?.push(edge.toNode);
    undirected.get(edge.toNode)?.push(edge.fromNode);
  }
  const visited = new Set<string>();
  const result: string[][] = [];
  for (const id of nodeIds) {
    if (visited.has(id)) continue;
    const component: string[] = [];
    const queue = [id];
    visited.add(id);
    while (queue.length) {
      const current = queue.shift();
      if (!current) break;
      component.push(current);
      for (const next of undirected.get(current) ?? []) {
        if (visited.has(next)) continue;
        visited.add(next);
        queue.push(next);
      }
    }
    result.push(component);
  }
  return result;
}

function findCycles(nodeIds: Set<string>, outgoing: Map<string, string[]>): string[][] {
  const state = new Map<string, 0 | 1 | 2>();
  const stack: string[] = [];
  const signatures = new Set<string>();
  const cycles: string[][] = [];
  const visit = (id: string) => {
    state.set(id, 1);
    stack.push(id);
    for (const next of outgoing.get(id) ?? []) {
      if (state.get(next) === 1) {
        const cycle = stack.slice(stack.indexOf(next));
        const signature = [...cycle].sort().join("|");
        if (!signatures.has(signature)) {
          signatures.add(signature);
          cycles.push(cycle);
        }
      } else if (!state.get(next)) visit(next);
    }
    stack.pop();
    state.set(id, 2);
  };
  for (const id of nodeIds) if (!state.get(id)) visit(id);
  return cycles;
}

function reachableFrom(starts: string[], outgoing: Map<string, string[]>): Set<string> {
  const reached = new Set(starts);
  const queue = [...starts];
  while (queue.length) {
    const current = queue.shift();
    if (!current) break;
    for (const next of outgoing.get(current) ?? []) {
      if (reached.has(next)) continue;
      reached.add(next);
      queue.push(next);
    }
  }
  return reached;
}

function longestPath(
  nodeIds: Set<string>,
  outgoing: Map<string, string[]>,
  incoming: Map<string, string[]>,
): string[] {
  const indegree = new Map([...nodeIds].map((id) => [id, incoming.get(id)?.length ?? 0]));
  const queue = [...nodeIds].filter((id) => indegree.get(id) === 0);
  const distance = new Map([...nodeIds].map((id) => [id, 0]));
  const previous = new Map<string, string>();
  while (queue.length) {
    const current = queue.shift();
    if (!current) break;
    for (const next of outgoing.get(current) ?? []) {
      if ((distance.get(current) ?? 0) + 1 > (distance.get(next) ?? 0)) {
        distance.set(next, (distance.get(current) ?? 0) + 1);
        previous.set(next, current);
      }
      indegree.set(next, (indegree.get(next) ?? 1) - 1);
      if (indegree.get(next) === 0) queue.push(next);
    }
  }
  const end = [...nodeIds].sort(
    (left, right) => (distance.get(right) ?? 0) - (distance.get(left) ?? 0),
  )[0];
  if (!end) return [];
  const path = [end];
  while (previous.has(path[0] ?? "")) path.unshift(previous.get(path[0] ?? "") as string);
  return path;
}

function reconstructPath(previous: Map<string, string>, from: string, to: string): string[] {
  const path = [to];
  while (path[0] !== from) {
    const item = previous.get(path[0] ?? "");
    if (!item) return [];
    path.unshift(item);
  }
  return path;
}

function issue(
  code: string,
  message: string,
  nodeIds: string[],
  edgeIds: string[] = [],
): CanvasGraphDiagnostic {
  return { code, severity: "warning", message, nodeIds, edgeIds };
}
