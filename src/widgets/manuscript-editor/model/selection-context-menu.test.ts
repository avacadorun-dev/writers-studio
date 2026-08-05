import { describe, expect, test } from "bun:test";

import { placeSelectionContextMenu } from "./selection-context-menu";

describe("selection context menu placement", () => {
  test("keeps an ordinary pointer position unchanged", () => {
    expect(placeSelectionContextMenu(320, 240, 1280, 720)).toEqual({ x: 320, y: 240 });
  });

  test("keeps the menu inside every viewport edge", () => {
    expect(placeSelectionContextMenu(-20, -10, 1280, 720)).toEqual({ x: 8, y: 8 });
    expect(placeSelectionContextMenu(1270, 710, 1280, 720)).toEqual({ x: 1004, y: 580 });
  });

  test("degrades safely in a viewport smaller than the menu", () => {
    expect(placeSelectionContextMenu(100, 100, 240, 120)).toEqual({ x: 8, y: 8 });
  });
});
