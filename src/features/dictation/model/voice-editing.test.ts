import { describe, expect, test } from "bun:test";

import type { CommandEvent } from "~entities/dictation";

import type { LiveFlowSegment } from "./live-transcript-flow";
import { resolveVoiceEditTarget } from "./voice-editing";

const segments: LiveFlowSegment[] = [
  {
    id: "one",
    text: "Дракон не чистил зубы по утрам, и зубы у него заболели.",
    startMs: 0,
    endMs: 4_000,
  },
  {
    id: "paragraph",
    text: "Команда, новый абзац",
    startMs: 4_000,
    endMs: 4_500,
    command: {
      kind: "new_paragraph",
      rawText: "Команда, новый абзац",
      executed: true,
    },
  },
  {
    id: "two",
    text: "И вот решил дракон полететь к стоматологу.",
    startMs: 4_500,
    endMs: 8_000,
  },
];

function command(value: Partial<CommandEvent>): CommandEvent {
  return {
    kind: "navigate_paragraph",
    rawText: "Команда",
    executed: true,
    ...value,
  };
}

describe("voice edit target resolution", () => {
  test("opens the requested paragraph with a caret at its end", () => {
    const target = resolveVoiceEditTarget(
      segments,
      command({ kind: "navigate_paragraph", ordinal: 1 }),
    );

    expect(target?.runId).toBe("one");
    expect(target?.segmentIds).toEqual(["one"]);
    expect(target?.selectionStart).toBe(target?.text.length);
    expect(target?.selectionEnd).toBe(target?.text.length);
  });

  test("selects the whole sentence containing a spoken phrase", () => {
    const target = resolveVoiceEditTarget(
      segments,
      command({
        kind: "navigate_sentence",
        target: "зубы у него заболели",
      }),
    );

    expect(target?.text.slice(target.selectionStart, target.selectionEnd)).toBe(
      "Дракон не чистил зубы по утрам, и зубы у него заболели.",
    );
  });

  test("selects only the requested words", () => {
    const target = resolveVoiceEditTarget(
      segments,
      command({
        kind: "navigate_text",
        target: "не чистил зубы",
      }),
    );

    expect(target?.text.slice(target.selectionStart, target.selectionEnd)).toBe("не чистил зубы");
  });

  test("resolves a sentence by its number across paragraphs", () => {
    const target = resolveVoiceEditTarget(
      segments,
      command({ kind: "navigate_sentence", ordinal: 2 }),
    );

    expect(target?.paragraphNumber).toBe(2);
    expect(target?.text.slice(target.selectionStart, target.selectionEnd)).toBe(
      "И вот решил дракон полететь к стоматологу.",
    );
  });

  test("does not guess when the spoken target is absent", () => {
    expect(
      resolveVoiceEditTarget(
        segments,
        command({ kind: "navigate_text", target: "несуществующие слова" }),
      ),
    ).toBeNull();
  });
});
