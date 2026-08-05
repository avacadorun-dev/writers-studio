import type { DocumentNode, ProjectStructure } from "~entities/project";

export interface ProjectOutlineNode {
  node: DocumentNode;
  children: ProjectOutlineNode[];
}

export interface ProjectOutline {
  roots: ProjectOutlineNode[];
}

export function buildProjectOutline(nodes: DocumentNode[]): ProjectOutline {
  const activeNodes = nodes
    .filter((node) => !node.trashedAt)
    .toSorted(
      (left, right) => left.position - right.position || left.title.localeCompare(right.title),
    );
  const byId = new Map(activeNodes.map((node) => [node.id, node]));
  const cyclic = new Set<string>();
  const state = new Map<string, 0 | 1 | 2>();
  for (const start of activeNodes) {
    if (state.get(start.id) === 2) continue;
    const chain: string[] = [];
    const chainIndex = new Map<string, number>();
    let current: DocumentNode | undefined = start;
    while (current && state.get(current.id) !== 2) {
      const index = chainIndex.get(current.id);
      if (index !== undefined) {
        for (let position = index; position < chain.length; position += 1) {
          cyclic.add(chain[position]);
        }
        break;
      }
      chainIndex.set(current.id, chain.length);
      chain.push(current.id);
      state.set(current.id, 1);
      current = current.parentId ? byId.get(current.parentId) : undefined;
    }
    for (const id of chain) state.set(id, 2);
  }

  const items = new Map<string, ProjectOutlineNode>();
  for (const node of activeNodes) items.set(node.id, { node, children: [] });
  const roots: ProjectOutlineNode[] = [];
  for (const node of activeNodes) {
    const item = items.get(node.id);
    const parent = node.parentId ? items.get(node.parentId) : undefined;
    if (!item) continue;
    if (!parent || cyclic.has(node.id)) roots.push(item);
    else parent.children.push(item);
  }
  return { roots };
}

export function getValidParentNodes(
  nodes: DocumentNode[],
  nodeId: string,
  structure: ProjectStructure,
): DocumentNode[] {
  const child = nodes.find((node) => node.id === nodeId && !node.trashedAt);
  if (!child) {
    return [];
  }
  const blocked = new Set([nodeId, ...getDescendantIds(nodes, nodeId)]);
  return nodes
    .filter(
      (node) =>
        !node.trashedAt &&
        !blocked.has(node.id) &&
        structure.nodeTypes
          .find((nodeType) => nodeType.key === node.kind)
          ?.allowedChildren?.includes(child.kind),
    )
    .toSorted(
      (left, right) => left.position - right.position || left.title.localeCompare(right.title),
    );
}

export function canMoveNodeToParent(
  nodes: DocumentNode[],
  nodeId: string,
  parentId?: string,
  structure?: ProjectStructure,
): boolean {
  const node = nodes.find((candidate) => candidate.id === nodeId && !candidate.trashedAt);
  if (!node || node.parentId === parentId || !structure) {
    return false;
  }
  if (!parentId) {
    return structure.nodeTypes.find((item) => item.key === node.kind)?.canBeRoot === true;
  }
  return getValidParentNodes(nodes, nodeId, structure).some(
    (candidate) => candidate.id === parentId,
  );
}

export function canPlaceNodeAtParent(
  nodes: DocumentNode[],
  nodeId: string,
  parentId: string | undefined,
  structure: ProjectStructure,
): boolean {
  const node = nodes.find((candidate) => candidate.id === nodeId && !candidate.trashedAt);
  if (!node) return false;
  if (node.parentId === parentId) return true;
  if (!parentId) {
    return structure.nodeTypes.find((item) => item.key === node.kind)?.canBeRoot === true;
  }
  return getValidParentNodes(nodes, nodeId, structure).some(
    (candidate) => candidate.id === parentId,
  );
}

export function siblingIndex(nodes: DocumentNode[], nodeId: string): number {
  const node = nodes.find((candidate) => candidate.id === nodeId);
  if (!node) return -1;
  return orderedSiblings(nodes, node.parentId).findIndex((candidate) => candidate.id === nodeId);
}

export function orderedSiblings(nodes: DocumentNode[], parentId?: string): DocumentNode[] {
  return nodes
    .filter((node) => !node.trashedAt && node.parentId === parentId)
    .toSorted(
      (left, right) =>
        left.position - right.position || left.createdAt.localeCompare(right.createdAt),
    );
}

export function moveNodeInOutline(
  nodes: DocumentNode[],
  nodeId: string,
  parentId: string | undefined,
  targetIndex: number,
): DocumentNode[] {
  const moved = nodes.find((node) => node.id === nodeId);
  if (!moved) return nodes;
  const oldParentId = moved.parentId;
  const target = orderedSiblings(nodes, parentId).filter((node) => node.id !== nodeId);
  if (targetIndex < 0 || targetIndex > target.length) return nodes;
  target.splice(targetIndex, 0, { ...moved, parentId });
  const positions = new Map<string, { parentId?: string; position: number }>();
  target.forEach((node, index) => {
    positions.set(node.id, { parentId, position: (index + 1) * 1_000 });
  });
  if (oldParentId !== parentId) {
    orderedSiblings(nodes, oldParentId)
      .filter((node) => node.id !== nodeId)
      .forEach((node, index) => {
        positions.set(node.id, { parentId: oldParentId, position: (index + 1) * 1_000 });
      });
  }
  return nodes.map((node) => {
    const placement = positions.get(node.id);
    return placement ? { ...node, ...placement } : node;
  });
}

export function flattenOutlineIds(items: ProjectOutlineNode[]): string[] {
  return items.flatMap((item) => [item.node.id, ...flattenOutlineIds(item.children)]);
}

export function nextNodePosition(nodes: DocumentNode[], parentId?: string): number {
  return (
    Math.max(
      0,
      ...nodes
        .filter((node) => !node.trashedAt && node.parentId === parentId)
        .map((node) => node.position),
    ) + 1_000
  );
}

function getDescendantIds(nodes: DocumentNode[], nodeId: string): string[] {
  const result: string[] = [];
  const pending = [nodeId];
  const visited = new Set<string>(pending);
  const children = new Map<string, DocumentNode[]>();
  for (const node of nodes) {
    if (!node.parentId) continue;
    const siblings = children.get(node.parentId) ?? [];
    siblings.push(node);
    children.set(node.parentId, siblings);
  }
  for (let index = 0; index < pending.length; index += 1) {
    const parentId = pending[index];
    for (const node of children.get(parentId) ?? []) {
      if (visited.has(node.id)) continue;
      visited.add(node.id);
      result.push(node.id);
      pending.push(node.id);
    }
  }
  return result;
}

export function nodeTypeLabel(structure: ProjectStructure, kind: string): string {
  return structure.nodeTypes.find((nodeType) => nodeType.key === kind)?.label ?? kind;
}
