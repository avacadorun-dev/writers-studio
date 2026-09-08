import { describe, expect, test } from "bun:test";

import { resolveToolbarPresentation } from "./formatting-toolbar";

describe("formatting toolbar presentation", () => {
  test("uses available width in automatic mode", () => {
    expect(resolveToolbarPresentation("auto", 800, 1440)).toBe("expanded");
    expect(resolveToolbarPresentation("auto", 790, 1280)).toBe("expanded");
    expect(resolveToolbarPresentation("auto", 789, 1440)).toBe("compact");
    expect(resolveToolbarPresentation("auto", 800, 1024)).toBe("compact");
    expect(resolveToolbarPresentation("auto", 390, 390)).toBe("compact");
  });

  test("honours explicit expanded and compact preferences", () => {
    expect(resolveToolbarPresentation("expanded", 390)).toBe("expanded");
    expect(resolveToolbarPresentation("compact", 1440)).toBe("compact");
  });
});
