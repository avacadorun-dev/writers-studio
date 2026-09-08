import { describe, expect, test } from "bun:test";

import {
  assistantDockStateKey,
  loadAssistantDockState,
  saveAssistantDockState,
} from "./assistant-dock-state";

describe("shared Assistant dock state", () => {
  test("keeps the active run and view per project across surfaces", () => {
    saveAssistantDockState("project-a", { runId: "run-1", view: "session" });
    saveAssistantDockState("project-b", { view: "new" });
    expect(loadAssistantDockState("project-a")).toEqual({ runId: "run-1", view: "session" });
    expect(loadAssistantDockState("project-b")).toEqual({ view: "new" });
  });

  test("falls back safely for malformed or impossible session state", () => {
    localStorage.setItem(assistantDockStateKey("bad"), "not-json");
    localStorage.setItem(assistantDockStateKey("missing-run"), JSON.stringify({ view: "session" }));
    expect(loadAssistantDockState("bad")).toEqual({ view: "new" });
    expect(loadAssistantDockState("missing-run")).toEqual({ view: "threads" });
  });
});
