// biome-ignore-all lint/security/noDangerouslySetInnerHtml: KaTeX runs with trust=false after an explicit unsafe-command rejection and emits static MathML/HTML.
import { Extension, mergeAttributes, Node, type NodeViewProps } from "@tiptap/core";
import type { Node as ProseMirrorNode } from "@tiptap/pm/model";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { NodeViewWrapper, ReactNodeViewRenderer } from "@tiptap/react";
import katex from "katex";
import { AlertTriangle, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { Button } from "~shared/ui";

export const maxMathSourceLength = 10_000;

const unsafeMathCommand =
  /\\(?:href|url|includegraphics|html(?:Class|Id|Style|Data)|def|edef|gdef|xdef|let|futurelet|newcommand|renewcommand|providecommand|input|include|openin|openout|read|write|usepackage|documentclass|csname)\b/i;

export interface MathAttributes {
  latex: string;
  description?: string;
}

export type SafeMathResult =
  | { ok: true; html: string; accessibleText: string }
  | { ok: false; error: string; accessibleText: string };

export function renderMathSafely(
  latex: string,
  displayMode: boolean,
  description = "",
): SafeMathResult {
  const source = latex.trim();
  const accessibleText = description.trim() || `Формула: ${source || "не задана"}`;
  if (!source) {
    return { ok: false, error: "Введите исходник формулы.", accessibleText };
  }
  if (source.length > maxMathSourceLength) {
    return { ok: false, error: "Формула слишком длинная.", accessibleText };
  }
  if (unsafeMathCommand.test(source)) {
    return {
      ok: false,
      error: "Эта команда отключена для безопасного отображения формул.",
      accessibleText,
    };
  }
  try {
    return {
      ok: true,
      html: katex.renderToString(source, {
        displayMode,
        output: "htmlAndMathml",
        throwOnError: true,
        strict: "error",
        trust: false,
        maxExpand: 1_000,
      }),
      accessibleText,
    };
  } catch {
    return {
      ok: false,
      error: "Не удалось отобразить формулу. Проверьте синтаксис LaTeX.",
      accessibleText,
    };
  }
}

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    math: {
      insertInlineMath: (attributes: MathAttributes) => ReturnType;
      insertMathBlock: (attributes: MathAttributes) => ReturnType;
    };
  }
}

export const InlineMath = Node.create({
  name: "inlineMath",
  group: "inline",
  inline: true,
  atom: true,
  selectable: true,
  addAttributes() {
    return mathAttributes();
  },
  parseHTML() {
    return [{ tag: "span[data-inline-math]" }];
  },
  renderHTML({ node, HTMLAttributes }) {
    return [
      "span",
      mergeAttributes(HTMLAttributes, {
        "data-inline-math": "",
        "data-latex": String(node.attrs.latex ?? ""),
        "data-description": String(node.attrs.description ?? ""),
        role: "math",
        "aria-label": mathAccessibleText(node),
      }),
      String(node.attrs.latex ?? ""),
    ];
  },
  addNodeView() {
    return ReactNodeViewRenderer(InlineMathView);
  },
});

export const MathBlock = Node.create({
  name: "mathBlock",
  group: "block",
  atom: true,
  draggable: true,
  selectable: true,
  addAttributes() {
    return mathAttributes();
  },
  parseHTML() {
    return [{ tag: "figure[data-math-block]" }];
  },
  renderHTML({ node, HTMLAttributes }) {
    return [
      "figure",
      mergeAttributes(HTMLAttributes, {
        "data-math-block": "",
        "data-latex": String(node.attrs.latex ?? ""),
        "data-description": String(node.attrs.description ?? ""),
        role: "math",
        "aria-label": mathAccessibleText(node),
      }),
      ["pre", { "data-math-source": "" }, String(node.attrs.latex ?? "")],
    ];
  },
  addNodeView() {
    return ReactNodeViewRenderer(MathBlockView);
  },
});

export const MathCommands = Extension.create({
  name: "writersStudioMathCommands",
  addCommands() {
    return {
      insertInlineMath:
        (attributes) =>
        ({ commands }) =>
          commands.insertContent({
            type: "inlineMath",
            attrs: normalizedMathAttributes(attributes),
          }),
      insertMathBlock:
        (attributes) =>
        ({ commands }) =>
          commands.insertContent({
            type: "mathBlock",
            attrs: normalizedMathAttributes(attributes),
          }),
    };
  },
});

export const MathPaste = Extension.create({
  name: "writersStudioMathPaste",
  addProseMirrorPlugins() {
    return [
      new Plugin({
        key: new PluginKey("writers-studio-math-paste"),
        props: {
          handlePaste(view, event) {
            const plainText = event.clipboardData?.getData("text/plain") ?? "";
            const match = parsePastedMath(plainText);
            if (!match) return false;
            const nodeType = view.state.schema.nodes[match.display ? "mathBlock" : "inlineMath"];
            if (!nodeType) return false;
            const node = nodeType.create({ latex: match.latex, description: "" });
            view.dispatch(view.state.tr.replaceSelectionWith(node).scrollIntoView());
            return true;
          },
        },
      }),
    ];
  },
});

