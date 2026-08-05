import { describe, expect, test } from "bun:test";

import type { DocumentNode, ProjectStructure } from "~entities/project";

import { buildExportQueue, buildPublicationPreviewPages, reorderQueue } from "./export-model";

test("builds a depth-first queue from content-bearing node types", () => {
  const structure = {
    nodeTypes: [
      { key: "part", contentBearing: false },
      { key: "scene", contentBearing: true },
      { key: "note", contentBearing: true },
    ],
  } as ProjectStructure;
  const nodes = [
    node("part", "part", undefined, 1000, 0),
    node("second", "scene", undefined, 2000),
    node("first", "scene", "part", 1000),
    node("note", "note", "first", 1000),
  ];
  expect(buildExportQueue(nodes, structure).map((item) => item.id)).toEqual([
    "first",
    "note",
    "second",
  ]);
});

describe("reorderQueue", () => {
  test("moves one selected document without mutating the input", () => {
    const input = ["a", "b", "c"];
    expect(reorderQueue(input, "c", "a")).toEqual(["c", "a", "b"]);
    expect(input).toEqual(["a", "b", "c"]);
  });
});

test("builds separate front-matter preview pages independently of document page breaks", () => {
  const pages = buildPublicationPreviewPages({ titlePage: true, tableOfContents: true }, true, [
    { id: "first", title: "Первая сцена" },
    { id: "second", title: "Вторая сцена" },
  ]);

  expect(pages).toEqual([
    { id: "cover", kind: "cover", label: "Обложка" },
    { id: "title", kind: "title", label: "Титульная страница" },
    { id: "toc", kind: "toc", label: "Оглавление" },
    {
      id: "document:first",
      kind: "document",
      label: "1. Первая сцена",
      documentId: "first",
    },
    {
      id: "document:second",
      kind: "document",
      label: "2. Вторая сцена",
      documentId: "second",
    },
  ]);
});

function node(
  id: string,
  kind: string,
  parentId: string | undefined,
  position: number,
  currentRevision = 1,
): DocumentNode {
  return {
    id,
    projectId: "project",
    parentId,
    kind,
    title: id,
    status: "draft",
    position,
    currentRevision,
    createdAt: `2026-08-05T00:00:0${position / 1000}Z`,
    updatedAt: "2026-08-05T00:00:00Z",
  };
}
