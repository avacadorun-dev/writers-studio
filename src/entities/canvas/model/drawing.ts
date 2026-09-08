import { getStroke } from "perfect-freehand";

import type {
  CanvasColor,
  CanvasDocument,
  CanvasGroupNode,
  CanvasShapeElement,
  CanvasStrokeElement,
  CanvasStrokePoint,
} from "./types";

export interface CanvasPoint {
  x: number;
  y: number;
  pressure?: number;
}

export interface CanvasDrawingStyle {
  color: CanvasColor;
  size: number;
  opacity: number;
}

const drawingPadding = 6;
const minimumNodeSize = 40;

export function appendSampledPoint(
  points: CanvasPoint[],
  point: CanvasPoint,
  minimumDistance = 0.75,
): CanvasPoint[] {
  const last = points.at(-1);
  if (!last) return [point];
  if (Math.hypot(point.x - last.x, point.y - last.y) < Math.max(0, minimumDistance)) {
    return points;
  }
  return [...points, point];
}

export function simplifyStrokePoints(
  points: CanvasPoint[],
  tolerance = 0.8,
  maximumPoints = 4_096,
): CanvasPoint[] {
  if (points.length <= 2) return points;
  const stride = Math.max(1, Math.ceil(points.length / Math.max(2, maximumPoints)));
  const sampled = points.filter(
    (_, index) => index === 0 || index === points.length - 1 || index % stride === 0,
  );
  return ramerDouglasPeucker(sampled, Math.max(0, tolerance));
}

export function createStrokeNode(
  id: string,
  points: CanvasPoint[],
  brush: CanvasStrokeElement["brush"],
  style: CanvasDrawingStyle,
): CanvasGroupNode | undefined {
  const simplified = simplifyStrokePoints(points);
  if (simplified.length < 2) return undefined;
  const radius = Math.max(style.size / 2, drawingPadding);
  const left = Math.floor(Math.min(...simplified.map((point) => point.x)) - radius);
  const top = Math.floor(Math.min(...simplified.map((point) => point.y)) - radius);
  const right = Math.ceil(Math.max(...simplified.map((point) => point.x)) + radius);
  const bottom = Math.ceil(Math.max(...simplified.map((point) => point.y)) + radius);
  return {
    id,
    type: "group",
    x: left,
    y: top,
    width: Math.max(minimumNodeSize, right - left),
    height: Math.max(minimumNodeSize, bottom - top),
    "x-writers-studio": {
      element: {
        kind: "stroke",
        points: simplified.map((point) =>
          point.pressure === undefined
            ? [round(point.x - left), round(point.y - top)]
            : [round(point.x - left), round(point.y - top), clamp(point.pressure, 0, 1)],
        ),
        brush,
        color: style.color,
        size: clamp(style.size, 1, 128),
        opacity: clamp(style.opacity, 0.01, 1),
      },
    },
  };
}

export function createShapeNode(
  id: string,
  start: CanvasPoint,
  end: CanvasPoint,
  shape: CanvasShapeElement["shape"],
  style: CanvasDrawingStyle,
): CanvasGroupNode {
  const isLinear = shape === "line" || shape === "arrow";
  const left = Math.floor(Math.min(start.x, end.x));
  const top = Math.floor(Math.min(start.y, end.y));
  const width = Math.max(minimumNodeSize, Math.round(Math.abs(end.x - start.x)));
  const height = Math.max(minimumNodeSize, Math.round(Math.abs(end.y - start.y)));
  return {
    id,
    type: "group",
    x: left,
    y: top,
    width,
    height,
    "x-writers-studio": {
      element: {
        kind: "shape",
        shape,
        color: style.color,
        fill: "transparent",
        size: clamp(style.size, 1, 32),
        opacity: clamp(style.opacity, 0.01, 1),
        rotation: 0,
        dash: "solid",
      },
      lineDirection: isLinear && end.x < start.x !== end.y < start.y ? "ascending" : "descending",
    },
  };
}

export function strokeElementPath(element: CanvasStrokeElement): string {
  const outline = getStroke(
    element.points.map((point) =>
      point[2] === undefined
        ? { x: point[0], y: point[1] }
        : { x: point[0], y: point[1], pressure: point[2] },
    ),
    {
      size: element.size,
      thinning: element.brush === "highlighter" ? 0 : 0.55,
      smoothing: 0.55,
      streamline: 0.5,
      simulatePressure: element.points.every((point) => point[2] === undefined),
      easing: (value) => value,
      start: { cap: true, taper: 0 },
      end: { cap: true, taper: 0 },
    },
  );
  return outlineToSvgPath(outline);
}

export function splitStrokePoints(
  points: CanvasStrokePoint[],
  center: CanvasPoint,
  radius: number,
): CanvasStrokePoint[][] {
  if (points.length < 2) return [];
  const kept: CanvasStrokePoint[][] = [];
  let current: CanvasStrokePoint[] = [];
  for (const point of points) {
    if (Math.hypot(point[0] - center.x, point[1] - center.y) > Math.max(0, radius)) {
      current.push(point);
      continue;
    }
    if (current.length >= 2) kept.push(current);
    current = [];
  }
  if (current.length >= 2) kept.push(current);
  return kept;
}

