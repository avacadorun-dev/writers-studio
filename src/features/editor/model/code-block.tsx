import { Extension, type NodeViewProps } from "@tiptap/core";
import TiptapCodeBlock from "@tiptap/extension-code-block";
import type { Node as ProseMirrorNode } from "@tiptap/pm/model";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";
import { NodeViewContent, NodeViewWrapper, ReactNodeViewRenderer } from "@tiptap/react";
import { Check, Copy } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { refractor } from "refractor";
import clojure from "refractor/clojure";
import cmake from "refractor/cmake";
import cobol from "refractor/cobol";
import dart from "refractor/dart";
import docker from "refractor/docker";
import elixir from "refractor/elixir";
import erlang from "refractor/erlang";
import fortran from "refractor/fortran";
import fsharp from "refractor/fsharp";
import gdscript from "refractor/gdscript";
import git from "refractor/git";
import glsl from "refractor/glsl";
import gradle from "refractor/gradle";
import graphql from "refractor/graphql";
import groovy from "refractor/groovy";
import haskell from "refractor/haskell";
import hcl from "refractor/hcl";
import http from "refractor/http";
import jq from "refractor/jq";
import json5 from "refractor/json5";
import jsx from "refractor/jsx";
import julia from "refractor/julia";
import latex from "refractor/latex";
import lisp from "refractor/lisp";
import matlab from "refractor/matlab";
import mermaid from "refractor/mermaid";
import nasm from "refractor/nasm";
import nginx from "refractor/nginx";
import nix from "refractor/nix";
import ocaml from "refractor/ocaml";
import pascal from "refractor/pascal";
import plantUML from "refractor/plant-uml";
import powershell from "refractor/powershell";
import promql from "refractor/promql";
import properties from "refractor/properties";
import protobuf from "refractor/protobuf";
import scala from "refractor/scala";
import scheme from "refractor/scheme";
import solidity from "refractor/solidity";
import toml from "refractor/toml";
import tsx from "refractor/tsx";

import { Button } from "~shared/ui";
import {
  codeHighlightLanguage,
  codeLanguageOptions,
  normalizeCodeLanguage,
} from "./code-languages";

if (!refractor.registered("powershell")) refractor.register(powershell);
for (const syntax of [
  clojure,
  cmake,
  cobol,
  dart,
  docker,
  elixir,
  erlang,
  fortran,
  fsharp,
  gdscript,
  git,
  glsl,
  gradle,
  graphql,
  groovy,
  haskell,
  hcl,
  http,
  jq,
  json5,
  jsx,
  julia,
  latex,
  lisp,
  matlab,
  mermaid,
  nasm,
  nginx,
  nix,
  ocaml,
  pascal,
  plantUML,
  promql,
  properties,
  protobuf,
  scala,
  scheme,
  solidity,
  toml,
  tsx,
]) {
  refractor.register(syntax);
}

interface HighlightNode {
  type: "root" | "element" | "text";
  value?: string;
  properties?: { className?: string[] | string };
  children?: HighlightNode[];
}

export interface ParsedFencedCode {
  language: string;
  text: string;
}

interface JSONDocumentNode {
  type: string;
  attrs?: Record<string, unknown>;
  marks?: JSONDocumentMark[];
  text?: string;
  content?: JSONDocumentNode[];
}

interface JSONDocumentMark {
  type: string;
  attrs?: Record<string, unknown>;
}

export interface InlineCodePair {
  openingStart: number;
  openingEnd: number;
  closingStart: number;
  closingEnd: number;
}

interface BacktickRun {
  start: number;
  end: number;
}

