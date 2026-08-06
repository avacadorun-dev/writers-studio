import { describe, expect, test } from "bun:test";

import { projectCanvasView } from "./canvas-views";
import type { CanvasDocument, CanvasNode } from "./types";

function node(id: string, typeId?: string, date?: string, x = 0): CanvasNode {
  return {
    id,
    type: "text",
    x,
    y: 0,
    width: 200,
    height: 120,
    text: id,
    "x-writers-studio": typeId ? { card: { typeId, fields: date ? { date } : {} } } : undefined,
  };
}

describe("canvas view projections", () => {
  const document: CanvasDocument = {
    nodes: [
      node("b", "beat", "2027-02-01"),
      node("a", "beat", "2027-01-01"),
      node("q", "quest", undefined, 5),
      node("u"),
    ],
    edges: [
      { id: "ab", fromNode: "a", toNode: "b" },
      { id: "bq", fromNode: "b", toNode: "q" },
    ],
  };

  test("keeps free canvas canonical and filters without copying records", () => {
    expect(projectCanvasView(document, "free")).toBe(document);
    expect(projectCanvasView(document, "filtered")).toBe(document);
    const filtered = projectCanvasView(document, "filtered", "beat");
    expect(filtered.nodes.map((item) => item.id)).toEqual(["b", "a"]);
    expect(filtered.edges.map((item) => item.id)).toEqual(["ab"]);
    expect(document.nodes).toHaveLength(4);
  });

  test("projects timeline, swimlanes, hierarchy and mind map deterministically", () => {
    const timeline = projectCanvasView(document, "timeline");
    expect(timeline.nodes.find((item) => item.id === "a")?.x).toBe(0);
    expect(timeline.nodes.find((item) => item.id === "b")?.x).toBe(340);
    const lanes = projectCanvasView(document, "swimlanes");
    expect(lanes.nodes.find((item) => item.id === "a")?.y).toBe(
      lanes.nodes.find((item) => item.id === "b")?.y,
    );
    expect(lanes.nodes.find((item) => item.id === "q")?.y).not.toBe(
      lanes.nodes.find((item) => item.id === "a")?.y,
    );
    const dependency = projectCanvasView(document, "dependency");
    expect(dependency.nodes.find((item) => item.id === "q")?.x).toBeGreaterThan(
      dependency.nodes.find((item) => item.id === "a")?.x ?? 0,
    );
    const mindMap = projectCanvasView(document, "mind-map");
    expect(mindMap.nodes.find((item) => item.id === "b")?.x).not.toBe(0);
  });

  test("handles cycles and invalid dates without hanging", () => {
    const cyclic: CanvasDocument = {
      nodes: [node("a", "beat", "invalid"), node("b", "beat")],
      edges: [
        { id: "ab", fromNode: "a", toNode: "b" },
        { id: "ba", fromNode: "b", toNode: "a" },
      ],
    };
    expect(projectCanvasView(cyclic, "dependency").nodes.map((item) => item.x)).toEqual([0, 0]);
    expect(projectCanvasView(cyclic, "timeline").nodes.map((item) => item.id)).toEqual(["a", "b"]);
  });
});
