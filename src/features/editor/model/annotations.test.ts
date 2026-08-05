import { describe, expect, test } from "bun:test";
import { Editor } from "@tiptap/core";

import type { DocumentAnnotation } from "~entities/project";

import {
  annotationAnchorFromSelection,
  mappedAnnotationAnchors,
  revealDocumentAnnotation,
  setDocumentAnnotations,
} from "./annotations";
import { editorExtensions } from "./extensions";

function createEditor(text = "Лиса 🦊 вошла в тёмный лес", blockId = "block-1") {
  return new Editor({
    extensions: editorExtensions,
    content: {
      type: "doc",
      content: [
        {
          type: "paragraph",
          attrs: { blockId, sourceRefs: [] },
          content: [{ type: "text", text }],
        },
      ],
    },
  });
}

function note(overrides: Partial<DocumentAnnotation> = {}): DocumentAnnotation {
  return {
    id: "annotation-1",
    projectId: "project-1",
    nodeId: "node-1",
    nodeTitle: "Глава 1",
    kind: "note",
    status: "open",
    anchor: {
      blockId: "block-1",
      start: 7,
      end: 12,
      exactText: "вошла",
      prefix: "Лиса 🦊 ",
      suffix: " в тёмный лес",
    },
    note: "Уточнить движение",
    baseRevision: 1,
    createdAt: "2026-08-03T00:00:00Z",
    updatedAt: "2026-08-03T00:00:00Z",
    ...overrides,
  };
}

describe("document annotation anchors", () => {
  test("creates a Unicode code-point anchor from a same-block selection", () => {
    const editor = createEditor();
    const text = editor.getText();
    const start = text.indexOf("вошла");
    editor.commands.setTextSelection({ from: 1 + start, to: 1 + start + "вошла".length });

    expect(annotationAnchorFromSelection(editor)).toEqual(note().anchor);
    editor.destroy();
  });

  test("rejects empty and cross-block selections", () => {
    const editor = new Editor({
      extensions: editorExtensions,
      content: {
        type: "doc",
        content: [
          {
            type: "paragraph",
            attrs: { blockId: "first", sourceRefs: [] },
            content: [{ type: "text", text: "Первый" }],
          },
          {
            type: "paragraph",
            attrs: { blockId: "second", sourceRefs: [] },
            content: [{ type: "text", text: "Второй" }],
          },
        ],
      },
    });
    expect(annotationAnchorFromSelection(editor)).toBeUndefined();
    editor.commands.setTextSelection({ from: 2, to: editor.state.doc.content.size - 1 });
    expect(annotationAnchorFromSelection(editor)).toBeUndefined();
    editor.destroy();
  });

  test("renders transient note and dictionary decorations without changing JSON", () => {
    const editor = createEditor();
    const before = editor.getJSON();
    setDocumentAnnotations(editor, [
      note(),
      note({
        id: "annotation-2",
        kind: "dictionary_link",
        dictionaryTermId: "term-1",
        dictionaryTerm: {
          id: "term-1",
          canonical: "Тёмный лес",
          kind: "place",
          language: "ru",
          active: true,
        },
        anchor: {
          blockId: "block-1",
          start: 15,
          end: 25,
          exactText: "тёмный лес",
          prefix: "Лиса 🦊 вошла в ",
          suffix: "",
        },
      }),
    ]);

    expect(editor.view.dom.querySelectorAll(".ws-note-highlight")).toHaveLength(1);
    expect(editor.view.dom.querySelectorAll(".ws-dictionary-link")).toHaveLength(1);
    expect(editor.getJSON()).toEqual(before);
    editor.destroy();
  });

  test("maps an anchor when text is inserted before and inside its range", () => {
    const editor = createEditor();
    setDocumentAnnotations(editor, [note()]);
    editor.commands.insertContentAt(1, "Вдруг ");
    editor.commands.insertContentAt(1 + "Вдруг Лиса 🦊 во".length, "медленно ");

    expect(mappedAnnotationAnchors(editor)).toEqual([
      {
        id: "annotation-1",
        anchor: expect.objectContaining({
          blockId: "block-1",
          exactText: "вомедленно шла",
          start: 13,
          end: 27,
        }),
        detached: false,
      },
    ]);
    editor.destroy();
  });

  test("marks a deleted range detached and retains its last safe anchor", () => {
    const editor = createEditor();
    setDocumentAnnotations(editor, [note()]);
    const selectionStart = 1 + "Лиса 🦊 ".length;
    editor.commands.deleteRange({ from: selectionStart, to: selectionStart + "вошла".length });

    expect(mappedAnnotationAnchors(editor)).toEqual([
      { id: "annotation-1", anchor: note().anchor, detached: true },
    ]);
    editor.destroy();
  });

  test("does not decorate resolved or mismatched anchors", () => {
    const editor = createEditor();
    setDocumentAnnotations(editor, [
      note({ status: "resolved" }),
      note({ id: "bad", anchor: { ...note().anchor, exactText: "другое" } }),
    ]);
    expect(editor.view.dom.querySelectorAll("[data-annotation-id]")).toHaveLength(0);
    expect(mappedAnnotationAnchors(editor).find((item) => item.id === "bad")?.detached).toBeTrue();
    editor.destroy();
  });

  test("reveals the exact annotation and reports detached IDs", () => {
    const editor = createEditor();
    setDocumentAnnotations(editor, [note()]);
    expect(revealDocumentAnnotation(editor, "annotation-1")).toBeTrue();
    expect(editor.state.selection.from).toBe(9);
    expect(editor.state.selection.to).toBe(14);
    expect(revealDocumentAnnotation(editor, "missing")).toBeFalse();
    editor.destroy();
  });

  test("supports synthetic IDs for legacy blocks", () => {
    const editor = createEditor("Старый текст", "");
    editor.commands.setTextSelection({ from: 1, to: 7 });
    expect(annotationAnchorFromSelection(editor)).toEqual(
      expect.objectContaining({
        blockId: expect.stringMatching(/^.+$/),
        exactText: "Старый",
        start: 0,
        end: 6,
      }),
    );
    editor.destroy();
  });
});
