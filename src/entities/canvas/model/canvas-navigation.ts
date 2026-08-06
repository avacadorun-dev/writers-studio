import type { ProjectMedia } from "~entities/project";

import type { CanvasDocument, CanvasDocumentPreview, CanvasGroupNode, CanvasNode } from "./types";

export interface CanvasObjectListItem {
  id: string;
  kind: CanvasNode["type"] | "stroke" | "shape" | "frame";
  title: string;
  searchText: string;
}

export function listCanvasObjects(
  document: CanvasDocument,
  documents: CanvasDocumentPreview[] = [],
  media: ProjectMedia[] = [],
): CanvasObjectListItem[] {
  const documentById = new Map(documents.map((item) => [item.id, item]));
  const mediaById = new Map(media.map((item) => [item.id, item]));
  return document.nodes.map((node) => objectListItem(node, documentById, mediaById));
}

export function searchCanvasObjects(
  objects: CanvasObjectListItem[],
  query: string,
): CanvasObjectListItem[] {
  const tokens = normalized(query).split(" ").filter(Boolean);
  if (tokens.length === 0) return objects;
  return objects.filter((object) => tokens.every((token) => object.searchText.includes(token)));
}

export function orderedCanvasFrames(document: CanvasDocument): CanvasGroupNode[] {
  return document.nodes
    .filter(
      (node): node is CanvasGroupNode =>
        node.type === "group" && node["x-writers-studio"]?.element?.kind === "frame",
    )
    .sort((left, right) => {
      const leftOrder = frameOrder(left);
      const rightOrder = frameOrder(right);
      return (
        leftOrder - rightOrder ||
        left.y - right.y ||
        left.x - right.x ||
        left.id.localeCompare(right.id)
      );
    });
}

export function clampFrameIndex(index: number, frameCount: number): number {
  if (frameCount <= 0) return 0;
  return Math.max(0, Math.min(frameCount - 1, Math.trunc(index)));
}

function objectListItem(
  node: CanvasNode,
  documents: Map<string, CanvasDocumentPreview>,
  media: Map<string, ProjectMedia>,
): CanvasObjectListItem {
  if (node.type === "text") {
    const title = firstContentLine(node.text) || "Пустая заметка";
    return item(node, "text", title, node.text);
  }
  if (node.type === "link") {
    const preview = node["x-writers-studio"]?.preview;
    const title = preview?.title || node.url;
    return item(
      node,
      "link",
      title,
      `${title} ${preview?.description ?? ""} ${preview?.siteName ?? ""} ${node.url}`,
    );
  }
  if (node.type === "file") {
    const resource = node["x-writers-studio"]?.resource;
    if (!resource) {
      const title = node.file.split("/").at(-1) || node.file || "Файл";
      return item(node, "file", title, `${title} ${node.file}`);
    }
    if (resource.kind === "document") {
      const document = documents.get(resource.id);
      const title = document?.title || "Недоступный документ";
      return item(
        node,
        "file",
        title,
        `${title} ${document?.plainText ?? ""} ${document?.status ?? ""}`,
      );
    }
    if (resource.kind === "media") {
      const asset = media.get(resource.id);
      const title = asset?.originalName || "Медиа";
      return item(node, "file", title, `${title} ${asset?.mediaType ?? ""}`);
    }
    const title = node.file.split("/").at(-1) || "Вложенная доска";
    return item(node, "file", title, `${title} canvas`);
  }
  const element = node["x-writers-studio"]?.element;
  if (element?.kind === "stroke") {
    return item(node, "stroke", element.brush === "pen" ? "Штрих пера" : "Маркер", element.brush);
  }
  if (element?.kind === "shape") {
    const title = element.label || shapeLabel(element.shape);
    return item(node, "shape", title, `${title} ${element.shape}`);
  }
  if (element?.kind === "frame") {
    const title = element.label || node.label || `Фрейм ${element.order}`;
    return item(node, "frame", title, `${title} frame ${element.order}`);
  }
  const title = node.label || "Группа";
  return item(node, "group", title, title);
}

function item(
  node: CanvasNode,
  kind: CanvasObjectListItem["kind"],
  title: string,
  searchText: string,
): CanvasObjectListItem {
  const card = node["x-writers-studio"]?.card;
  return {
    id: node.id,
    kind,
    title,
    searchText: normalized(
      `${title} ${searchText} ${card?.typeId ?? ""} ${JSON.stringify(card?.fields ?? {})}`,
    ),
  };
}

function firstContentLine(value: string): string {
  return (
    value
      .split("\n")
      .map((line) => line.replace(/^#{1,6}\s+/, "").trim())
      .find(Boolean) ?? ""
  );
}

function shapeLabel(shape: string): string {
  return (
    (
      {
        rectangle: "Прямоугольник",
        ellipse: "Эллипс",
        diamond: "Ромб",
        line: "Линия",
        arrow: "Стрелка",
      } as Record<string, string>
    )[shape] ?? "Фигура"
  );
}

function normalized(value: string): string {
  return value.toLocaleLowerCase("ru-RU").replace(/\s+/g, " ").trim();
}

function frameOrder(node: CanvasGroupNode): number {
  const element = node["x-writers-studio"]?.element;
  return element?.kind === "frame" ? element.order : Number.MAX_SAFE_INTEGER;
}
