const allowedElements = new Set([
  "A",
  "BLOCKQUOTE",
  "BR",
  "CODE",
  "EM",
  "FIGCAPTION",
  "FIGURE",
  "H1",
  "H2",
  "H3",
  "HR",
  "IMG",
  "LI",
  "OL",
  "P",
  "PRE",
  "S",
  "STRONG",
  "TABLE",
  "TBODY",
  "TD",
  "TH",
  "THEAD",
  "TR",
  "U",
  "UL",
  "ASIDE",
]);

const removedElements = new Set([
  "BUTTON",
  "EMBED",
  "FORM",
  "IFRAME",
  "INPUT",
  "LINK",
  "MATH",
  "META",
  "OBJECT",
  "SCRIPT",
  "STYLE",
  "SVG",
  "TEXTAREA",
]);

const blockElements = new Set([
  "ASIDE",
  "BLOCKQUOTE",
  "DIV",
  "FIGURE",
  "H1",
  "H2",
  "H3",
  "LI",
  "OL",
  "P",
  "PRE",
  "TABLE",
  "UL",
]);

export function sanitizePastedHTML(html: string): string {
  const parsed = new DOMParser().parseFromString(html, "text/html");
  sanitizeChildren(parsed.body, parsed);
  return parsed.body.innerHTML;
}

function sanitizeChildren(parent: ParentNode, document: Document): void {
  for (const node of Array.from(parent.childNodes)) {
    if (node.nodeType === Node.COMMENT_NODE) {
      node.remove();
      continue;
    }
    if (!(node instanceof Element)) {
      continue;
    }
    if (removedElements.has(node.tagName)) {
      node.remove();
      continue;
    }

    sanitizeChildren(node, document);
    const semanticMarks = marksFromStyle(node.getAttribute("style") ?? "");
    const tagName = normalizeTagName(node);
    if (tagName === "SPAN") {
      replaceWithChildren(node, document, semanticMarks);
      continue;
    }
    if (!allowedElements.has(tagName)) {
      if (tagName === "DIV" && !hasDirectBlockChild(node)) {
        replaceElement(node, document, "P", semanticMarks);
      } else {
        replaceWithChildren(node, document, semanticMarks);
      }
      continue;
    }

    if (tagName === "IMG" && !safeImageSource(node.getAttribute("src") ?? "")) {
      node.remove();
      continue;
    }

    removeUnsupportedAttributes(node);
    applyMarksToChildren(node, document, semanticMarks);
  }
}

function normalizeTagName(element: Element): string {
  switch (element.tagName) {
    case "B":
      return "STRONG";
    case "I":
      return "EM";
    case "STRIKE":
      return "S";
    default:
      return element.tagName;
  }
}

function replaceElement(
  element: Element,
  document: Document,
  tagName: string,
  marks: string[],
): void {
  const replacement = document.createElement(tagName);
  while (element.firstChild) {
    replacement.append(element.firstChild);
  }
  applyMarksToChildren(replacement, document, marks);
  element.replaceWith(replacement);
}

function replaceWithChildren(element: Element, document: Document, marks: string[]): void {
  const fragment = document.createDocumentFragment();
  while (element.firstChild) {
    fragment.append(element.firstChild);
  }
  const replacement = wrapFragment(fragment, document, marks);
  element.replaceWith(replacement);
}

function applyMarksToChildren(element: Element, document: Document, marks: string[]): void {
  if (marks.length === 0 || element.childNodes.length === 0) {
    return;
  }
  const fragment = document.createDocumentFragment();
  while (element.firstChild) {
    fragment.append(element.firstChild);
  }
  element.append(wrapFragment(fragment, document, marks));
}

function wrapFragment(
  fragment: DocumentFragment,
  document: Document,
  marks: string[],
): DocumentFragment | HTMLElement {
  let wrapped: DocumentFragment | HTMLElement = fragment;
  for (const tagName of marks) {
    const mark = document.createElement(tagName);
    mark.append(wrapped);
    wrapped = mark;
  }
  return wrapped;
}

