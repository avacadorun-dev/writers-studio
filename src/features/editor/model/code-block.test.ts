import { describe, expect, test } from "bun:test";

import { migrateFencedCodeBlocks, migrateInlineCodeMarks, parseFencedCode } from "./code-block";
import {
  codeHighlightLanguage,
  codeLanguageOptions,
  normalizeCodeLanguage,
} from "./code-languages";

describe("native code blocks", () => {
  test("parses complete Go fences without normalizing whitespace", () => {
    const text = 'package main  \r\n\r\nfunc main() {\r\n\tprintln("hello")\r\n}\r\n';
    expect(parseFencedCode(`\`\`\`\`go\r\n${text}\r\n\`\`\`\`\r\n`)).toEqual({
      language: "go",
      text,
    });
  });

  test("supports optional languages, tildes and longer closing fences", () => {
    expect(parseFencedCode("~~~\n\tplain  \n~~~~")).toEqual({
      language: "",
      text: "\tplain  ",
    });
    expect(parseFencedCode("```html\n<div />\n````")).toEqual({
      language: "html",
      text: "<div />",
    });
  });

  test("rejects partial text and malformed fences", () => {
    for (const value of [
      "```go\nmissing close",
      "``go\ncode\n``",
      "```bad/language\ncode\n```",
      "```go\ncode\n~~~",
      "before\n```go\ncode\n```",
    ]) {
      expect(parseFencedCode(value)).toBeNull();
    }
  });

  test("migrates only whole fenced paragraphs and retains their block identity", () => {
    const original = {
      type: "doc",
      attrs: { schemaVersion: 1 },
      content: [
        {
          type: "paragraph",
          attrs: { blockId: "legacy", sourceRefs: ["assistant"] },
          content: [{ type: "text", text: "```go\npackage main\n```" }],
        },
        { type: "paragraph", content: [{ type: "text", text: "ordinary" }] },
      ],
    };
    expect(migrateFencedCodeBlocks(original) as unknown).toEqual({
      type: "doc",
      attrs: { schemaVersion: 2 },
      content: [
        {
          type: "codeBlock",
          attrs: { blockId: "legacy", language: "go" },
          content: [{ type: "text", text: "package main" }],
        },
        { type: "paragraph", content: [{ type: "text", text: "ordinary" }] },
      ],
    });
    const plain = { type: "doc", content: [{ type: "paragraph", text: "ordinary" }] };
    expect(migrateFencedCodeBlocks(plain)).toBe(plain);
  });

  test("migrates inline backticks to code marks without changing their whitespace", () => {
    const original = {
      type: "doc",
      attrs: { schemaVersion: 1 },
      content: [
        {
          type: "paragraph",
          content: [
            {
              type: "text",
              text: "Команды: `go run`, ``fmt `raw`  `` и `go test`.",
              marks: [{ type: "bold" }],
            },
          ],
        },
        {
          type: "callout",
          attrs: { kind: "info" },
          content: [
            {
              type: "paragraph",
              content: [{ type: "text", text: "Путь `a  b\t c`" }],
            },
          ],
        },
        { type: "codeBlock", content: [{ type: "text", text: "`println`" }] },
        {
          type: "paragraph",
          content: [
            { type: "text", text: "`already marked`", marks: [{ type: "code" }] },
            { type: "text", text: " and `unclosed" },
          ],
        },
      ],
    };
    const migrated = migrateInlineCodeMarks(original);
    expect(migrated).not.toBe(original);
    expect(migrated.attrs.schemaVersion).toBe(2);
    expect(migrated.content[0].content).toEqual([
      { type: "text", text: "Команды: ", marks: [{ type: "bold" }] },
      { type: "text", text: "go run", marks: [{ type: "bold" }, { type: "code" }] },
      { type: "text", text: ", ", marks: [{ type: "bold" }] },
      {
        type: "text",
        text: "fmt `raw`  ",
        marks: [{ type: "bold" }, { type: "code" }],
      },
      { type: "text", text: " и ", marks: [{ type: "bold" }] },
      { type: "text", text: "go test", marks: [{ type: "bold" }, { type: "code" }] },
      { type: "text", text: ".", marks: [{ type: "bold" }] },
    ]);
    const callout = migrated.content[1] as {
      content?: Array<{
        content?: Array<{ type: string; text: string; marks?: Array<{ type: string }> }>;
      }>;
    };
    expect(callout.content?.[0].content?.[1]).toEqual({
      type: "text",
      text: "a  b\t c",
      marks: [{ type: "code" }],
    });
    expect(migrated.content[2]).toEqual(original.content[2]);
    expect(migrated.content[3]).toEqual(original.content[3]);
    expect(migrateInlineCodeMarks(migrated)).toBe(migrated);
  });

  test("keeps documents without complete inline delimiters unchanged", () => {
    const plain = {
      type: "doc",
      content: [{ type: "paragraph", content: [{ type: "text", text: "before `missing" }] }],
    };
    expect(migrateInlineCodeMarks(plain)).toBe(plain);
  });

  test("offers a broad language registry including Go without rewriting fence aliases", () => {
    expect(codeLanguageOptions.length).toBeGreaterThanOrEqual(75);
    expect(codeLanguageOptions).toContainEqual({ value: "go", label: "Go / Golang" });
    expect(new Set(codeLanguageOptions.map((option) => option.value)).size).toBe(
      codeLanguageOptions.length,
    );
    expect(normalizeCodeLanguage(" HTML ")).toBe("html");
    expect(codeHighlightLanguage("html")).toBe("markup");
    expect(codeHighlightLanguage("golang")).toBe("go");
    expect(codeHighlightLanguage("dockerfile")).toBe("docker");
    expect(codeHighlightLanguage("terraform")).toBe("hcl");
    expect(codeHighlightLanguage("plantuml")).toBe("plant-uml");
    expect(normalizeCodeLanguage(null)).toBe("");
  });
});