export function parseFencedCode(value: string): ParsedFencedCode | null {
  const firstLineEnd = value.indexOf("\n");
  if (firstLineEnd < 0) return null;
  const openingLine = value.slice(0, firstLineEnd).replace(/\r$/u, "");
  const opening = /^(`{3,}|~{3,})([^\r\n]*)$/u.exec(openingLine);
  if (!opening) return null;
  const fence = opening[1];
  const info = opening[2].trim();
  const language = info ? info.split(/\s+/u)[0] : "";
  if (language && !/^[a-zA-Z0-9_+#.-]{1,40}$/u.test(language)) return null;

  const remainder = value.slice(firstLineEnd + 1).replace(/\r?\n$/u, "");
  const lastLineEnd = remainder.lastIndexOf("\n");
  let text = "";
  let closingLine = remainder;
  if (lastLineEnd >= 0) {
    const separatorStart =
      lastLineEnd > 0 && remainder[lastLineEnd - 1] === "\r" ? lastLineEnd - 1 : lastLineEnd;
    text = remainder.slice(0, separatorStart);
    closingLine = remainder.slice(lastLineEnd + 1);
  }
  const closingIndent = /^[ \t]*/u.exec(closingLine)?.[0] ?? "";
  if (closingIndent.length > 3) return null;
  const closing = closingLine.slice(closingIndent.length).trimEnd();
  const closingFence = new RegExp(`^${escapeRegExp(fence[0])}{${fence.length},}$`, "u");
  if (!closingFence.test(closing)) return null;
  return { language: normalizeCodeLanguage(language), text };
}

export function migrateFencedCodeBlocks<T extends JSONDocumentNode>(document: T): T {
  if (document.type !== "doc" || !Array.isArray(document.content)) return document;
  let changed = false;
  const content = document.content.map((block) => {
    if (block.type !== "paragraph") return block;
    const parsed = parseFencedCode(jsonNodeText(block));
    if (!parsed) return block;
    changed = true;
    const attrs: Record<string, unknown> = {};
    if (typeof block.attrs?.blockId === "string") attrs.blockId = block.attrs.blockId;
    if (parsed.language) attrs.language = parsed.language;
    return {
      type: "codeBlock",
      attrs,
      ...(parsed.text ? { content: [{ type: "text", text: parsed.text }] } : {}),
    };
  });
  if (!changed) return document;
  return {
    ...document,
    attrs: {
      ...document.attrs,
      schemaVersion: Math.max(2, Number(document.attrs?.schemaVersion ?? 1)),
    },
    content,
  };
}

export function migrateInlineCodeMarks<T extends JSONDocumentNode>(document: T): T {
  if (document.type !== "doc") return document;
  const [migrated, changed] = migrateInlineCodeNode(document, false);
  if (!changed) return document;
  return {
    ...migrated,
    attrs: {
      ...migrated.attrs,
      schemaVersion: Math.max(2, Number(migrated.attrs?.schemaVersion ?? 1)),
    },
  } as T;
}

function migrateInlineCodeNode<T extends JSONDocumentNode>(
  node: T,
  insideCodeBlock: boolean,
): [T, boolean] {
  if (insideCodeBlock || node.type === "codeBlock" || !Array.isArray(node.content)) {
    return [node, false];
  }
  let changed = false;
  const content: JSONDocumentNode[] = [];
  for (const child of node.content) {
    if (child.type === "text" && typeof child.text === "string") {
      const parsed = parseInlineCodeText(child);
      if (parsed) {
        content.push(...parsed);
        changed = true;
        continue;
      }
    }
    const [migratedChild, childChanged] = migrateInlineCodeNode(child, false);
    content.push(migratedChild);
    changed ||= childChanged;
  }
  return changed ? [{ ...node, content } as T, true] : [node, false];
}

function parseInlineCodeText(node: JSONDocumentNode): JSONDocumentNode[] | null {
  const value = node.text ?? "";
  const marks = node.marks ?? [];
  if (!value || marks.some((mark) => mark.type === "code")) return null;
  const result: JSONDocumentNode[] = [];
  let cursor = 0;
  let search = 0;
  let changed = false;
  while (search < value.length) {
    const opening = value.indexOf("`", search);
    if (opening < 0) break;
    const openingLength = backtickRunLength(value, opening);
    const closing = matchingBacktickRun(value, opening + openingLength, openingLength);
    if (closing < 0) {
      search = opening + openingLength;
      continue;
    }
    if (opening > cursor) {
      result.push(textPart(node, value.slice(cursor, opening), marks));
    }
    result.push(
      textPart(node, value.slice(opening + openingLength, closing), [...marks, { type: "code" }]),
    );
    cursor = closing + openingLength;
    search = cursor;
    changed = true;
  }
  if (!changed) return null;
  if (cursor < value.length) result.push(textPart(node, value.slice(cursor), marks));
  return result;
}

function textPart(
  node: JSONDocumentNode,
  text: string,
  marks: JSONDocumentMark[],
): JSONDocumentNode {
  const result: JSONDocumentNode = { ...node, text };
  if (marks.length > 0) result.marks = marks;
  else delete result.marks;
  return result;
}

function backtickRunLength(value: string, start: number): number {
  let end = start;
  while (end < value.length && value[end] === "`") end += 1;
  return end - start;
}

function matchingBacktickRun(value: string, start: number, expectedLength: number): number {
  while (start < value.length) {
    const candidate = value.indexOf("`", start);
    if (candidate < 0) return -1;
    const length = backtickRunLength(value, candidate);
    if (length === expectedLength) return candidate;
    start = candidate + length;
  }
  return -1;
}

