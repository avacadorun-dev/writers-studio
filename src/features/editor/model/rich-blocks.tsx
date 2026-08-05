import { mergeAttributes, Node, type NodeViewProps } from "@tiptap/core";
import { NodeViewWrapper, ReactNodeViewRenderer } from "@tiptap/react";
import { AlertTriangle, ImageIcon, Info, Lightbulb, Pencil, Trash2 } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";

import { Button } from "~shared/ui";

const calloutKinds = new Set(["info", "idea", "warning", "todo"]);

export const Callout = Node.create({
  name: "callout",
  group: "block",
  content: "block+",
  defining: true,
  addAttributes() {
    return {
      kind: {
        default: "info",
        parseHTML: (element) => {
          const value = element.getAttribute("data-callout-kind") ?? "info";
          return calloutKinds.has(value) ? value : "info";
        },
        renderHTML: (attributes) => ({
          "data-callout-kind": calloutKinds.has(String(attributes.kind))
            ? String(attributes.kind)
            : "info",
        }),
      },
    };
  },
  parseHTML() {
    return [{ tag: "aside[data-callout-kind]" }];
  },
  renderHTML({ HTMLAttributes }) {
    return ["aside", mergeAttributes(HTMLAttributes, { "data-callout": "" }), 0];
  },
});

export const ImageBlock = Node.create({
  name: "imageBlock",
  group: "block",
  atom: true,
  draggable: true,
  selectable: true,
  addAttributes() {
    return {
      src: { default: "" },
      assetId: { default: null },
      alt: { default: "" },
      caption: { default: "" },
      width: { default: null },
      height: { default: null },
    };
  },
  parseHTML() {
    return [
      {
        tag: "figure[data-image-block]",
        getAttrs: (element) => {
          if (!(element instanceof HTMLElement)) return false;
          const image = element.querySelector("img");
          if (!image) return false;
          return {
            src: image.getAttribute("src") ?? "",
            assetId: element.getAttribute("data-asset-id"),
            alt: image.getAttribute("alt") ?? "",
            caption: element.querySelector("figcaption")?.textContent ?? "",
            width: numericAttribute(image.getAttribute("width")),
            height: numericAttribute(image.getAttribute("height")),
          };
        },
      },
    ];
  },
  renderHTML({ node, HTMLAttributes }) {
    const imageAttributes: Record<string, string | number> = {
      src: String(node.attrs.src ?? ""),
      alt: String(node.attrs.alt ?? ""),
      loading: "lazy",
      decoding: "async",
    };
    if (typeof node.attrs.width === "number") imageAttributes.width = node.attrs.width;
    if (typeof node.attrs.height === "number") imageAttributes.height = node.attrs.height;
    const caption = String(node.attrs.caption ?? "");
    return [
      "figure",
      mergeAttributes(HTMLAttributes, {
        "data-image-block": "",
        "data-asset-id": node.attrs.assetId ?? undefined,
      }),
      ["img", imageAttributes],
      ["figcaption", {}, caption],
    ];
  },
  addNodeView() {
    return ReactNodeViewRenderer(ImageBlockView);
  },
});

function ImageBlockView({ node, selected, updateAttributes, deleteNode }: NodeViewProps) {
  const [caption, setCaption] = useState(String(node.attrs.caption ?? ""));
  const [alt, setAlt] = useState(String(node.attrs.alt ?? ""));

  useEffect(() => setCaption(String(node.attrs.caption ?? "")), [node.attrs.caption]);
  useEffect(() => setAlt(String(node.attrs.alt ?? "")), [node.attrs.alt]);

  return (
    <NodeViewWrapper
      as="figure"
      data-image-block=""
      data-selected={selected ? "true" : "false"}
      className="ws-image-block"
    >
      {node.attrs.src ? (
        <img
          src={String(node.attrs.src)}
          alt={String(node.attrs.alt ?? "")}
          width={typeof node.attrs.width === "number" ? node.attrs.width : undefined}
          height={typeof node.attrs.height === "number" ? node.attrs.height : undefined}
          loading="lazy"
          decoding="async"
          draggable={false}
        />
      ) : (
        <div className="ws-image-placeholder">
          <ImageIcon className="size-5" aria-hidden="true" />
          Изображение недоступно
        </div>
      )}
      <input
        type="text"
        value={caption}
        className="ws-image-caption"
        placeholder="Подпись к изображению"
        aria-label="Подпись к изображению"
        onChange={(event) => setCaption(event.target.value)}
        onBlur={() => updateAttributes({ caption: caption.trim() })}
      />
      {selected ? (
        <div className="ws-rich-block-controls" contentEditable={false}>
          <label className="ws-rich-block-field">
            <span>Alt-текст</span>
            <input
              type="text"
              value={alt}
              onChange={(event) => setAlt(event.target.value)}
              onBlur={() => updateAttributes({ alt: alt.trim() })}
            />
          </label>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-8 min-h-8"
            aria-label="Удалить изображение из документа"
            onClick={deleteNode}
          >
            <Trash2 className="size-4" aria-hidden="true" />
          </Button>
        </div>
      ) : null}
    </NodeViewWrapper>
  );
}

