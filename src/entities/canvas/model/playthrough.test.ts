import { describe, expect, test } from "bun:test";

import {
  advanceCanvasPlaythrough,
  canvasPlaythroughChoices,
  canvasPlaythroughStarts,
  startCanvasPlaythrough,
} from "./playthrough";
import type { CanvasDocument, CanvasNode } from "./types";

function node(id: string, typeId: string, fields: Record<string, unknown> = {}): CanvasNode {
  return {
    id,
    type: "text",
    x: 0,
    y: 0,
    width: 200,
    height: 120,
    text: `# ${id}`,
    "x-writers-studio": { card: { typeId, fields } },
  };
}

describe("canvas play-through", () => {
  test("starts from dialogue roots and advances labelled transitions", () => {
    const document: CanvasDocument = {
      nodes: [node("hello", "dialogue"), node("answer", "dialogue")],
      edges: [
        {
          id: "choice",
          fromNode: "hello",
          toNode: "answer",
          label: "Ответить",
          "x-writers-studio": {
            relation: { typeId: "transitions_to", fields: { condition: "has_radio" } },
          },
        },
      ],
    };
    expect(canvasPlaythroughStarts(document, "dialogue").map((item) => item.id)).toEqual(["hello"]);
    const state = startCanvasPlaythrough("dialogue", "hello");
    const choices = canvasPlaythroughChoices(document, state);
    expect(choices).toEqual([
      { edgeId: "choice", toNodeId: "answer", label: "Ответить", condition: "has_radio" },
    ]);
    expect(advanceCanvasPlaythrough(state, choices[0] as (typeof choices)[number])).toEqual({
      profile: "dialogue",
      path: ["hello", "answer"],
      edgePath: ["choice"],
    });
  });

  test("prefers explicit state initial and supports quest dependencies", () => {
    const document: CanvasDocument = {
      nodes: [
        node("idle", "state", { initial: true }),
        node("play", "state"),
        node("q1", "quest"),
        node("q2", "quest"),
      ],
      edges: [
        {
          id: "state",
          fromNode: "idle",
          toNode: "play",
          "x-writers-studio": { relation: { typeId: "transitions_to" } },
        },
        {
          id: "quest",
          fromNode: "q1",
          toNode: "q2",
          "x-writers-studio": { relation: { typeId: "depends_on" } },
        },
      ],
    };
    expect(canvasPlaythroughStarts(document, "state").map((item) => item.id)).toEqual(["idle"]);
    const quest = startCanvasPlaythrough("quest", "q1");
    expect(canvasPlaythroughChoices(document, quest)[0]?.label).toBe("q2");
    expect(canvasPlaythroughChoices(document, startCanvasPlaythrough("quest", "q2"))).toEqual([]);
  });

  test("falls back to one node for cyclic graphs and handles missing current", () => {
    const document: CanvasDocument = {
      nodes: [node("a", "dialogue"), node("b", "dialogue")],
      edges: [
        {
          id: "ab",
          fromNode: "a",
          toNode: "b",
          "x-writers-studio": { relation: { typeId: "transitions_to" } },
        },
        {
          id: "ba",
          fromNode: "b",
          toNode: "a",
          "x-writers-studio": { relation: { typeId: "transitions_to" } },
        },
      ],
    };
    expect(canvasPlaythroughStarts(document, "dialogue")).toHaveLength(1);
    expect(
      canvasPlaythroughChoices(document, { profile: "dialogue", path: [], edgePath: [] }),
    ).toEqual([]);
  });
});
