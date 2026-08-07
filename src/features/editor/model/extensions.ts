import { type Editor, Extension, mergeAttributes, Node as TiptapNode } from "@tiptap/core";
import Link from "@tiptap/extension-link";
import Placeholder from "@tiptap/extension-placeholder";
import { TableKit } from "@tiptap/extension-table";
import Underline from "@tiptap/extension-underline";
import type { Node as ProseMirrorNode } from "@tiptap/pm/model";
import { Plugin, PluginKey, type Transaction } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";
import StarterKit from "@tiptap/starter-kit";

import { createClientId } from "~shared/lib";
import { DocumentAnnotationsExtension } from "./annotations";
import { codeBlockExtensions } from "./code-block";
import { Callout, ImageBlock, MermaidBlock } from "./rich-blocks";
import { sanitizePastedHTML } from "./sanitize-pasted-html";

export const editorDocumentSchemaVersion = 3;

const identifiedBlockTypes = [
  "paragraph",
  "heading",
  "blockquote",
  "codeBlock",
  "callout",
  "table",
  "imageBlock",
  "mermaidBlock",
];

const richBlockTypes = new Set([
  "codeBlock",
  "callout",
  "table",
  "tableRow",
  "tableHeader",
  "tableCell",
  "imageBlock",
  "mermaidBlock",
]);

const activeSourcePluginKey = new PluginKey<DecorationSet>("writers-studio-active-source");

const ActiveSourceHighlight = Extension.create({
  name: "activeSourceHighlight",
  addProseMirrorPlugins() {
    return [
      new Plugin<DecorationSet>({
        key: activeSourcePluginKey,
        state: {
          init: () => DecorationSet.empty,
          apply: (transaction, current) => {
            const meta = transaction.getMeta(activeSourcePluginKey) as
              | { position: number }
              | null
              | undefined;
            if (meta === undefined) {
              return current.map(transaction.mapping, transaction.doc);
            }
            if (meta === null) {
              return DecorationSet.empty;
            }
            const node = transaction.doc.nodeAt(meta.position);
            if (!node) {
              return DecorationSet.empty;
            }
            return DecorationSet.create(transaction.doc, [
              Decoration.node(meta.position, meta.position + node.nodeSize, {
                "data-active-source": "true",
              }),
            ]);
          },
        },
        props: {
          decorations: (state) => activeSourcePluginKey.getState(state) ?? null,
        },
      }),
    ];
  },
});

export function highlightSourceNode(editor: Editor, element: HTMLElement): void {
  const contentPosition = editor.view.posAtDOM(element, 0);
  const resolved = editor.state.doc.resolve(contentPosition);
  const position = resolved.depth > 0 ? resolved.before(resolved.depth) : contentPosition;
  editor.view.dispatch(
    editor.state.tr.setMeta(activeSourcePluginKey, {
      position,
    }),
  );
}

const BlockIdentity = Extension.create({
  name: "blockIdentity",
  addGlobalAttributes() {
    return [
      {
        types: ["doc"],
        attributes: {
          schemaVersion: {
            default: 1,
            parseHTML: (element) => Number(element.getAttribute("data-schema-version") ?? 1),
            renderHTML: (attributes) => ({
              "data-schema-version": String(attributes.schemaVersion ?? 1),
            }),
          },
        },
      },
      {
        types: identifiedBlockTypes,
        attributes: {
          blockId: {
            default: null,
            keepOnSplit: false,
            parseHTML: (element) => element.getAttribute("data-block-id"),
            renderHTML: (attributes) =>
              attributes.blockId ? { "data-block-id": attributes.blockId as string } : {},
          },
        },
      },
      {
        types: ["paragraph", "heading", "blockquote"],
        attributes: {
          sourceRefs: {
            default: [],
            keepOnSplit: true,
            parseHTML: (element) => {
              const value = element.getAttribute("data-source-refs");
              return value ? value.split(",").filter(Boolean) : [];
            },
            renderHTML: (attributes) => {
              const refs = Array.isArray(attributes.sourceRefs)
                ? (attributes.sourceRefs as string[])
                : [];
              return refs.length > 0 ? { "data-source-refs": refs.join(",") } : {};
            },
          },
        },
      },
    ];
  },
  addProseMirrorPlugins() {
    return [
      new Plugin({
        key: new PluginKey("writers-studio-block-identity"),
        appendTransaction: (transactions, oldState, newState) => {
          if (!transactions.some((transaction) => transaction.docChanged)) {
            return null;
          }
          const mappedSourceRefs =
            countProvenanceBlocks(newState.doc) < countProvenanceBlocks(oldState.doc)
              ? mapPreviousSourceRefs(transactions, oldState.doc, newState.doc)
              : new Map<number, string[]>();
          const seen = new Set<string>();
          const transaction = newState.tr;
          let changed = false;
          const requiredSchemaVersion = documentRequiredSchemaVersion(newState.doc);
          if (Number(newState.doc.attrs.schemaVersion ?? 1) < requiredSchemaVersion) {
            transaction.setDocAttribute("schemaVersion", requiredSchemaVersion);
            changed = true;
          }
          newState.doc.descendants((node, position) => {
            if (!identifiedBlockTypes.includes(node.type.name)) {
              return;
            }
            const existing = typeof node.attrs.blockId === "string" ? node.attrs.blockId : "";
            const blockId = existing && !seen.has(existing) ? existing : createClientId();
            seen.add(blockId);
            const currentSourceRefs = sourceRefsFromAttrs(node.attrs.sourceRefs);
            const sourceRefs = Array.from(
              new Set([...currentSourceRefs, ...(mappedSourceRefs.get(position) ?? [])]),
            );
            if (
              blockId !== existing ||
              sourceRefs.length !== currentSourceRefs.length ||
              sourceRefs.some((reference, index) => reference !== currentSourceRefs[index])
            ) {
              transaction.setNodeMarkup(position, undefined, {
                ...node.attrs,
                blockId,
                sourceRefs,
              });
              changed = true;
            }
          });
          return changed ? transaction : null;
        },
      }),
    ];
  },
});

