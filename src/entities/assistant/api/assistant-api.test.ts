import { describe, expect, test } from "bun:test";

import { assistantEventsURL } from "./assistant-api";

describe("assistantEventsURL", () => {
  test("lets EventSource resume with Last-Event-ID by default", () => {
    expect(assistantEventsURL("run-1")).toBe("/api/assistant/runs/run-1/events");
    expect(assistantEventsURL("run-1", 0)).toBe("/api/assistant/runs/run-1/events");
  });

  test("supports an explicit positive durable cursor", () => {
    expect(assistantEventsURL("run-1", 42)).toBe("/api/assistant/runs/run-1/events?after=42");
  });
});