export function findInlineCodePairs(value: string): InlineCodePair[] {
  const pairs: InlineCodePair[] = [];
  let search = 0;
  while (search < value.length) {
    const opening = nextUnescapedBacktickRun(value, search);
    if (!opening) break;
    if (opening.end >= value.length || /\s/u.test(value[opening.end])) {
      search = opening.end;
      continue;
    }
    let closingSearch = opening.end;
    let closing: BacktickRun | null = null;
    while (closingSearch < value.length) {
      const candidate = nextUnescapedBacktickRun(value, closingSearch);
      if (!candidate) break;
      if (candidate.start > 0 && !/\s/u.test(value[candidate.start - 1])) {
        closing = candidate;
        break;
      }
      closingSearch = candidate.end;
    }
    if (!closing) break;
    if (closing.end - closing.start !== opening.end - opening.start) {
      search = closing.end;
      continue;
    }
    if (closing.start > opening.end) {
      pairs.push({
        openingStart: opening.start,
        openingEnd: opening.end,
        closingStart: closing.start,
        closingEnd: closing.end,
      });
    }
    search = closing.end;
  }
  return pairs;
}

function nextUnescapedBacktickRun(value: string, start: number): BacktickRun | null {
  let cursor = start;
  while (cursor < value.length) {
    const candidate = value.indexOf("`", cursor);
    if (candidate < 0) return null;
    const length = backtickRunLength(value, candidate);
    if (!isEscapedDelimiter(value, candidate)) {
      return { start: candidate, end: candidate + length };
    }
    cursor = candidate + length;
  }
  return null;
}

function isEscapedDelimiter(value: string, position: number): boolean {
  let backslashes = 0;
  for (let cursor = position - 1; cursor >= 0 && value[cursor] === "\\"; cursor -= 1) {
    backslashes += 1;
  }
  return backslashes % 2 === 1;
}

function jsonNodeText(node: JSONDocumentNode): string {
  if (node.type === "text") return node.text ?? "";
  if (node.type === "hardBreak") return "\n";
  return (node.content ?? []).map(jsonNodeText).join("");
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/gu, "\\$&");
}

const CodeBlockWithControls = TiptapCodeBlock.extend({
  addNodeView() {
    return ReactNodeViewRenderer(CodeBlockView);
  },
});

