import { describe, expect, test } from "bun:test";

import {
  createGroupAroundSelection,
  duplicateCanvasSelection,
  isNodeInside,
  moveGroupContents,
  removeCanvasSelection,
  selectionBounds,
  snapCanvasNodePosition,
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
    expect(snapCanvasNodePosition(moving, [{ ...peer, x: 23, y: 23 }])).toEqual({ x: 0, y: 0 });
    expect(snapCanvasNodePosition(moving, [moving, peer], -1)).toEqual({ x: 0, y: 0 });
  });
});
