import { describe, expect, test } from "bun:test";

import {
  canvasFocusRemainingSeconds,
  canvasFocusTimerEnd,
  formatCanvasFocusTime,
} from "./focus-timer";

describe("canvas focus timer", () => {
  test("calculates a bounded local countdown", () => {
    expect(canvasFocusTimerEnd(1_000, 5)).toBe(301_000);
    expect(canvasFocusTimerEnd(1_000, 0)).toBe(1_000);
    expect(canvasFocusRemainingSeconds(3_001, 1_001)).toBe(2);
    expect(canvasFocusRemainingSeconds(1_000, 3_000)).toBe(0);
    expect(canvasFocusRemainingSeconds(undefined, 0)).toBe(0);
  });

  test("formats long and invalid values without negative output", () => {
    expect(formatCanvasFocusTime(65)).toBe("01:05");
    expect(formatCanvasFocusTime(3_661)).toBe("61:01");
    expect(formatCanvasFocusTime(-3)).toBe("00:00");
    expect(formatCanvasFocusTime(Number.NaN)).toBe("00:00");
  });
});
