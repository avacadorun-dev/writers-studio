import { describe, expect, test } from "bun:test";

import { strokeElementPath } from "./drawing";
import { analyzeCanvasGraph } from "./graph-analysis";
import type { CanvasEdge, CanvasStrokeElement, CanvasTextNode } from "./types";

describe("canvas performance smoke", () => {
  test("analyzes the release-size graph without blocking the UI budget", () => {
    const nodes: CanvasTextNode[] = Array.from({ length: 500 }, (_, index) => ({
      id: `node-${index}`,
      type: "text",
      x: (index % 25) * 320,
      y: Math.floor(index / 25) * 220,
      width: 280,
      height: 180,
      text: `Idea ${index}`,
    }));
    const edges: CanvasEdge[] = [];
    for (let index = 0; index < 499; index += 1) {
      edges.push({ id: `chain-${index}`, fromNode: `node-${index}`, toNode: `node-${index + 1}` });
    }
    for (let index = 0; index < 499; index += 1) {
      edges.push({
        id: `skip-${index}`,
        fromNode: `node-${index}`,
        toNode: `node-${Math.min(index + 2, 499)}`,
      });
    }
    edges.push(
      { id: "shortcut-mid", fromNode: "node-0", toNode: "node-250" },
      { id: "shortcut-end", fromNode: "node-0", toNode: "node-499" },
    );

    const startedAt = performance.now();
    const analysis = analyzeCanvasGraph({ nodes, edges }, "generic");
    const elapsed = performance.now() - startedAt;

    expect(edges).toHaveLength(1_000);
    expect(analysis.components).toHaveLength(1);
    expect(analysis.cycles).toEqual([]);
    expect(analysis.criticalPath).toHaveLength(500);
    expect(elapsed).toBeLessThan(1_000);
  });

  test("builds vector paths for 2,000 visible strokes within a smoke budget", () => {
    const strokes: CanvasStrokeElement[] = Array.from({ length: 2_000 }, (_, index) => ({
      kind: "stroke",
      points: [
        [0, index % 20, 0.2],
        [20, (index % 20) + 5, 0.6],
        [40, index % 20, 0.9],
      ],
      brush: index % 2 === 0 ? "pen" : "highlighter",
      color: "5",
      size: index % 2 === 0 ? 4 : 18,
      opacity: index % 2 === 0 ? 1 : 0.32,
    }));

    const startedAt = performance.now();
    const paths = strokes.map(strokeElementPath);
    const elapsed = performance.now() - startedAt;

    expect(paths).toHaveLength(2_000);
    expect(paths.every((path) => path.startsWith("M ") && path.endsWith(" Z"))).toBe(true);
    expect(elapsed).toBeLessThan(2_000);
  });
});
