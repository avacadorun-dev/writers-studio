import { describe, expect, test } from "bun:test";

import {
  assistantTranscriptDistanceFromEnd,
  assistantTranscriptScrollStorageKey,
  captureAssistantTranscriptScroll,
  isAssistantTranscriptNearEnd,
  normalizeAssistantTranscriptScrollSnapshot,
  restoredAssistantTranscriptScrollTop,
} from "./assistant-transcript-scroll";

describe("assistant transcript scroll", () => {
  test("keeps following activity while the author is at the end", () => {
    expect(
      isAssistantTranscriptNearEnd({ scrollHeight: 1_000, scrollTop: 304, clientHeight: 600 }),
    ).toBe(true);
    expect(
      isAssistantTranscriptNearEnd({ scrollHeight: 1_000, scrollTop: 303, clientHeight: 600 }),
    ).toBe(false);
  });

  test("normalizes short content and accepts a custom threshold", () => {
    const short = { scrollHeight: 400, scrollTop: 0, clientHeight: 600 };
    expect(assistantTranscriptDistanceFromEnd(short)).toBe(0);
    expect(isAssistantTranscriptNearEnd(short)).toBe(true);
    expect(
      isAssistantTranscriptNearEnd({ scrollHeight: 1_000, scrollTop: 350, clientHeight: 600 }, 40),
    ).toBe(false);
  });

  test("stores independent run positions and restores a bounded scroll offset", () => {
    expect(assistantTranscriptScrollStorageKey("run-a")).not.toBe(
      assistantTranscriptScrollStorageKey("run-b"),
    );
    const snapshot = captureAssistantTranscriptScroll({
      scrollHeight: 2_000,
      scrollTop: 320,
      clientHeight: 600,
    });
    expect(snapshot).toEqual({ scrollTop: 320, followsActivity: false });
    expect(
      restoredAssistantTranscriptScrollTop({ scrollHeight: 900, clientHeight: 600 }, snapshot),
    ).toBe(300);
    expect(
      restoredAssistantTranscriptScrollTop(
        { scrollHeight: 900, clientHeight: 600 },
        { scrollTop: 0, followsActivity: true },
      ),
    ).toBe(300);
  });

  test("rejects malformed stored positions", () => {
    expect(normalizeAssistantTranscriptScrollSnapshot(null)).toBeNull();
    expect(normalizeAssistantTranscriptScrollSnapshot({ scrollTop: "4" })).toBeNull();
    expect(
      normalizeAssistantTranscriptScrollSnapshot({ scrollTop: -20, followsActivity: false }),
    ).toEqual({ scrollTop: 0, followsActivity: false });
  });
});
