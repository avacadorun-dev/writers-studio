import { describe, expect, test } from "bun:test";

import { groupLiveTranscriptSegments } from "./live-transcript-flow";

describe("live transcript flow", () => {
  test("coalesces provider finals into prose instead of one paragraph per frame", () => {
    const runs = groupLiveTranscriptSegments([
      { id: "one", text: "Первое предложение.", startMs: 0, endMs: 1_000 },
      { id: "two", text: "Второе предложение.", startMs: 1_000, endMs: 2_000 },
      {
        id: "break",
        text: "Команда: новый абзац",
        startMs: 2_000,
        endMs: 2_500,
        command: {
          kind: "new_paragraph",
          rawText: "Команда: новый абзац",
          executed: true,
        },
      },
      { id: "three", text: "Новый абзац.", startMs: 2_500, endMs: 3_500 },
    ]);

    expect(runs).toHaveLength(3);
    expect(runs[0]).toMatchObject({
      kind: "prose",
      segments: [{ id: "one" }, { id: "two" }],
    });
    expect(runs[1]).toMatchObject({ kind: "command", id: "break" });
    expect(runs[2]).toMatchObject({
      kind: "prose",
      segments: [{ id: "three" }],
    });
  });

  test("formats the utterance after a dialogue command with a Russian em dash", () => {
    const runs = groupLiveTranscriptSegments([
      {
        id: "dialogue",
        text: "Команда: начало диалога",
        startMs: 0,
        endMs: 500,
        command: {
          kind: "dialogue_start",
          rawText: "Команда: начало диалога",
          executed: true,
        },
      },
      {
        id: "line",
        text: "Дорогой мистер Беннет, сказала она.",
        startMs: 500,
        endMs: 2_000,
      },
    ]);

    expect(runs[1]).toMatchObject({
      kind: "prose",
      segments: [{ text: "— Дорогой мистер Беннет, сказала она." }],
    });
  });
});
