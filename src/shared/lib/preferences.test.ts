import { afterEach, describe, expect, test } from "bun:test";

import {
  applyWriterPreferences,
  defaultWriterPreferences,
  loadWriterPreferences,
  normalizeWriterPreferences,
  saveWriterPreferences,
  writerPreferencesChangedEvent,
  writerPreferencesStorageKey,
} from "./preferences";

afterEach(() => {
  localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
  document.documentElement.removeAttribute("data-style");
  document.documentElement.removeAttribute("style");
});

describe("writer preferences", () => {
  test("normalizes untrusted and legacy local values", () => {
    expect(
      normalizeWriterPreferences({
        language: "de",
        commandPrefix: "  Мастерская  ",
        commandsEnabled: false,
        fontFamily: "comic",
        fontSize: 200,
        lineHeight: 2,
        manuscriptWidth: 80,
        theme: "dark",
        style: "dried-rose-and-wine",
        spellcheck: false,
        wordGoal: 1234.6,
      }),
    ).toEqual({
      ...defaultWriterPreferences,
      commandPrefix: "Мастерская",
      commandsEnabled: false,
      lineHeight: 2,
      manuscriptWidth: 80,
      theme: "dark",
      style: "dried-rose-and-wine",
      spellcheck: false,
      wordGoal: 1235,
    });
  });

  test("falls back safely when stored JSON is invalid", () => {
    localStorage.setItem(writerPreferencesStorageKey, "{not json");
    expect(loadWriterPreferences()).toEqual(defaultWriterPreferences);
  });

  test("keeps every supported manuscript font and defaults unknown values to Sofia Sans", () => {
    expect(normalizeWriterPreferences({ fontFamily: "sofia" }).fontFamily).toBe("sofia");
    expect(normalizeWriterPreferences({ fontFamily: "inter" }).fontFamily).toBe("inter");
    expect(normalizeWriterPreferences({ fontFamily: "source-serif" }).fontFamily).toBe(
      "source-serif",
    );
    expect(normalizeWriterPreferences({ fontFamily: "sans" }).fontFamily).toBe("inter");
    expect(normalizeWriterPreferences({ fontFamily: "serif" }).fontFamily).toBe("source-serif");
    expect(normalizeWriterPreferences({ fontFamily: "literata" }).fontFamily).toBe("source-serif");
    expect(normalizeWriterPreferences({ fontFamily: "comic" }).fontFamily).toBe("sofia");
  });

  test("keeps all eight interface styles and migrates the two replaced themes", () => {
    for (const style of [
      "quiet",
      "cedar-and-copper",
      "dried-rose-and-wine",
      "olive-ink",
      "fog-graphite",
      "linen-clay",
      "dusty-lavender",
      "porcelain-ink",
    ] as const) {
      expect(normalizeWriterPreferences({ style }).style).toBe(style);
    }
    expect(normalizeWriterPreferences({ style: "pine-copper" }).style).toBe("cedar-and-copper");
    expect(normalizeWriterPreferences({ style: "moss-wine" }).style).toBe("dried-rose-and-wine");
    expect(normalizeWriterPreferences({ style: "neon" }).style).toBe("quiet");
    expect(normalizeWriterPreferences({}).style).toBe("quiet");
  });

  test("keeps only supported Assistant silence checkpoints", () => {
    for (const value of [0, 3000, 5000, 8000] as const) {
      expect(
        normalizeWriterPreferences({ assistantSilenceCheckpointMs: value })
          .assistantSilenceCheckpointMs,
      ).toBe(value);
    }
    expect(
      normalizeWriterPreferences({ assistantSilenceCheckpointMs: 1000 })
        .assistantSilenceCheckpointMs,
    ).toBe(5000);
  });

  test("saves, applies and announces preferences", () => {
    let announced = false;
    window.addEventListener(
      writerPreferencesChangedEvent,
      () => {
        announced = true;
      },
      { once: true },
    );
    const saved = saveWriterPreferences({
      ...defaultWriterPreferences,
      fontSize: 21,
      lineHeight: 2,
      manuscriptWidth: 60,
      theme: "dark",
      style: "dried-rose-and-wine",
    });

    expect(JSON.parse(localStorage.getItem(writerPreferencesStorageKey) ?? "{}")).toEqual(saved);
    expect(document.documentElement.dataset.theme).toBe("dark");
    expect(document.documentElement.dataset.style).toBe("dried-rose-and-wine");
    expect(document.documentElement.style.getPropertyValue("--editor-font-size")).toBe("21px");
    expect(document.documentElement.style.getPropertyValue("--editor-line-height")).toBe("2");
    expect(document.documentElement.style.getPropertyValue("--editor-max-width")).toBe("60ch");
    expect(document.documentElement.style.getPropertyValue("--editor-font-family")).toBe(
      "var(--font-manuscript-sofia)",
    );
    expect(document.documentElement.style.getPropertyValue("--editor-font-features")).toBe(
      '"locl" 0, "liga" 0',
    );
    expect(announced).toBe(true);
  });

  test("applies utilitarian font choices without Sofia Sans localized-form overrides", () => {
    applyWriterPreferences({ ...defaultWriterPreferences, fontFamily: "inter" });

    expect(document.documentElement.style.getPropertyValue("--editor-font-family")).toBe(
      "var(--font-manuscript-inter)",
    );
    expect(document.documentElement.style.getPropertyValue("--editor-font-features")).toBe(
      '"liga" 0',
    );
  });

  test("resolves the system theme through the media preference", () => {
    const originalMatchMedia = window.matchMedia;
    window.matchMedia = ((query: string) => ({
      matches: query === "(prefers-color-scheme: dark)",
      media: query,
    })) as typeof window.matchMedia;

    applyWriterPreferences({ ...defaultWriterPreferences, theme: "system" });
    expect(document.documentElement.dataset.theme).toBe("dark");

    window.matchMedia = originalMatchMedia;
  });

  test("keeps the browser theme color in sync with the active token", () => {
    const meta = document.createElement("meta");
    meta.name = "theme-color";
    document.head.append(meta);
    document.documentElement.style.setProperty("--surface-app", "token-color");

    applyWriterPreferences({ ...defaultWriterPreferences, theme: "light" });

    expect(meta.content).toBe("token-color");
    meta.remove();
  });
});
