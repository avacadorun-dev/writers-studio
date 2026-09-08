import { describe, expect, test } from "bun:test";

import {
  appendSampledPoint,
  createShapeNode,
  createStrokeNode,
  eraseCanvasElements,
  pointHitsNode,
  simplifyStrokePoints,
  splitStrokePoints,
  strokeElementPath,
} from "./drawing";

describe("canvas drawing", () => {
  test("samples and simplifies pointer input without losing endpoints or pressure", () => {
    const first = appendSampledPoint([], { x: 0, y: 0, pressure: 0.2 });
    expect(appendSampledPoint(first, { x: 0.1, y: 0.1 })).toBe(first);
    const sampled = appendSampledPoint(first, { x: 4, y: 4, pressure: 0.8 });
    expect(sampled).toHaveLength(2);
    const simplified = simplifyStrokePoints([
      { x: 0, y: 0 },
      { x: 1, y: 0.1 },
      { x: 2, y: 0 },
      { x: 3, y: 4 },
    ]);
    expect(simplified[0]).toEqual({ x: 0, y: 0 });
    expect(simplified.at(-1)).toEqual({ x: 3, y: 4 });
    expect(simplifyStrokePoints([{ x: 0, y: 0 }])).toHaveLength(1);
  });

  test("creates a bounded local stroke and a vector SVG path", () => {
    const node = createStrokeNode(
      "stroke",
      [
        { x: 100, y: 80, pressure: -1 },
        { x: 140, y: 90, pressure: 2 },
        { x: 180, y: 120, pressure: 0.5 },
      ],
      "pen",
      { color: "1", size: 8, opacity: 1 },
    );
    expect(node).toMatchObject({ id: "stroke", type: "group", x: 94, y: 74 });
    const element = node?.["x-writers-studio"]?.element;
    expect(element?.kind).toBe("stroke");
    if (element?.kind !== "stroke") throw new Error("stroke fixture missing");
    expect(element.points[0]?.[2]).toBe(0);
    expect(element.points[1]?.[2]).toBe(1);
    expect(strokeElementPath(element)).toMatch(/^M .* Z$/);
    expect(
      createStrokeNode("short", [{ x: 0, y: 0 }], "pen", {
        color: "1",
        size: 2,
        opacity: 1,
      }),
    ).toBeUndefined();
  });

  test("creates shape geometry and preserves line direction", () => {
    const rectangle = createShapeNode("rect", { x: 100, y: 80 }, { x: 20, y: 10 }, "rectangle", {
      color: "2",
      size: 4,
      opacity: 0.5,
    });
    expect(rectangle).toMatchObject({ x: 20, y: 10, width: 80, height: 70 });
    expect(rectangle["x-writers-studio"]?.element).toMatchObject({
      kind: "shape",
      shape: "rectangle",
      fill: "transparent",
    });
    const line = createShapeNode("line", { x: 10, y: 80 }, { x: 80, y: 10 }, "arrow", {
      color: "3",
      size: 100,
      opacity: 0,
    });
    expect(line["x-writers-studio"]).toMatchObject({
      lineDirection: "ascending",
      element: { size: 32, opacity: 0.01 },
    });
  });

  test("splits strokes at the eraser and hit-tests node bounds", () => {
    expect(
      splitStrokePoints(
        [
          [0, 0],
          [10, 0],
          [20, 0],
          [30, 0],
          [40, 0],
          [50, 0],
        ],
        { x: 25, y: 0 },
        6,
      ),
    ).toEqual([
      [
        [0, 0],
        [10, 0],
      ],
      [
        [40, 0],
        [50, 0],
      ],
    ]);
    expect(splitStrokePoints([[0, 0]], { x: 0, y: 0 }, 1)).toEqual([]);
    const bounds = { x: 10, y: 20, width: 40, height: 60 };
    expect(pointHitsNode({ x: 5, y: 15 }, bounds, 5)).toBe(true);
    expect(pointHitsNode({ x: 100, y: 100 }, bounds)).toBe(false);
  });

  test("applies an eraser gesture atomically and removes dangling edges", () => {
    const stroke = createStrokeNode(
      "stroke",
      [
        { x: 0, y: 0 },
        { x: 10, y: 0 },
        { x: 20, y: 0 },
        { x: 30, y: 0 },
        { x: 40, y: 0 },
        { x: 50, y: 0 },
      ],
      "pen",
      { color: "1", size: 2, opacity: 1 },
    );
    if (!stroke) throw new Error("stroke fixture missing");
    const card = {
      id: "card",
      type: "text" as const,
      x: 100,
      y: 100,
      width: 100,
      height: 80,
      text: "X",
    };
    const document = {
      nodes: [stroke, card],
      edges: [{ id: "edge", fromNode: "card", toNode: "stroke" }],
    };
    expect(eraseCanvasElements(document, [], 5, () => "unused")).toBe(document);
    const untouched = eraseCanvasElements(document, [{ x: 500, y: 500 }], 5, () => "unused");
    expect(untouched).toBe(document);
    let sequence = 0;
    const split = eraseCanvasElements(
      document,
      [{ x: stroke.x + 31, y: stroke.y + 6 }],
      6,
      () => `split-${sequence++}`,
    );
    expect(split.nodes.filter((node) => node.type === "group").length).toBeGreaterThanOrEqual(1);
    const withoutCard = eraseCanvasElements(split, [{ x: 150, y: 140 }], 1, () => "unused");
    expect(withoutCard.nodes.some((node) => node.id === "card")).toBe(false);
    expect(withoutCard.edges).toEqual([]);
  });
});