export function pointHitsNode(
  point: CanvasPoint,
  node: Pick<CanvasGroupNode, "x" | "y" | "width" | "height">,
  padding = 0,
): boolean {
  return (
    point.x >= node.x - padding &&
    point.y >= node.y - padding &&
    point.x <= node.x + node.width + padding &&
    point.y <= node.y + node.height + padding
  );
}

export function eraseCanvasElements(
  document: CanvasDocument,
  path: CanvasPoint[],
  radius: number,
  idFactory: () => string,
): CanvasDocument {
  if (path.length === 0) return document;
  const removed = new Set<string>();
  const nextNodes = document.nodes.flatMap((node) => {
    if (!path.some((point) => pointHitsNode(point, node, radius))) return [node];
    const element = node.type === "group" ? node["x-writers-studio"]?.element : undefined;
    if (element?.kind !== "stroke") {
      removed.add(node.id);
      return [];
    }
    let segments: CanvasStrokePoint[][] = [element.points];
    for (const point of path) {
      const local = { x: point.x - node.x, y: point.y - node.y };
      segments = segments.flatMap((segment) => splitStrokePoints(segment, local, radius));
      if (segments.length === 0) break;
    }
    const retainedPointCount = segments.reduce((total, segment) => total + segment.length, 0);
    if (segments.length === 1 && retainedPointCount === element.points.length) return [node];
    removed.add(node.id);
    return segments.flatMap((segment, index) => {
      const replacement = createStrokeNode(
        index === 0 ? node.id : idFactory(),
        segment.map((point) => ({
          x: node.x + point[0],
          y: node.y + point[1],
          ...(point[2] === undefined ? {} : { pressure: point[2] }),
        })),
        element.brush,
        { color: element.color, size: element.size, opacity: element.opacity },
      );
      if (replacement && index === 0) removed.delete(node.id);
      return replacement ? [replacement] : [];
    });
  });
  if (removed.size === 0 && nextNodes.every((node, index) => node === document.nodes[index])) {
    return document;
  }
  const nextNodeIds = new Set(nextNodes.map((node) => node.id));
  return {
    ...document,
    nodes: nextNodes,
    edges: document.edges.filter(
      (edge) => nextNodeIds.has(edge.fromNode) && nextNodeIds.has(edge.toNode),
    ),
  };
}

function ramerDouglasPeucker(points: CanvasPoint[], tolerance: number): CanvasPoint[] {
  if (points.length <= 2 || tolerance === 0) return points;
  const first = points[0];
  const last = points.at(-1);
  if (!first || !last) return points;
  let furthestIndex = -1;
  let furthestDistance = 0;
  for (let index = 1; index < points.length - 1; index += 1) {
    const point = points[index];
    if (!point) continue;
    const distance = perpendicularDistance(point, first, last);
    if (distance > furthestDistance) {
      furthestDistance = distance;
      furthestIndex = index;
    }
  }
  if (furthestIndex < 0 || furthestDistance <= tolerance) return [first, last];
  const left = ramerDouglasPeucker(points.slice(0, furthestIndex + 1), tolerance);
  const right = ramerDouglasPeucker(points.slice(furthestIndex), tolerance);
  return [...left.slice(0, -1), ...right];
}

function perpendicularDistance(point: CanvasPoint, start: CanvasPoint, end: CanvasPoint): number {
  const dx = end.x - start.x;
  const dy = end.y - start.y;
  if (dx === 0 && dy === 0) return Math.hypot(point.x - start.x, point.y - start.y);
  const projection = clamp(
    ((point.x - start.x) * dx + (point.y - start.y) * dy) / (dx * dx + dy * dy),
    0,
    1,
  );
  return Math.hypot(point.x - (start.x + projection * dx), point.y - (start.y + projection * dy));
}

function outlineToSvgPath(points: number[][]): string {
  const first = points[0];
  if (!first) return "";
  if (points.length === 1) return `M ${round(first[0] ?? 0)} ${round(first[1] ?? 0)} Z`;
  let path = `M ${round(first[0] ?? 0)} ${round(first[1] ?? 0)} Q`;
  for (let index = 1; index < points.length; index += 1) {
    const point = points[index];
    const next = points[(index + 1) % points.length];
    if (!point || !next) continue;
    path += ` ${round(point[0] ?? 0)} ${round(point[1] ?? 0)} ${round(((point[0] ?? 0) + (next[0] ?? 0)) / 2)} ${round(((point[1] ?? 0) + (next[1] ?? 0)) / 2)}`;
  }
  return `${path} Z`;
}

function round(value: number): number {
  return Math.round(value * 100) / 100;
}

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.min(maximum, Math.max(minimum, value));
}
