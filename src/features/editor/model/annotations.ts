import { type Editor, Extension } from "@tiptap/core";
import type { Node as ProseMirrorNode } from "@tiptap/pm/model";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";

import type {
  AnnotationAnchor,
  AnnotationAnchorUpdate,
  DocumentAnnotation,
} from "~entities/project";

interface MappedAnnotation {
  annotation: DocumentAnnotation;
  from: number;
  to: number;
  detached: boolean;
}

const annotationPluginKey = new PluginKey<MappedAnnotation[]>("writers-studio-annotations");

export const DocumentAnnotationsExtension = Extension.create({
  name: "writersStudioAnnotations",
  addProseMirrorPlugins() {
    return [
      new Plugin<MappedAnnotation[]>({
        key: annotationPluginKey,
        state: {
          init: () => [],
          apply: (transaction, current) => {
            const replacement = transaction.getMeta(annotationPluginKey) as
              | DocumentAnnotation[]
              | undefined;
            if (replacement) {
              return replacement.map((annotation) => {
                const range = locateAnchorRange(transaction.doc, annotation.anchor);
                return {
                  annotation,
                  from: range?.from ?? 0,
                  to: range?.to ?? 0,
                  detached: !range,
                };
              });
            }
            if (!transaction.docChanged) return current;
            return current.map((item) => {
              if (item.detached) return item;
              const start = transaction.mapping.mapResult(item.from, 1);
              const end = transaction.mapping.mapResult(item.to, -1);
              return {
                ...item,
                from: start.pos,
                to: end.pos,
                detached: start.deletedAcross || end.deletedAcross || end.pos <= start.pos,
              };
            });
          },
        },
        props: {
          decorations: (state) => {
            const items = annotationPluginKey.getState(state) ?? [];
            const decorations = items
              .filter(
                (item) =>
                  !item.detached &&
                  item.annotation.status !== "resolved" &&
                  item.from > 0 &&
                  item.to > item.from &&
                  item.to <= state.doc.content.size,
              )
              .map((item) =>
                Decoration.inline(item.from, item.to, {
                  class: annotationClass(item.annotation),
                  "data-annotation-id": item.annotation.id,
                  "data-annotation-kind": item.annotation.kind,
                  "aria-label": annotationLabel(item.annotation),
                }),
              );
            return DecorationSet.create(state.doc, decorations);
          },
        },
      }),
    ];
  },
});

export function setDocumentAnnotations(editor: Editor, annotations: DocumentAnnotation[]): void {
  editor.view.dispatch(editor.state.tr.setMeta(annotationPluginKey, annotations));
}

export function annotationAnchorFromSelection(editor: Editor): AnnotationAnchor | undefined {
  const { from, to, empty } = editor.state.selection;
  if (empty) return undefined;
  return anchorFromRange(editor.state.doc, from, to);
}

export function mappedAnnotationAnchors(editor: Editor): AnnotationAnchorUpdate[] {
  const items = annotationPluginKey.getState(editor.state) ?? [];
  return items.map((item) => {
    const anchor = item.detached
      ? undefined
      : anchorFromRange(editor.state.doc, item.from, item.to);
    return {
      id: item.annotation.id,
      anchor: anchor ?? item.annotation.anchor,
      detached: !anchor,
    };
  });
}

export function revealDocumentAnnotation(editor: Editor, annotationId: string): boolean {
  const item = (annotationPluginKey.getState(editor.state) ?? []).find(
    (candidate) => candidate.annotation.id === annotationId,
  );
  if (!item || item.detached || item.to <= item.from) return false;
  editor.commands.setTextSelection({ from: item.from, to: item.to });
  editor.commands.focus();
  const editorRoot = editor.view.dom;
  window.requestAnimationFrame(() => {
    const element = editorRoot.querySelector<HTMLElement>(
      `[data-annotation-id="${CSS.escape(annotationId)}"]`,
    );
    element?.scrollIntoView({ behavior: "smooth", block: "center" });
  });
  return true;
}

function locateAnchorRange(
  document: ProseMirrorNode,
  anchor: AnnotationAnchor,
): { from: number; to: number } | undefined {
  let result: { from: number; to: number } | undefined;
  document.forEach((block, offset, index) => {
    if (result) return;
    if (!supportsTextAnchor(block)) return;
    const blockId = effectiveBlockId(block, index);
    if (blockId !== anchor.blockId) return;
    const text = block.textBetween(0, block.content.size, "\n");
    const start = utf16OffsetAtCodePoint(text, anchor.start);
    const end = utf16OffsetAtCodePoint(text, anchor.end);
    if (start === undefined || end === undefined || end <= start) return;
    if (text.slice(start, end) !== anchor.exactText) return;
    result = { from: offset + 1 + start, to: offset + 1 + end };
  });
  return result;
}

function anchorFromRange(
  document: ProseMirrorNode,
  from: number,
  to: number,
): AnnotationAnchor | undefined {
  let result: AnnotationAnchor | undefined;
  document.forEach((block, offset, index) => {
    if (result) return;
    if (!supportsTextAnchor(block)) return;
    const contentStart = offset + 1;
    const contentEnd = contentStart + block.content.size;
    if (from < contentStart || to > contentEnd || to <= from) return;
    const localStart = from - contentStart;
    const localEnd = to - contentStart;
    const exactText = block.textBetween(localStart, localEnd, "\n");
    if (!exactText) return;
    const before = block.textBetween(0, localStart, "\n");
    const after = block.textBetween(localEnd, block.content.size, "\n");
    const start = Array.from(before).length;
    result = {
      blockId: effectiveBlockId(block, index),
      start,
      end: start + Array.from(exactText).length,
      exactText,
      prefix: Array.from(before).slice(-32).join(""),
      suffix: Array.from(after).slice(0, 32).join(""),
    };
  });
  return result;
}

function supportsTextAnchor(block: ProseMirrorNode): boolean {
  return ["paragraph", "heading", "blockquote", "codeBlock"].includes(block.type.name);
}

function effectiveBlockId(block: ProseMirrorNode, index: number): string {
  const value = block.attrs?.blockId;
  return typeof value === "string" && value.trim()
    ? value
    : `__writers_studio_legacy_block_${index}`;
}

function utf16OffsetAtCodePoint(text: string, offset: number): number | undefined {
  if (offset < 0) return undefined;
  const points = Array.from(text);
  if (offset > points.length) return undefined;
  return points.slice(0, offset).join("").length;
}

function annotationClass(annotation: DocumentAnnotation): string {
  const classes = ["ws-annotation"];
  classes.push(annotation.kind === "note" ? "ws-note-highlight" : "ws-dictionary-link");
  if (
    annotation.anchorStatus === "detached" ||
    annotation.status === "needs_attention" ||
    !annotation.dictionaryTerm?.id
  ) {
    classes.push("ws-annotation-needs-attention");
  }
  return classes.join(" ");
}

function annotationLabel(annotation: DocumentAnnotation): string {
  if (annotation.kind === "note") return `Заметка: ${annotation.note ?? ""}`;
  return annotation.dictionaryTerm?.id
    ? `Термин словаря: ${annotation.dictionaryTerm.canonical}`
    : `Удалённый термин: ${annotation.dictionaryTerm?.canonical ?? annotation.anchor.exactText}`;
}