function marksFromStyle(style: string): string[] {
  const normalized = style.toLocaleLowerCase();
  const marks: string[] = [];
  if (/font-weight\s*:\s*(?:bold|[6-9]00)/u.test(normalized)) {
    marks.push("strong");
  }
  if (/font-style\s*:\s*italic/u.test(normalized)) {
    marks.push("em");
  }
  if (/text-decoration(?:-line)?\s*:[^;]*underline/u.test(normalized)) {
    marks.push("u");
  }
  if (/text-decoration(?:-line)?\s*:[^;]*line-through/u.test(normalized)) {
    marks.push("s");
  }
  return marks;
}

function removeUnsupportedAttributes(element: Element): void {
  const blockWithProvenance = [
    "ASIDE",
    "BLOCKQUOTE",
    "FIGURE",
    "H1",
    "H2",
    "H3",
    "P",
    "PRE",
    "TABLE",
  ].includes(element.tagName);
  for (const attribute of Array.from(element.attributes)) {
    const name = attribute.name.toLocaleLowerCase();
    const keepHref = element.tagName === "A" && name === "href" && safeHref(attribute.value);
    const keepImage =
      element.tagName === "IMG" &&
      ((name === "src" && safeImageSource(attribute.value)) ||
        (name === "alt" && attribute.value.length <= 1_000) ||
        ((name === "width" || name === "height") && safeDimension(attribute.value)));
    const keepFigure =
      element.tagName === "FIGURE" &&
      ((name === "data-image-block" && attribute.value === "") ||
        (name === "data-mermaid-block" && attribute.value === "") ||
        (name === "data-asset-id" && safeIdentifier(attribute.value)));
    const keepCallout =
      element.tagName === "ASIDE" &&
      name === "data-callout-kind" &&
      ["info", "idea", "warning", "todo"].includes(attribute.value);
    const keepMermaidSource =
      element.tagName === "PRE" && name === "data-mermaid-source" && attribute.value === "";
    const keepCodeLanguage =
      element.tagName === "CODE" &&
      name === "class" &&
      /^language-[a-z0-9_+#.-]{1,40}$/iu.test(attribute.value);
    const keepTableCell =
      (element.tagName === "TD" || element.tagName === "TH") &&
      ((name === "colspan" && safeSpan(attribute.value)) ||
        (name === "rowspan" && safeSpan(attribute.value)) ||
        (name === "data-colwidth" && safeColumnWidths(attribute.value)));
    const keepProvenance =
      blockWithProvenance &&
      (name === "data-block-id" || name === "data-source-refs") &&
      safeProvenance(attribute.value);
    if (
      !keepHref &&
      !keepImage &&
      !keepFigure &&
      !keepCallout &&
      !keepMermaidSource &&
      !keepCodeLanguage &&
      !keepTableCell &&
      !keepProvenance
    ) {
      element.removeAttribute(attribute.name);
    }
  }
  if (element.tagName === "A" && !element.hasAttribute("href")) {
    element.replaceWith(...Array.from(element.childNodes));
  }
}

function safeHref(value: string): boolean {
  const trimmed = value.trim();
  const normalized = trimmed.toLocaleLowerCase();
  return (
    normalized.startsWith("https://") ||
    normalized.startsWith("http://") ||
    normalized.startsWith("mailto:") ||
    /^\/projects\/[0-9a-f-]{36}\/studio\/[0-9a-f-]{36}(?:#[^\s]*)?$/iu.test(trimmed)
  );
}

function safeProvenance(value: string): boolean {
  return value.length <= 2_048 && /^[a-z0-9,_-]+$/iu.test(value);
}

function safeIdentifier(value: string): boolean {
  return /^[a-z0-9_-]{1,160}$/iu.test(value);
}

function safeImageSource(value: string): boolean {
  const normalized = value.trim();
  return (
    /^https:\/\/[^\s]{1,2000}$/iu.test(normalized) ||
    /^\/api\/media\/[a-f0-9-]{36}\/content$/iu.test(normalized)
  );
}

function safeDimension(value: string): boolean {
  const number = Number(value);
  return Number.isInteger(number) && number > 0 && number <= 20_000;
}

function safeSpan(value: string): boolean {
  const number = Number(value);
  return Number.isInteger(number) && number > 0 && number <= 100;
}

function safeColumnWidths(value: string): boolean {
  return value.length <= 1_000 && /^\d+(?:,\d+)*$/u.test(value);
}

function hasDirectBlockChild(element: Element): boolean {
  return Array.from(element.children).some((child) => blockElements.has(child.tagName));
}
