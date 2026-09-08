import { describe, expect, test } from "bun:test";

import { mergeTranscriptAtEnd } from "./transcript-transfer";

describe("mergeTranscriptAtEnd", () => {
  test("preserves existing document blocks before transcript blocks", () => {
    const existing = {
      type: "doc",
      attrs: { schemaVersion: 1 },
      content: [{ type: "paragraph", content: [{ type: "text", text: "Начало сцены" }] }],
    };
    const transcript = {
      type: "doc",
      content: [{ type: "paragraph", content: [{ type: "text", text: "Новая реплика" }] }],
    };

    expect(mergeTranscriptAtEnd(existing, transcript)).toEqual({
      type: "doc",
      attrs: { schemaVersion: 1 },
      content: [
        { type: "paragraph", content: [{ type: "text", text: "Начало сцены" }] },
        { type: "paragraph", content: [{ type: "text", text: "Новая реплика" }] },
      ],
    });
  });

  test("supports an empty existing document", () => {
    const transcript = {
      type: "doc",
      content: [{ type: "paragraph", content: [{ type: "text", text: "Первая реплика" }] }],
    };

    expect(mergeTranscriptAtEnd({ type: "doc" }, transcript)).toEqual(transcript);
  });
});
