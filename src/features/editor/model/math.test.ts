import { describe, expect, test } from "bun:test";

import { maxMathSourceLength, parsePastedMath, renderMathSafely } from "./math";

describe("safe math", () => {
  test("renders valid inline and display LaTeX as static KaTeX output", () => {
    const inline = renderMathSafely("x^2 + y^2", false);
    const display = renderMathSafely("\\frac{a}{b}", true, "Дробь a на b");

    expect(inline.ok).toBeTrue();
    expect(inline.ok && inline.html).toContain("katex");
    expect(inline.accessibleText).toBe("Формула: x^2 + y^2");
    expect(display.ok).toBeTrue();
    expect(display.accessibleText).toBe("Дробь a на b");
  });

  test("falls back to the source for malformed, empty and oversized formulas", () => {
    expect(renderMathSafely("\\frac{", false).ok).toBeFalse();
    expect(renderMathSafely("", false).ok).toBeFalse();
    expect(renderMathSafely("x".repeat(maxMathSourceLength + 1), true).ok).toBeFalse();
  });

  test("rejects trusted HTML, external resource and user-defined commands", () => {
    for (const source of [
      "\\href{https://example.com}{click}",
      "\\includegraphics{https://example.com/a.png}",
      "\\htmlClass{x}{content}",
      "\\def\\x{boom}\\x",
      "\\newcommand{\\x}{boom}",
    ]) {
      const result = renderMathSafely(source, false);
      expect(result.ok).toBeFalse();
      expect(!result.ok && result.error).toContain("отключена");
    }
  });

  test("recognizes only complete inline and display paste syntax", () => {
    expect(parsePastedMath("$x + y$")).toEqual({ latex: "x + y", display: false });
    expect(parsePastedMath("$$\n\\sum_i x_i\n$$")).toEqual({
      latex: "\\sum_i x_i",
      display: true,
    });
    expect(parsePastedMath("text $x$ text")).toBeNull();
    expect(parsePastedMath("$unclosed")).toBeNull();
    expect(parsePastedMath("$$ $$")).toBeNull();
  });
});