function documentRequiredSchemaVersion(document: ProseMirrorNode): number {
  let required = 1;
  document.descendants((node) => {
    if (node.type.name === "heading" && Number(node.attrs.level) >= 4) {
      required = editorDocumentSchemaVersion;
      return false;
    }
    if (
      required < 2 &&
      (richBlockTypes.has(node.type.name) || node.marks.some((mark) => mark.type.name === "code"))
    ) {
      required = 2;
    }
  });
  return required;
}

function mapPreviousSourceRefs(
  transactions: readonly Transaction[],
  oldDocument: ProseMirrorNode,
  newDocument: ProseMirrorNode,
): Map<number, string[]> {
  const result = new Map<number, string[]>();
  oldDocument.descendants((node, position) => {
    if (!["paragraph", "heading", "blockquote"].includes(node.type.name)) {
      return;
    }
    const references = sourceRefsFromAttrs(node.attrs.sourceRefs);
    if (references.length === 0) {
      return;
    }
    let mappedPosition = position + 1;
    let deletedAcross = false;
    for (const transaction of transactions) {
      const mapped = transaction.mapping.mapResult(mappedPosition, 1);
      mappedPosition = mapped.pos;
      deletedAcross ||= mapped.deletedAcross;
    }
    if (deletedAcross) {
      return;
    }
    newDocument.descendants((candidate, candidatePosition) => {
      if (
        !["paragraph", "heading", "blockquote"].includes(candidate.type.name) ||
        mappedPosition < candidatePosition + 1 ||
        mappedPosition > candidatePosition + candidate.nodeSize - 1
      ) {
        return;
      }
      result.set(candidatePosition, [...(result.get(candidatePosition) ?? []), ...references]);
      return false;
    });
  });
  return result;
}

function sourceRefsFromAttrs(value: unknown): string[] {
  return Array.isArray(value)
    ? value.filter((reference): reference is string => typeof reference === "string")
    : [];
}

function countProvenanceBlocks(document: ProseMirrorNode): number {
  let count = 0;
  document.descendants((node) => {
    if (["paragraph", "heading", "blockquote"].includes(node.type.name)) {
      count += 1;
    }
  });
  return count;
}

const PasteSanitizer = Extension.create({
  name: "writersStudioPasteSanitizer",
  transformPastedHTML(html) {
    return sanitizePastedHTML(html);
  },
});

const SceneBreak = TiptapNode.create({
  name: "sceneBreak",
  group: "block",
  atom: true,
  selectable: true,
  parseHTML() {
    return [{ tag: "div[data-scene-break]" }];
  },
  renderHTML({ HTMLAttributes }) {
    return [
      "div",
      mergeAttributes(HTMLAttributes, {
        "data-scene-break": "",
        class: "scene-break",
        role: "separator",
        "aria-label": "Разделитель сцены",
      }),
    ];
  },
});

export const editorExtensions = [
  StarterKit.configure({
    heading: { levels: [1, 2, 3, 4, 5, 6] },
    codeBlock: false,
    link: false,
    underline: false,
  }),
  ...codeBlockExtensions,
  TableKit.configure({
    table: {
      resizable: true,
      renderWrapper: true,
      allowTableNodeSelection: true,
    },
  }),
  Underline,
  Link.configure({
    openOnClick: false,
    autolink: true,
    HTMLAttributes: {
      rel: "noopener noreferrer",
    },
  }),
  Placeholder.configure({
    placeholder: "Начните писать или вставьте новый блок",
  }),
  PasteSanitizer,
  ActiveSourceHighlight,
  BlockIdentity,
  DocumentAnnotationsExtension,
  SceneBreak,
  Callout,
  ImageBlock,
  MermaidBlock,
];
