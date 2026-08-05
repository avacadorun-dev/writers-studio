import { beforeEach, describe, expect, test } from "bun:test";

import {
  clearAssistantGuideDraft,
  loadAssistantGuideDraft,
  saveAssistantGuideDraft,
} from "./assistant-guide-draft";

describe("assistant guide draft recovery", () => {
  beforeEach(() => localStorage.clear());

  test("keeps an unsaved guide draft isolated by project and base revision", () => {
    const saved = saveAssistantGuideDraft("project-a", 3, "# Голос\n\nКороткие фразы.");
    expect(loadAssistantGuideDraft("project-a")).toEqual(saved);
    expect(loadAssistantGuideDraft("project-b")).toBeUndefined();
  });

  test("rejects malformed or oversized local values", () => {
    localStorage.setItem(
      "writers-studio-assistant-guide-draft:project-a",
      JSON.stringify({
        projectId: "project-a",
        baseRevision: -1,
        content: "invalid",
        updatedAt: Date.now(),
      }),
    );
    expect(loadAssistantGuideDraft("project-a")).toBeUndefined();

    localStorage.setItem(
      "writers-studio-assistant-guide-draft:project-a",
      JSON.stringify({
        projectId: "project-a",
        baseRevision: 1,
        content: "я".repeat(24_001),
        updatedAt: Date.now(),
      }),
    );
    expect(loadAssistantGuideDraft("project-a")).toBeUndefined();
  });

  test("clears a draft after a durable save", () => {
    saveAssistantGuideDraft("project-a", 0, "Черновик");
    clearAssistantGuideDraft("project-a");
    expect(loadAssistantGuideDraft("project-a")).toBeUndefined();
  });
});