export const MermaidBlock = Node.create({
  name: "mermaidBlock",
  group: "block",
  atom: true,
  draggable: true,
  selectable: true,
  addAttributes() {
    return {
      code: { default: "graph TD\n  A[Начало] --> B[Результат]" },
      caption: { default: "" },
    };
  },
  parseHTML() {
    return [
      {
        tag: "figure[data-mermaid-block]",
        getAttrs: (element) => {
          if (!(element instanceof HTMLElement)) return false;
          return {
            code: element.querySelector("pre[data-mermaid-source]")?.textContent ?? "",
            caption: element.querySelector("figcaption")?.textContent ?? "",
          };
        },
      },
    ];
  },
  renderHTML({ node, HTMLAttributes }) {
    return [
      "figure",
      mergeAttributes(HTMLAttributes, { "data-mermaid-block": "" }),
      ["pre", { "data-mermaid-source": "" }, String(node.attrs.code ?? "")],
      ["figcaption", {}, String(node.attrs.caption ?? "")],
    ];
  },
  addNodeView() {
    return ReactNodeViewRenderer(MermaidBlockView);
  },
});

function MermaidBlockView({ node, selected, updateAttributes, deleteNode }: NodeViewProps) {
  const reactId = useId().replaceAll(":", "");
  const previewRef = useRef<HTMLDivElement | null>(null);
  const [source, setSource] = useState(String(node.attrs.code ?? ""));
  const [caption, setCaption] = useState(String(node.attrs.caption ?? ""));
  const [error, setError] = useState("");

  useEffect(() => setSource(String(node.attrs.code ?? "")), [node.attrs.code]);
  useEffect(() => setCaption(String(node.attrs.caption ?? "")), [node.attrs.caption]);

  useEffect(() => {
    let active = true;
    const timeout = window.setTimeout(() => {
      void import("mermaid")
        .then(async ({ default: mermaid }) => {
          mermaid.initialize({
            startOnLoad: false,
            securityLevel: "strict",
            theme: document.documentElement.dataset.theme === "dark" ? "dark" : "neutral",
            fontFamily: "inherit",
          });
          try {
            const rendered = await mermaid.render(`ws-mermaid-${reactId}`, source);
            if (!active || !previewRef.current) return;
            previewRef.current.innerHTML = rendered.svg;
            rendered.bindFunctions?.(previewRef.current);
            setError("");
          } catch {
            if (!active || !previewRef.current) return;
            previewRef.current.replaceChildren();
            setError("Не удалось построить схему. Проверьте синтаксис Mermaid.");
          }
        })
        .catch(() => {
          if (!active || !previewRef.current) return;
          previewRef.current.replaceChildren();
          setError("Модуль схем временно недоступен. Обновите страницу и повторите попытку.");
        });
    }, 180);
    return () => {
      active = false;
      window.clearTimeout(timeout);
    };
  }, [reactId, source]);

  return (
    <NodeViewWrapper
      as="figure"
      data-mermaid-block=""
      data-selected={selected ? "true" : "false"}
      className="ws-mermaid-block"
    >
      <div className="ws-rich-block-heading" contentEditable={false}>
        <span>Mermaid-схема</span>
        {selected ? (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-8 min-h-8"
            aria-label="Удалить схему"
            onClick={deleteNode}
          >
            <Trash2 className="size-4" aria-hidden="true" />
          </Button>
        ) : null}
      </div>
      <div ref={previewRef} className="ws-mermaid-preview" contentEditable={false} />
      {error ? (
        <p className="ws-mermaid-error" contentEditable={false}>
          <AlertTriangle className="size-4" aria-hidden="true" />
          {error}
        </p>
      ) : null}
      <textarea
        value={source}
        className="ws-mermaid-source"
        aria-label="Исходный код Mermaid-схемы"
        spellCheck={false}
        onChange={(event) => setSource(event.target.value)}
        onBlur={() => updateAttributes({ code: source.trim() })}
      />
      <input
        type="text"
        value={caption}
        className="ws-image-caption"
        placeholder="Подпись к схеме"
        aria-label="Подпись к схеме"
        onChange={(event) => setCaption(event.target.value)}
        onBlur={() => updateAttributes({ caption: caption.trim() })}
      />
    </NodeViewWrapper>
  );
}

export const calloutPresentation = {
  info: { label: "Информация", icon: Info },
  idea: { label: "Идея", icon: Lightbulb },
  warning: { label: "Предупреждение", icon: AlertTriangle },
  todo: { label: "TODO", icon: Pencil },
} as const;

function numericAttribute(value: string | null): number | null {
  if (!value) return null;
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : null;
}
