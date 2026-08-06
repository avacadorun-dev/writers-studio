import { describe, expect, test } from "bun:test";
import {
  canvasExportBounds,
  canvasExportDimensions,
  canvasExportFileName,
  canvasExportNodeIds,
} from "./canvas-export";
import type { CanvasDocument } from "./types";

const document: CanvasDocument = {
  nodes: [
    {
      id: "frame",
      type: "group",
      x: 0,
      y: 0,
      width: 600,
      height: 400,
      label: "Act I",
      "x-writers-studio": { element: { kind: "frame", order: 1 } },
    },
    { id: "inside", type: "text", x: 50, y: 60, width: 200, height: 120, text: "Idea" },
    { id: "outside", type: "text", x: 700, y: 500, width: 300, height: 200, text: "Later" },
  ],
  edges: [],
};

describe("canvas export model", () => {
  test("resolves all, selection, and spatial frame scopes", () => {
    expect(canvasExportNodeIds(document, { scope: "all" })).toEqual(["frame", "inside", "outside"]);
    expect(
      canvasExportNodeIds(document, {
        scope: "selection",
        selectedNodeIds: ["outside", "missing"],
      }),
    ).toEqual(["outside"]);
    expect(canvasExportNodeIds(document, { scope: "frame", frameId: "frame" })).toEqual([
      "frame",
      "inside",
    ]);
    expect(canvasExportNodeIds(document, { scope: "frame", frameId: "inside" })).toEqual([]);
  });

  test("calculates bounds and handles empty selections", () => {
    expect(canvasExportBounds(document, ["inside", "outside"])).toEqual({
      x: 50,
      y: 60,
      width: 950,
      height: 640,
    });
    expect(canvasExportBounds(document, [])).toBeUndefined();
  });

  test("uses high resolution while respecting the side and minimum limits", () => {
    expect(canvasExportDimensions({ x: 0, y: 0, width: 840, height: 340 })).toEqual({
      width: 2_000,
      height: 1_000,
    });
    expect(canvasExportDimensions({ x: 0, y: 0, width: 20_000, height: 10_000 })).toEqual({
      width: 8_192,
      height: 4_129,
    });
    expect(canvasExportDimensions({ x: 0, y: 0, width: 1, height: 1 }, 1)).toEqual({
      width: 256,
      height: 256,
    });
  });

  test("builds safe localized filenames", () => {
    expect(canvasExportFileName("  Эхо Предела: Act I!  ", "svg")).toBe("эхо-предела-act-i.svg");
    expect(canvasExportFileName("!!!", "pdf")).toBe("canvas.pdf");
    expect(canvasExportFileName("Board", "canvas")).toBe("board.canvas");
  });
});
