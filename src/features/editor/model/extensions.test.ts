import { describe, expect, test } from "bun:test";
import { Editor } from "@tiptap/core";

import { editorExtensions, highlightSourceNode } from "./extensions";

describe("Writer's Studio Tiptap schema", () => {
  test("keeps legacy documents on schema v1 until a rich block is used", () => {
    const editor = new Editor({
      extensions: editorExtensions,
      content: {
        type: "doc",
        attrs: { schemaVersion: 1 },
        content: [{ type: "paragraph" }],
      },
    });

    editor.commands.insertContent("Обычная правка старого проекта");
    expect(editor.getJSON().attrs?.schemaVersion).toBe(1);

    editor.commands.insertTable({ rows: 2, cols: 2, withHeaderRow: true });
    expect(editor.getJSON().attrs?.schemaVersion).toBe(2);
    expect(editor.getJSON().content?.some((node) => node.type === "table")).toBeTrue();
    editor.destroy();
  });

  test("supports deep headings, code, callouts, images and Mermaid in schema v3", () => {
    const editor = new Editor({
      extensions: editorExtensions,
      content: { type: "doc", attrs: { schemaVersion: 1 }, content: [{ type: "paragraph" }] },
    });
    editor.commands.insertContent([
      {
        type: "heading",
        attrs: { level: 6 },
        content: [{ type: "text", text: "Глубокий раздел" }],
      },
      {
        type: "codeBlock",
        attrs: { language: "lua" },
        content: [{ type: "text", text: "return true" }],
      },
      {
        type: "callout",
        attrs: { kind: "idea" },
        content: [{ type: "paragraph", content: [{ type: "text", text: "Игровая идея" }] }],
      },
      {
        type: "imageBlock",
        attrs: {
          src: "/api/media/11111111-1111-1111-1111-111111111111/content",
          assetId: "11111111-1111-1111-1111-111111111111",
          alt: "Карта",
        },
      },
      { type: "mermaidBlock", attrs: { code: "flowchart LR\nA --> B" } },
    ]);

    const document = editor.getJSON();
    expect(document.attrs?.schemaVersion).toBe(3);
    const nodeTypes = document.content?.map((node) => node.type) ?? [];
    for (const type of ["heading", "codeBlock", "callout", "imageBlock", "mermaidBlock"]) {
      expect(nodeTypes).toContain(type);
    }
    editor.destroy();
  });

  test("keeps code ranges in preformatted mode without prose input rules", () => {
    const source = "1. not a list  \n\tindented\n\nfinal";
    const editor = new Editor({
      extensions: editorExtensions,
      content: {
        type: "doc",
        attrs: { schemaVersion: 2 },
        content: [
          {
            type: "codeBlock",
            attrs: { language: "go" },
            content: [{ type: "text", text: source }],
          },
        ],
      },
    });
    expect(editor.schema.nodes.codeBlock.spec.code).toBeTrue();
    expect(
      (editor.getJSON().content?.[0]?.content?.[0] as { text?: string } | undefined)?.text,
    ).toBe(source);
    expect(editor.getJSON().content?.some((node) => node.type.endsWith("List"))).toBeFalse();
    editor.destroy();
  });

  test("preserves provenance and assigns a new block identity when splitting", () => {
    const editor = new Editor({
      extensions: editorExtensions,
      content: {
        type: "doc",
        content: [
          {
            type: "paragraph",
            attrs: { blockId: "source-block", sourceRefs: ["segment-1"] },
            content: [{ type: "text", text: "Первый абзац" }],
          },
        ],
      },
    });

    editor.commands.setTextSelection(7);
    expect(editor.commands.splitBlock()).toBeTrue();

    const paragraphs = editor.getJSON().content ?? [];
    expect(paragraphs).toHaveLength(2);
    expect(paragraphs[0]?.attrs?.blockId).toBe("source-block");
    expect(paragraphs[1]?.attrs?.blockId).not.toBe("source-block");
    expect(paragraphs[1]?.attrs?.sourceRefs).toEqual(["segment-1"]);
    editor.destroy();
  });

  test("undo restores a manual editor transaction", () => {
    const editor = new Editor({
      extensions: editorExtensions,
      content: { type: "doc", content: [{ type: "paragraph" }] },
    });
    editor.commands.insertContent("Авторский текст");
    expect(editor.getText()).toBe("Авторский текст");
    expect(editor.commands.undo()).toBeTrue();
    expect(editor.getText()).toBe("");
    editor.destroy();
  });

  test("merges and deduplicates provenance when paragraphs are joined", () => {
    const editor = new Editor({
      extensions: editorExtensions,
      content: {
        type: "doc",
        content: [
          {
            type: "paragraph",
            attrs: { blockId: "first", sourceRefs: ["segment-1"] },
            content: [{ type: "text", text: "Первый" }],
          },
          {
            type: "paragraph",
            attrs: { blockId: "second", sourceRefs: ["segment-1", "segment-2"] },
            content: [{ type: "text", text: "Второй" }],
          },
        ],
      },
    });

    editor.commands.setTextSelection(9);
    expect(editor.commands.joinBackward()).toBeTrue();

    const joined = editor.getJSON().content ?? [];
    expect(joined).toHaveLength(1);
    expect(joined[0]?.attrs?.sourceRefs).toEqual(["segment-1", "segment-2"]);
    expect(editor.commands.undo()).toBeTrue();
    expect(editor.getJSON().content?.map((node) => node.attrs?.sourceRefs)).toEqual([
      ["segment-1"],
      ["segment-1", "segment-2"],
    ]);
    editor.destroy();
  });

  test("gives pasted internal blocks a new identity without losing provenance", () => {
    const editor = new Editor({
      extensions: editorExtensions,
      content: {
        type: "doc",
        content: [
          {
            type: "paragraph",
            attrs: { blockId: "source-block", sourceRefs: ["segment-1"] },
            content: [{ type: "text", text: "Исходный абзац" }],
          },
        ],
      },
    });

    editor.commands.insertContentAt(
      editor.state.doc.content.size,
      '<p data-block-id="source-block" data-source-refs="segment-1">Копия</p>',
    );

    const blocks = editor.getJSON().content ?? [];
    expect(blocks).toHaveLength(2);
    expect(blocks[1]?.attrs?.blockId).not.toBe("source-block");
    expect(blocks[1]?.attrs?.sourceRefs).toEqual(["segment-1"]);
    editor.destroy();
  });

  test("source highlighting is transient and does not change document JSON", () => {
    const editor = new Editor({
      extensions: editorExtensions,
      content: {
        type: "doc",
        content: [
          {
            type: "paragraph",
            attrs: { blockId: "source-block", sourceRefs: ["segment-1"] },
            content: [{ type: "text", text: "Привязанный абзац" }],
          },
        ],
      },
    });
    const before = editor.getJSON();
    const paragraph = editor.view.dom.querySelector<HTMLElement>("[data-source-refs]");
    expect(paragraph).not.toBeNull();

    highlightSourceNode(editor, paragraph as HTMLElement);

    expect(
      editor.view.dom
        .querySelector<HTMLElement>("[data-source-refs]")
        ?.getAttribute("data-active-source"),
    ).toBe("true");
    expect(editor.getJSON()).toEqual(before);
    editor.destroy();
  });

  test("keeps a 50,000-word editor node responsive", () => {
    const text = Array.from({ length: 50_000 }, (_, index) => `слово${index}`).join(" ");
    const loadStarted = performance.now();
    const editor = new Editor({
      extensions: editorExtensions,
      content: {
        type: "doc",
        content: [
          {
            type: "paragraph",
            attrs: { blockId: "large-block", sourceRefs: [] },
            content: [{ type: "text", text }],
          },
        ],
      },
    });
    const loadDuration = performance.now() - loadStarted;

    const editStarted = performance.now();
    editor.commands.setTextSelection(editor.state.doc.content.size - 1);
    editor.commands.insertContent(" финал");
    const editDuration = performance.now() - editStarted;

    expect(editor.getText().split(/\s+/u)).toHaveLength(50_001);
    expect(loadDuration).toBeLessThan(5_000);
    expect(editDuration).toBeLessThan(500);
    editor.destroy();
  });
});
