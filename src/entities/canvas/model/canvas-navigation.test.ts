import { describe, expect, test } from "bun:test";

import type { ProjectMedia } from "~entities/project";

import {
  clampFrameIndex,
  listCanvasObjects,
  orderedCanvasFrames,
  searchCanvasObjects,
} from "./canvas-navigation";
import type { CanvasDocument, CanvasDocumentPreview } from "./types";

const document: CanvasDocument = {
  nodes: [
    { id: "note", type: "text", x: 0, y: 0, width: 100, height: 100, text: "# Тайная идея\n\nЭхо" },
    {
      id: "file",
      type: "file",
      file: "document",
      x: 0,
      y: 0,
      width: 100,
      height: 100,
      "x-writers-studio": {
        resource: { kind: "document", id: "doc" },
        card: { typeId: "mechanic", fields: {} },
      },
    },
    {
      id: "media",
      type: "file",
      file: "media",
      x: 0,
      y: 0,
      width: 100,
      height: 100,
      "x-writers-studio": { resource: { kind: "media", id: "asset" } },
    },
    {
      id: "external",
      type: "file",
      file: "research/rules.pdf",
      x: 0,
      y: 0,
      width: 100,
      height: 100,
    },
    {
      id: "link",
      type: "link",
      url: "https://example.com",
      x: 0,
      y: 0,
      width: 100,
      height: 100,
      "x-writers-studio": { preview: { title: "Исследование звука" } },
    },
    { id: "group", type: "group", x: 0, y: 0, width: 100, height: 100, label: "Кластер" },
    {
      id: "frame-2",
      type: "group",
      x: 0,
      y: 200,
      width: 100,
      height: 100,
      "x-writers-studio": { element: { kind: "frame", order: 2, label: "Финал" } },
    },
    {
      id: "frame-1",
      type: "group",
      x: 0,
      y: 300,
      width: 100,
      height: 100,
      "x-writers-studio": { element: { kind: "frame", order: 1, label: "Начало" } },
    },
    {
      id: "shape",
      type: "group",
      x: 0,
      y: 0,
      width: 100,
      height: 100,
      "x-writers-studio": {
        element: {
          kind: "shape",
          shape: "diamond",
          color: "2",
          fill: "transparent",
          size: 2,
          opacity: 1,
          rotation: 0,
          dash: "solid",
          label: "Решение",
        },
      },
    },
    {
      id: "stroke",
      type: "group",
      x: 0,
      y: 0,
      width: 100,
      height: 100,
      "x-writers-studio": {
        element: {
          kind: "stroke",
          brush: "pen",
          points: [
            [0, 0],
            [20, 20],
          ],
          color: "5",
          size: 3,
          opacity: 1,
        },
      },
    },
  ],
  edges: [],
};

const documents: CanvasDocumentPreview[] = [
  {
    id: "doc",
    title: "Система эха",
    kind: "system",
    status: "ready",
    currentRevision: 2,
    plainText: "Проявляет маршрут",
  },
];
const media: ProjectMedia[] = [
  {
    id: "asset",
    projectId: "project",
    mediaType: "image/png",
    originalName: "island.png",
    byteSize: 10,
    width: 10,
    height: 10,
    sha256: "hash",
    contentUrl: "/asset",
    createdAt: "now",
  },
];

describe("canvas navigation", () => {
  test("builds searchable accessible labels for every object kind", () => {
    const objects = listCanvasObjects(document, documents, media);
    expect(objects).toHaveLength(document.nodes.length);
    expect(searchCanvasObjects(objects, "система mechanic").map((item) => item.id)).toEqual([
      "file",
    ]);
    expect(searchCanvasObjects(objects, "island png").map((item) => item.id)).toEqual(["media"]);
    expect(searchCanvasObjects(objects, "rules pdf").map((item) => item.id)).toEqual(["external"]);
    expect(searchCanvasObjects(objects, "решение diamond").map((item) => item.id)).toEqual([
      "shape",
    ]);
    expect(searchCanvasObjects(objects, "исследование звука").map((item) => item.id)).toEqual([
      "link",
    ]);
    expect(objects.find((item) => item.id === "group")?.title).toBe("Кластер");
    expect(objects.find((item) => item.id === "frame-1")?.kind).toBe("frame");
    expect(objects.find((item) => item.id === "stroke")?.title).toBe("Штрих пера");
    expect(searchCanvasObjects(objects, "   ")).toBe(objects);
  });

  test("keeps missing resources and unlabeled objects discoverable", () => {
    const missing: CanvasDocument = {
      nodes: [
        { id: "empty", type: "text", text: "", x: 0, y: 0, width: 40, height: 40 },
        {
          id: "doc",
          type: "file",
          file: "doc",
          x: 0,
          y: 0,
          width: 40,
          height: 40,
          "x-writers-studio": { resource: { kind: "document", id: "missing" } },
        },
        {
          id: "media",
          type: "file",
          file: "media",
          x: 0,
          y: 0,
          width: 40,
          height: 40,
          "x-writers-studio": { resource: { kind: "media", id: "missing" } },
        },
        {
          id: "nested",
          type: "file",
          file: "writers-studio://canvases/Архив",
          x: 0,
          y: 0,
          width: 40,
          height: 40,
          "x-writers-studio": { resource: { kind: "canvas", id: "canvas" } },
        },
        {
          id: "shape",
          type: "group",
          x: 0,
          y: 0,
          width: 40,
          height: 40,
          "x-writers-studio": {
            element: {
              kind: "shape",
              shape: "ellipse",
              color: "5",
              fill: "transparent",
              size: 2,
              opacity: 1,
              rotation: 0,
              dash: "solid",
            },
          },
        },
        {
          id: "marker",
          type: "group",
          x: 0,
          y: 0,
          width: 40,
          height: 40,
          "x-writers-studio": {
            element: {
              kind: "stroke",
              brush: "highlighter",
              points: [
                [0, 0],
                [10, 10],
              ],
              color: "3",
              size: 8,
              opacity: 0.3,
            },
          },
        },
      ],
      edges: [],
    };
    const objects = listCanvasObjects(missing);
    expect(objects.map((item) => item.title)).toEqual([
      "Пустая заметка",
      "Недоступный документ",
      "Медиа",
      "Архив",
      "Эллипс",
      "Маркер",
    ]);
  });

  test("orders frames deterministically and clamps presentation indices", () => {
    expect(orderedCanvasFrames(document).map((frame) => frame.id)).toEqual(["frame-1", "frame-2"]);
    expect(clampFrameIndex(-4, 2)).toBe(0);
    expect(clampFrameIndex(9, 2)).toBe(1);
    expect(clampFrameIndex(1.8, 3)).toBe(1);
    expect(clampFrameIndex(5, 0)).toBe(0);
  });
});