function CodeBlockView({ node, updateAttributes }: NodeViewProps) {
  const [copied, setCopied] = useState(false);
  const language = normalizeCodeLanguage(node.attrs.language);
  const options = useMemo(() => {
    if (!language || codeLanguageOptions.some((option) => option.value === language)) {
      return codeLanguageOptions;
    }
    return [
      codeLanguageOptions[0],
      { value: language, label: language },
      ...codeLanguageOptions.slice(1),
    ];
  }, [language]);

  useEffect(() => {
    if (!copied) return;
    const timeout = window.setTimeout(() => setCopied(false), 1600);
    return () => window.clearTimeout(timeout);
  }, [copied]);

  const copyCode = async () => {
    try {
      await navigator.clipboard.writeText(node.textContent);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  };

  return (
    <NodeViewWrapper className="ws-code-block" data-language={language || undefined}>
      <div className="ws-code-block-toolbar" contentEditable={false}>
        <label>
          <span className="sr-only">Язык блока кода</span>
          <select
            value={language}
            aria-label="Язык блока кода"
            onChange={(event) => updateAttributes({ language: event.target.value || null })}
          >
            {options.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
        <Button
          type="button"
          variant="ghost"
          size="compact"
          className="min-h-8 gap-1.5 px-2 text-xs"
          aria-label={copied ? "Код скопирован" : "Скопировать код"}
          onClick={() => void copyCode()}
        >
          {copied ? (
            <Check className="size-3.5" aria-hidden="true" />
          ) : (
            <Copy className="size-3.5" aria-hidden="true" />
          )}
          {copied ? "Скопировано" : "Копировать"}
        </Button>
      </div>
      <pre spellCheck={false}>
        <NodeViewContent as={"code" as "div"} />
      </pre>
    </NodeViewWrapper>
  );
}

const syntaxHighlightPluginKey = new PluginKey<DecorationSet>("writers-studio-code-highlight");
const inlineCodePluginKey = new PluginKey("writers-studio-inline-code-normalizer");

function syntaxDecorations(document: ProseMirrorNode): DecorationSet {
  const decorations: Decoration[] = [];
  document.descendants((node, position) => {
    if (node.type.name !== "codeBlock") return;
    const language = codeHighlightLanguage(node.attrs.language);
    if (!language || !refractor.registered(language)) return;
    let highlighted: HighlightNode;
    try {
      highlighted = refractor.highlight(node.textContent, language) as HighlightNode;
    } catch {
      return;
    }
    collectHighlightDecorations(highlighted.children ?? [], position + 1, [], decorations);
  });
  return DecorationSet.create(document, decorations);
}

function collectHighlightDecorations(
  nodes: HighlightNode[],
  start: number,
  inheritedClasses: string[],
  decorations: Decoration[],
): number {
  let cursor = start;
  for (const node of nodes) {
    if (node.type === "text") {
      const end = cursor + (node.value ?? "").length;
      if (end > cursor && inheritedClasses.length > 0) {
        decorations.push(Decoration.inline(cursor, end, { class: inheritedClasses.join(" ") }));
      }
      cursor = end;
      continue;
    }
    const ownClasses = node.properties?.className;
    const classes = [
      ...inheritedClasses,
      ...(Array.isArray(ownClasses) ? ownClasses : ownClasses ? [ownClasses] : []),
    ];
    cursor = collectHighlightDecorations(node.children ?? [], cursor, classes, decorations);
  }
  return cursor;
}

const CodeBlockEnhancements = Extension.create({
  name: "writersStudioCodeBlockEnhancements",
  addProseMirrorPlugins() {
    return [
      new Plugin<DecorationSet>({
        key: syntaxHighlightPluginKey,
        state: {
          init: (_configuration, state) => syntaxDecorations(state.doc),
          apply: (transaction, current) =>
            transaction.docChanged
              ? syntaxDecorations(transaction.doc)
              : current.map(transaction.mapping, transaction.doc),
        },
        props: {
          decorations: (state) => syntaxHighlightPluginKey.getState(state) ?? null,
          handlePaste: (view, event) => {
            const parsed = parseFencedCode(event.clipboardData?.getData("text/plain") ?? "");
            if (!parsed) return false;
            const codeBlock = view.state.schema.nodes.codeBlock;
            if (!codeBlock) return false;
            const content = parsed.text ? view.state.schema.text(parsed.text) : undefined;
            const node = codeBlock.create({ language: parsed.language || null }, content);
            view.dispatch(view.state.tr.replaceSelectionWith(node).scrollIntoView());
            return true;
          },
        },
      }),
      new Plugin({
        key: inlineCodePluginKey,
        appendTransaction: (transactions, _oldState, newState) => {
          if (
            !transactions.some((transaction) => transaction.docChanged) ||
            transactions.some((transaction) => transaction.getMeta(inlineCodePluginKey))
          ) {
            return null;
          }
          const codeMark = newState.schema.marks.code;
          if (!codeMark) return null;
          const matches: Array<InlineCodePair & { contentStart: number }> = [];
          newState.doc.descendants((node, position) => {
            if (!node.isTextblock || node.type.name === "codeBlock") return;
            const children = node.content.content;
            if (children.some((child) => !child.isText)) return;
            const text = children.map((child) => child.text ?? "").join("");
            if (!text.includes("`")) return;
            const codeRanges: Array<{ from: number; to: number }> = [];
            let offset = 0;
            for (const child of children) {
              const end = offset + child.nodeSize;
              if (child.marks.some((mark) => mark.type === codeMark)) {
                codeRanges.push({ from: offset, to: end });
              }
              offset = end;
            }
            for (const pair of findInlineCodePairs(text)) {
              if (
                codeRanges.some(
                  (range) => range.from < pair.closingEnd && range.to > pair.openingStart,
                )
              ) {
                continue;
              }
              matches.push({ ...pair, contentStart: position + 1 });
            }
          });
          if (matches.length === 0) return null;
          const transaction = newState.tr.setMeta(inlineCodePluginKey, true);
          for (const match of matches.reverse()) {
            const openingStart = match.contentStart + match.openingStart;
            const openingEnd = match.contentStart + match.openingEnd;
            const closingStart = match.contentStart + match.closingStart;
            const closingEnd = match.contentStart + match.closingEnd;
            transaction.addMark(openingEnd, closingStart, codeMark.create());
            transaction.delete(closingStart, closingEnd);
            transaction.delete(openingStart, openingEnd);
          }
          return transaction;
        },
      }),
    ];
  },
});

export const codeBlockExtensions = [CodeBlockWithControls, CodeBlockEnhancements];
