import { afterEach, describe, expect, mock, test } from "bun:test";

import {
  createCanvas,
  duplicateCanvas,
  getCanvas,
  listCanvases,
  restoreCanvas,
  saveCanvasRevision,
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
        body: init?.body ? JSON.parse(String(init.body)) : undefined,
      });
      return Response.json({ canvases: [], canvas: {}, revision: {} });
    }) as unknown as typeof fetch;

    await listCanvases("project", false);
    await listCanvases("project");
    await createCanvas("project", "Сюжет");
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

    expect(requests).toEqual([
      { url: "/api/projects/project/canvases?includeTrashed=false", method: "GET" },
      { url: "/api/projects/project/canvases?includeTrashed=true", method: "GET" },
      {
        url: "/api/projects/project/canvases",
        method: "POST",
        body: { title: "Сюжет" },
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
    ]);
  });
});
