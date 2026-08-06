import { describe, expect, test } from "bun:test";

import {
  commitCanvasHistory,
  createCanvasHistory,
  redoCanvasHistory,
  undoCanvasHistory,
} from "./canvas-history";
import type { CanvasDocument } from "./types";

const empty: CanvasDocument = { nodes: [], edges: [] };

describe("canvas history", () => {
  test("undoes and redoes semantic document snapshots", () => {
    const changed: CanvasDocument = {
      nodes: [{ id: "n", type: "text", x: 0, y: 0, width: 200, height: 100, text: "Идея" }],
      edges: [],
    };
    const committed = commitCanvasHistory(createCanvasHistory(empty), changed);
    const undone = undoCanvasHistory(committed);
    expect(undone.present).toEqual(empty);
    expect(redoCanvasHistory(undone).present).toEqual(changed);
  });

  test("ignores no-op transitions and limits retained history", () => {
    const initial = createCanvasHistory(empty);
    expect(commitCanvasHistory(initial, empty)).toBe(initial);
    expect(commitCanvasHistory(initial, { nodes: [], edges: [] })).toBe(initial);
    expect(undoCanvasHistory(initial)).toBe(initial);
    expect(redoCanvasHistory(initial)).toBe(initial);
    const one = commitCanvasHistory(initial, { nodes: [], edges: [], step: 1 }, 1);
    const two = commitCanvasHistory(one, { nodes: [], edges: [], step: 2 }, 1);
    expect(two.past).toHaveLength(1);
    expect(two.past[0]).toMatchObject({ step: 1 });
  });
});
