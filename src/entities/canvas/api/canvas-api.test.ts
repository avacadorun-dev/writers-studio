import { afterEach, describe, expect, mock, test } from "bun:test";

import {
  createCanvas,
  duplicateCanvas,
  fetchCanvasLinkPreviews,
  getCanvas,
  getCanvasTypeSchema,
  importCanvas,
  listCanvases,
  promoteCanvasNode,
  restoreCanvas,
  saveCanvasRevision,
  saveCanvasTypeSchema,
  trashCanvas,
  updateCanvas,
} from "./canvas-api";

afterEach(() => mock.restore());

describe("canvas API", () => {
  test("uses the complete canvas resource and revision contract", async () => {
    const requests: Array<{ url: string; method: string; body?: unknown }> = [];
    globalThis.fetch = mock(async (input: RequestInfo | URL, init?: RequestInit) => {
      requests.push({
        url: String(input),
        method: init?.method ?? "GET",
        body: init?.body
          ? init.body instanceof Blob
            ? "blob"
            : JSON.parse(String(init.body))
          : undefined,
      });
      return Response.json({ canvases: [], canvas: {}, revision: {}, schema: {} });
    }) as unknown as typeof fetch;

    await listCanvases("project", false);
    await listCanvases("project");
    await createCanvas("project", "Сюжет");
    await importCanvas("project", new Blob(["{}"], { type: "application/json" }), "Mood Board");
    await getCanvas("canvas");
    await updateCanvas("canvas", { title: "Персонажи", position: 2_000 });
    await duplicateCanvas("canvas");
    await trashCanvas("canvas");
    await restoreCanvas("canvas");
    await saveCanvasRevision("canvas", {
      baseRevision: 3,
      content: { nodes: [], edges: [] },
      source: "manual",
    });
    await fetchCanvasLinkPreviews("project", ["https://example.com/article"]);
    await getCanvasTypeSchema("project");
    await saveCanvasTypeSchema("project", {
      revision: 2,
      cardTypes: [],
      relationTypes: [],
    });
    await promoteCanvasNode("canvas", {
      baseRevision: 4,
      nodeId: "idea",
      title: "Сигнал маяка",
      kind: "scene",
      parentId: "chapter",
    });

    expect(requests).toEqual([
      { url: "/api/projects/project/canvases?includeTrashed=false", method: "GET" },
      { url: "/api/projects/project/canvases?includeTrashed=true", method: "GET" },
      {
        url: "/api/projects/project/canvases",
        method: "POST",
        body: { title: "Сюжет" },
      },
      {
        url: "/api/projects/project/canvases/import?title=Mood%20Board",
        method: "POST",
        body: "blob",
      },
      { url: "/api/canvases/canvas", method: "GET" },
      {
        url: "/api/canvases/canvas",
        method: "PATCH",
        body: { title: "Персонажи", position: 2_000 },
      },
      { url: "/api/canvases/canvas/duplicate", method: "POST" },
      { url: "/api/canvases/canvas", method: "DELETE" },
      { url: "/api/canvases/canvas/restore", method: "POST" },
      {
        url: "/api/canvases/canvas/revisions",
        method: "PUT",
        body: { baseRevision: 3, content: { nodes: [], edges: [] }, source: "manual" },
      },
      {
        url: "/api/projects/project/link-previews",
        method: "POST",
        body: { urls: ["https://example.com/article"] },
      },
      { url: "/api/projects/project/canvas-schema", method: "GET" },
      {
        url: "/api/projects/project/canvas-schema",
        method: "PUT",
        body: {
          baseRevision: 2,
          cardTypes: [],
          relationTypes: [],
          source: "manual",
        },
      },
      {
        url: "/api/canvases/canvas/promotions",
        method: "POST",
        body: {
          baseRevision: 4,
          nodeId: "idea",
          title: "Сигнал маяка",
          kind: "scene",
          parentId: "chapter",
        },
      },
    ]);
  });
});
