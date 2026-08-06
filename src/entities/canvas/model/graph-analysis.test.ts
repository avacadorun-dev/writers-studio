import { describe, expect, test } from "bun:test";

import { analyzeCanvasGraph, shortestCanvasPath } from "./graph-analysis";
import type { CanvasDocument, CanvasNode } from "./types";

function typedNode(id: string, typeId: string, fields: Record<string, unknown> = {}): CanvasNode {
  return {
    id,
    type: "text",
    x: 0,
    y: 0,
    width: 100,
    height: 80,
    text: id,
    "x-writers-studio": { card: { typeId, fields } },
  };
}

function relation(
  id: string,
  fromNode: string,
  toNode: string,
  typeId: string,
  fields?: Record<string, unknown>,
) {
  return { id, fromNode, toNode, "x-writers-studio": { relation: { typeId, fields } } };
}

describe("canvas graph analysis", () => {
  test("finds components, roots, leaves, shortest and critical paths", () => {
    const document: CanvasDocument = {
      nodes: [
        typedNode("a", "idea"),
        typedNode("b", "idea"),
        typedNode("c", "idea"),
        typedNode("orphan", "idea"),
      ],
      edges: [relation("ab", "a", "b", "causes"), relation("bc", "b", "c", "causes")],
    };
    const analysis = analyzeCanvasGraph(document, "generic");
    expect(analysis.components).toHaveLength(2);
    expect(analysis.orphans).toEqual(["orphan"]);
    expect(analysis.roots).toEqual(["a", "orphan"]);
    expect(analysis.leaves).toEqual(["c", "orphan"]);
    expect(analysis.criticalPath).toEqual(["a", "b", "c"]);
    expect(shortestCanvasPath(document, "a", "c")).toEqual(["a", "b", "c"]);
    expect(shortestCanvasPath(document, "a", "a")).toEqual(["a"]);
    expect(shortestCanvasPath(document, "c", "a")).toEqual([]);
    expect(shortestCanvasPath(document, "missing", "a")).toEqual([]);
  });

  test("reports generic and quest cycles plus missing prerequisites", () => {
    const document: CanvasDocument = {
      nodes: [typedNode("q1", "quest", { prerequisites: ["missing"] }), typedNode("q2", "quest")],
      edges: [relation("one", "q1", "q2", "depends_on"), relation("two", "q2", "q1", "depends_on")],
    };
    const analysis = analyzeCanvasGraph(document, "quest");
    expect(analysis.cycles).toEqual([["q1", "q2"]]);
    expect(analysis.criticalPath).toEqual([]);
    expect(analysis.diagnostics.map((item) => item.code)).toEqual([
      "cycle",
      "quest-cycle",
      "quest-missing-prerequisite",
    ]);
  });

  test("validates dialogue entry, reachability, dead ends and unconditional loops", () => {
    const document: CanvasDocument = {
      nodes: [
        typedNode("a", "dialogue", { line: "A" }),
        typedNode("b", "dialogue", { line: "B" }),
        typedNode("c", "dialogue", { line: "C" }),
      ],
      edges: [
        relation("ab", "a", "b", "transitions_to"),
        relation("ba", "b", "a", "transitions_to"),
      ],
    };
    const codes = analyzeCanvasGraph(document, "dialogue").diagnostics.map((item) => item.code);
    expect(codes).toContain("dialogue-unreachable");
    expect(codes).toContain("dialogue-dead-end");
    expect(codes).toContain("dialogue-unconditional-cycle");
    const loopOnly: CanvasDocument = {
      nodes: document.nodes.slice(0, 2),
      edges: document.edges,
    };
    expect(analyzeCanvasGraph(loopOnly, "dialogue").diagnostics.map((item) => item.code)).toContain(
      "dialogue-no-entry",
    );

    document.edges[1] = relation("ba", "b", "a", "transitions_to", { condition: "retry" });
    expect(
      analyzeCanvasGraph(document, "dialogue").diagnostics.map((item) => item.code),
    ).not.toContain("dialogue-unconditional-cycle");
  });

  test("validates state reachability/exits and economy producers/consumers", () => {
    const document: CanvasDocument = {
      nodes: [
        typedNode("start", "state", { initial: true }),
        typedNode("end", "state", { terminal: true }),
        typedNode("lost", "state"),
        typedNode("gold", "resource"),
        typedNode("wood", "resource"),
        typedNode("system", "mechanic"),
      ],
      edges: [
        relation("transition", "start", "end", "transitions_to"),
        relation("produce", "system", "gold", "produces"),
        relation("consume", "gold", "system", "consumes"),
      ],
    };
    const codes = analyzeCanvasGraph(document, "all").diagnostics.map((item) => item.code);
    expect(codes).toContain("state-unreachable");
    expect(codes).toContain("state-no-exit");
    expect(codes.filter((code) => code === "resource-no-producer")).toHaveLength(1);
    expect(codes.filter((code) => code === "resource-no-consumer")).toHaveLength(1);
  });
});
