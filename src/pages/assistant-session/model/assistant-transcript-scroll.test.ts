import { describe, expect, test } from "bun:test";

import {
  assistantTranscriptDistanceFromEnd,
  isAssistantTranscriptNearEnd,
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
});