export function parsePastedMath(value: string): { latex: string; display: boolean } | null {
  const source = value.trim();
  const display = source.match(/^\$\$([\s\S]+)\$\$$/);
  if (display?.[1]?.trim()) return { latex: display[1].trim(), display: true };
  const inline = source.match(/^\$([^$\r\n]+)\$$/);
  if (inline?.[1]?.trim()) return { latex: inline[1].trim(), display: false };
  return null;
}

function mathAttributes() {
  return {
    latex: {
      default: "",
      parseHTML: (element: HTMLElement) =>
        element.getAttribute("data-latex") ??
        element.querySelector("[data-math-source]")?.textContent ??
        element.textContent ??
        "",
    },
    description: {
      default: "",
      parseHTML: (element: HTMLElement) => element.getAttribute("data-description") ?? "",
    },
  };
}

function normalizedMathAttributes(attributes: MathAttributes): MathAttributes {
  return {
    latex: attributes.latex.trim().slice(0, maxMathSourceLength),
    description: attributes.description?.trim().slice(0, 1_000) ?? "",
  };
}

function mathAccessibleText(node: ProseMirrorNode): string {
  const description = String(node.attrs.description ?? "").trim();
  return description || `Формула: ${String(node.attrs.latex ?? "")}`;
}

function InlineMathView({ node, selected, updateAttributes }: NodeViewProps) {
  const [source, setSource] = useState(String(node.attrs.latex ?? ""));
  const [description, setDescription] = useState(String(node.attrs.description ?? ""));
  const rendered = useMemo(
    () => renderMathSafely(source, false, description),
    [description, source],
  );

  useEffect(() => setSource(String(node.attrs.latex ?? "")), [node.attrs.latex]);
  useEffect(() => setDescription(String(node.attrs.description ?? "")), [node.attrs.description]);

  return (
    <NodeViewWrapper
      as="span"
      className="ws-inline-math"
      data-inline-math=""
      data-selected={selected ? "true" : "false"}
      role="math"
      aria-label={rendered.accessibleText}
    >
      <MathPreview result={rendered} source={source} />
      {selected ? (
        <span className="ws-inline-math-editor" contentEditable={false}>
          <label>
            <span>LaTeX</span>
            <input
              value={source}
              aria-label="Исходник строчной формулы"
              spellCheck={false}
              onChange={(event) => setSource(event.target.value)}
              onBlur={() =>
                updateAttributes(normalizedMathAttributes({ latex: source, description }))
              }
            />
          </label>
          <label>
            <span>Описание</span>
            <input
              value={description}
              aria-label="Описание строчной формулы"
              onChange={(event) => setDescription(event.target.value)}
              onBlur={() =>
                updateAttributes(normalizedMathAttributes({ latex: source, description }))
              }
            />
          </label>
        </span>
      ) : null}
    </NodeViewWrapper>
  );
}

function MathBlockView({ node, selected, updateAttributes, deleteNode }: NodeViewProps) {
  const [source, setSource] = useState(String(node.attrs.latex ?? ""));
  const [description, setDescription] = useState(String(node.attrs.description ?? ""));
  const rendered = useMemo(
    () => renderMathSafely(source, true, description),
    [description, source],
  );

  useEffect(() => setSource(String(node.attrs.latex ?? "")), [node.attrs.latex]);
  useEffect(() => setDescription(String(node.attrs.description ?? "")), [node.attrs.description]);

  const commit = () => updateAttributes(normalizedMathAttributes({ latex: source, description }));

  return (
    <NodeViewWrapper
      as="figure"
      className="ws-math-block"
      data-math-block=""
      data-selected={selected ? "true" : "false"}
      role="math"
      aria-label={rendered.accessibleText}
    >
      <div className="ws-rich-block-heading" contentEditable={false}>
        <span>Формула</span>
        {selected ? (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-8 min-h-8"
            aria-label="Удалить формулу"
            onClick={deleteNode}
          >
            <Trash2 className="size-4" aria-hidden="true" />
          </Button>
        ) : null}
      </div>
      <MathPreview result={rendered} source={source} />
      <label className="ws-rich-block-field">
        <span>Исходник LaTeX</span>
        <textarea
          value={source}
          aria-label="Исходник блочной формулы"
          spellCheck={false}
          rows={3}
          onChange={(event) => setSource(event.target.value)}
          onBlur={commit}
        />
      </label>
      <label className="ws-rich-block-field">
        <span>Описание для скринридера</span>
        <input
          value={description}
          aria-label="Описание блочной формулы"
          placeholder="Необязательно — по умолчанию читается исходник"
          onChange={(event) => setDescription(event.target.value)}
          onBlur={commit}
        />
      </label>
    </NodeViewWrapper>
  );
}

function MathPreview({ result, source }: { result: SafeMathResult; source: string }) {
  if (result.ok) {
    return (
      <span
        className="ws-math-preview"
        contentEditable={false}
        aria-hidden="true"
        dangerouslySetInnerHTML={{ __html: result.html }}
      />
    );
  }
  return (
    <span className="ws-math-error" contentEditable={false} role="alert">
      <AlertTriangle className="size-4" aria-hidden="true" />
      <span>
        {result.error} <code>{source || "∅"}</code>
      </span>
    </span>
  );
}
