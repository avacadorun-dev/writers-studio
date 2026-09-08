import { describe, expect, test } from "bun:test";

import { maximumNodeTitleLength, normalizeNodeTitle } from "./node-title";

describe("node title editing", () => {
  test("trims a changed valid title", () => {
    expect(normalizeNodeTitle("  Новая глава  ", "Старая глава")).toBe("Новая глава");
  });

  test("does not submit empty, unchanged or oversized values", () => {
    expect(normalizeNodeTitle("   ", "Глава")).toBeNull();
    expect(normalizeNodeTitle(" Глава ", "Глава")).toBeNull();
    expect(normalizeNodeTitle("я".repeat(maximumNodeTitleLength + 1), "Глава")).toBeNull();
  });

  test("counts Unicode code points instead of UTF-16 units", () => {
    expect(normalizeNodeTitle("📖".repeat(maximumNodeTitleLength), "Глава")).not.toBeNull();
  });
});
