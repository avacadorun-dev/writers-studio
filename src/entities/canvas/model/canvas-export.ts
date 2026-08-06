import type { CanvasDocument, CanvasNode } from "./types";

export type CanvasExportFormat = "png" | "svg" | "pdf" | "canvas";
export type CanvasExportScope = "all" | "selection" | "frame";

export interface CanvasExportBounds {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface CanvasExportSelection {
  scope: CanvasExportScope;
  selectedNodeIds?: string[];
  frameId?: string;
}

function isInsideFrame(node: CanvasNode, frame: CanvasNode): boolean {
  return (
    node.x >= frame.x &&
    node.y >= frame.y &&
    node.x + node.width <= frame.x + frame.width &&
    node.y + node.height <= frame.y + frame.height
  );
}

export function canvasExportNodeIds(
  document: CanvasDocument,
  selection: CanvasExportSelection,
): string[] {
  if (selection.scope === "all") return document.nodes.map((node) => node.id);
  if (selection.scope === "selection") {
    const selected = new Set(selection.selectedNodeIds ?? []);
    return document.nodes.filter((node) => selected.has(node.id)).map((node) => node.id);
  }

  const frame = document.nodes.find((node) => node.id === selection.frameId);
  if (frame?.["x-writers-studio"]?.element?.kind !== "frame") return [];
  return document.nodes
    .filter((node) => node.id === frame.id || isInsideFrame(node, frame))
    .map((node) => node.id);
}

export function canvasExportBounds(
  document: CanvasDocument,
  nodeIds: readonly string[],
): CanvasExportBounds | undefined {
  const included = new Set(nodeIds);
  const nodes = document.nodes.filter((node) => included.has(node.id));
  if (nodes.length === 0) return undefined;
  const left = Math.min(...nodes.map((node) => node.x));
  const top = Math.min(...nodes.map((node) => node.y));
  const right = Math.max(...nodes.map((node) => node.x + node.width));
  const bottom = Math.max(...nodes.map((node) => node.y + node.height));
  return { x: left, y: top, width: right - left, height: bottom - top };
}

export function canvasExportDimensions(
  bounds: CanvasExportBounds,
  pixelRatio = 2,
  maxSide = 8_192,
): { width: number; height: number } {
  const paddedWidth = Math.max(1, bounds.width) + 160;
  const paddedHeight = Math.max(1, bounds.height) + 160;
  const scale = Math.min(pixelRatio, maxSide / paddedWidth, maxSide / paddedHeight);
  return {
    width: Math.max(256, Math.round(paddedWidth * scale)),
    height: Math.max(256, Math.round(paddedHeight * scale)),
  };
}

export function canvasExportFileName(title: string, format: CanvasExportFormat): string {
  const slug = title
    .trim()
    .toLocaleLowerCase("ru")
    .replace(/[^\p{L}\p{N}]+/gu, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 80);
  return `${slug || "canvas"}.${format}`;
}
