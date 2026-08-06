import { describe, expect, test } from "bun:test";

import {
  alignCanvasSelection,
  createFrameAroundSelection,
  createGroupAroundSelection,
  distributeCanvasSelection,
  duplicateCanvasSelection,
  isNodeInside,
  moveGroupContents,
  removeCanvasSelection,
  rotateCanvasSelection,
  selectionBounds,
  snapCanvasNodePosition,
  snapCanvasNodeWithGuides,
  tidyCanvasSelection,
} from "./canvas-operations";
import type { CanvasDocument } from "./types";

const document: CanvasDocument = {
  nodes: [
    { id: "a", type: "text", x: 100, y: 100, width: 200, height: 120, text: "A" },
    { id: "b", type: "text", x: 360, y: 120, width: 200, height: 120, text: "B" },
  ],
  edges: [{ id: "ab", fromNode: "a", toNode: "b", toEnd: "arrow" }],
  "x-writers-studio": { schemaVersion: 1, future: { preserved: true } },
};

describe("canvas operations", () => {
  test("creates a spatial group and moves enclosed nodes with it", () => {
    const grouped = createGroupAroundSelection(document, ["a", "b"], () => "group");
    const group = grouped.document.nodes.find((item) => item.id === "group");
    expect(group?.type).toBe("group");
    const moved = moveGroupContents(grouped.document, "group", { x: 50, y: -20 });
    expect(moved.nodes.find((item) => item.id === "a")).toMatchObject({ x: 150, y: 80 });
    expect(moved.nodes.find((item) => item.id === "b")).toMatchObject({ x: 410, y: 100 });
  });

  test("duplicates selected nodes and only their internal edges", () => {
    let sequence = 0;
    const duplicated = duplicateCanvasSelection(
      document,
      ["a", "b"],
      [],
      () => `copy-${sequence++}`,
    );
    expect(duplicated.nodeIds).toEqual(["copy-0", "copy-1"]);
    expect(duplicated.document.edges.at(-1)).toMatchObject({
      id: "copy-2",
      fromNode: "copy-0",
      toNode: "copy-1",
    });
    expect(duplicated.document["x-writers-studio"]).toEqual(document["x-writers-studio"]);
  });

  test("removing a group keeps its enclosed cards but removing a card removes edges", () => {
    const grouped = createGroupAroundSelection(document, ["a", "b"], () => "group").document;
    const withoutGroup = removeCanvasSelection(grouped, ["group"], []);
    expect(withoutGroup.nodes.map((item) => item.id)).toEqual(["a", "b"]);
    const withoutA = removeCanvasSelection(document, ["a"], []);
    expect(withoutA.edges).toEqual([]);
  });

  test("handles empty, stationary and expanded group selections", () => {
    expect(selectionBounds([])).toBeUndefined();
    expect(createGroupAroundSelection(document, [], () => "unused").groupId).toBeUndefined();
    expect(moveGroupContents(document, "missing", { x: 1, y: 1 })).toBe(document);
    const grouped = createGroupAroundSelection(document, ["a", "b"], () => "group", "Act", "4");
    expect(moveGroupContents(grouped.document, "group", { x: 0, y: 0 })).toBe(grouped.document);
    const group = grouped.document.nodes[0];
    if (!group) throw new Error("fixture group is missing");
    const firstNode = document.nodes[0];
    if (!firstNode) throw new Error("fixture node is missing");
    expect(isNodeInside(firstNode, group)).toBe(true);
    const duplicated = duplicateCanvasSelection(
      grouped.document,
      ["group"],
      ["ab"],
      (() => {
        let id = 0;
        return () => `id-${id++}`;
      })(),
    );
    expect(duplicated.nodeIds).toHaveLength(3);
    expect(duplicated.edgeIds).toHaveLength(1);
    expect(removeCanvasSelection(document, [], ["ab"]).edges).toEqual([]);
  });

  test("snaps differently sized cards by their handle centers", () => {
    const moving = { id: "document", x: 0, y: 0, width: 280, height: 180 };
    const peer = { id: "note", x: 0, y: 0, width: 260, height: 160 };
    expect(snapCanvasNodePosition(moving, [peer])).toEqual({ x: -10, y: -10 });
    expect(snapCanvasNodePosition(moving, [{ ...peer, x: 23, y: 23 }])).toEqual({ x: 3, y: 3 });
    expect(snapCanvasNodePosition(moving, [moving, peer], -1)).toEqual({ x: 0, y: 0 });
    const edgePeer = { ...peer, x: 100, width: 100 };
    expect(snapCanvasNodePosition({ ...moving, x: 94 }, [edgePeer])).toEqual({ x: 100, y: -10 });
    expect(snapCanvasNodeWithGuides({ ...moving, x: 94 }, [edgePeer])).toMatchObject({
      position: { x: 100, y: -10 },
      vertical: { position: 100 },
      horizontal: { position: 80 },
    });
  });

  test("rotates only selected shapes and normalizes the angle", () => {
    const shape = {
      id: "shape",
      type: "group" as const,
      x: 0,
      y: 0,
      width: 100,
      height: 100,
      "x-writers-studio": {
        element: {
          kind: "shape" as const,
          shape: "rectangle" as const,
          color: "1" as const,
          fill: "transparent" as const,
          size: 2,
          opacity: 1,
          rotation: 170,
          dash: "solid" as const,
        },
      },
    };
    const document = { nodes: [shape, { ...shape, id: "other" }], edges: [] };
    const rotated = rotateCanvasSelection(document, ["shape"], 30);
    expect(rotated.nodes[0]?.["x-writers-studio"]?.element).toMatchObject({ rotation: -160 });
    expect(rotated.nodes[1]?.["x-writers-studio"]?.element).toMatchObject({ rotation: 170 });
    expect(rotateCanvasSelection(document, ["shape"], Number.NaN)).toBe(document);
  });

  test("aligns, distributes and tidies a selection as single document operations", () => {
    const source: CanvasDocument = {
      nodes: [
        { id: "a", type: "text", x: 0, y: 20, width: 100, height: 80, text: "A" },
        { id: "b", type: "text", x: 160, y: 100, width: 80, height: 100, text: "B" },
        { id: "c", type: "text", x: 400, y: 240, width: 120, height: 60, text: "C" },
        { id: "d", type: "text", x: 900, y: 900, width: 40, height: 40, text: "D" },
      ],
      edges: [],
    };
    expect(alignCanvasSelection(source, ["a"], "left")).toBe(source);
    expect(alignCanvasSelection(source, ["a", "b", "c"], "center-y").nodes.slice(0, 3)).toEqual([
      { ...source.nodes[0], y: 120 },
      { ...source.nodes[1], y: 110 },
      { ...source.nodes[2], y: 130 },
    ]);
    expect(alignCanvasSelection(source, ["a", "b", "c"], "right").nodes[0]?.x).toBe(420);
    const horizontal = distributeCanvasSelection(source, ["a", "b", "c"], "horizontal");
    expect(horizontal.nodes.map((node) => node.x).slice(0, 3)).toEqual([0, 210, 400]);
    const vertical = distributeCanvasSelection(source, ["a", "b", "c"], "vertical");
    expect(vertical.nodes[1]?.y).toBe(120);
    expect(distributeCanvasSelection(source, ["a", "b"], "horizontal")).toBe(source);
    const tidy = tidyCanvasSelection(source, ["a", "b", "c"], 20);
    expect(tidy.nodes.slice(0, 3).map(({ x, y }) => ({ x, y }))).toEqual([
      { x: 0, y: 20 },
      { x: 140, y: 20 },
      { x: 0, y: 140 },
    ]);
    expect(tidy.nodes[3]).toBe(source.nodes[3]);
    expect(tidyCanvasSelection(source, [], 20)).toBe(source);
  });

  test("creates ordered presentation frames around selected elements", () => {
    const first = createFrameAroundSelection(document, ["a", "b"], () => "frame-1", "Акт I");
    expect(first.frameId).toBe("frame-1");
    expect(first.document.nodes[0]).toMatchObject({
      x: 52,
      y: 36,
      width: 556,
      height: 252,
      "x-writers-studio": { element: { kind: "frame", order: 1, label: "Акт I" } },
    });
    const second = createFrameAroundSelection(first.document, ["a"], () => "frame-2");
    expect(second.document.nodes[0]?.["x-writers-studio"]?.element).toMatchObject({ order: 2 });
    expect(createFrameAroundSelection(document, [], () => "unused").document).toBe(document);
  });
});
