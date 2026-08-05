import { afterEach, describe, expect, mock, test } from "bun:test";

import type { AnnotationAnchor } from "../model/types";
import {
  createDocumentAnnotation,
  deleteDocumentAnnotation,
  listDocumentAnnotations,
  listProjectAnnotations,
  saveRevision,
  updateDocumentAnnotation,
} from "./project-api";

afterEach(() => mock.restore());

const anchor: AnnotationAnchor = {
  blockId: "block-1",
  start: 0,
  end: 5,
  exactText: "Текст",
  prefix: "",
  suffix: "",
};

describe("project annotation API", () => {
  test("uses the project and document list endpoints", async () => {
    const requests: Array<{ url: string; method: string }> = [];
    globalThis.fetch = mock(async (input: RequestInfo | URL, init?: RequestInit) => {
      requests.push({ url: String(input), method: init?.method ?? "GET" });
      return Response.json({ annotations: [] });
    }) as unknown as typeof fetch;

    await listProjectAnnotations("project-1");
    await listDocumentAnnotations("node-1");

    expect(requests).toEqual([
      { url: "/api/projects/project-1/annotations", method: "GET" },
      { url: "/api/nodes/node-1/annotations", method: "GET" },
    ]);
  });

  test("creates, updates, and deletes annotations with explicit JSON contracts", async () => {
    const requests: Array<{ url: string; method: string; body?: unknown }> = [];
    globalThis.fetch = mock(async (input: RequestInfo | URL, init?: RequestInit) => {
      requests.push({
        url: String(input),
        method: init?.method ?? "GET",
        body: init?.body ? JSON.parse(String(init.body)) : undefined,
      });
      if (init?.method === "DELETE") return new Response(null, { status: 204 });
      return Response.json({ annotation: { id: "annotation-1" } });
    }) as unknown as typeof fetch;

    await createDocumentAnnotation("node-1", {
      kind: "note",
      anchor,
      note: "Проверить",
      baseRevision: 2,
    });
    await updateDocumentAnnotation("annotation-1", { status: "resolved", note: "Готово" });
    await deleteDocumentAnnotation("annotation-1");

    expect(requests).toEqual([
      {
        url: "/api/nodes/node-1/annotations",
        method: "POST",
        body: { kind: "note", anchor, note: "Проверить", baseRevision: 2 },
      },
      {
        url: "/api/annotations/annotation-1",
        method: "PATCH",
        body: { status: "resolved", note: "Готово" },
      },
      { url: "/api/annotations/annotation-1", method: "DELETE", body: undefined },
    ]);
  });

  test("sends anchor mappings atomically with manuscript content", async () => {
    let body: Record<string, unknown> | undefined;
    globalThis.fetch = mock(async (_input: RequestInfo | URL, init?: RequestInit) => {
      body = JSON.parse(String(init?.body)) as Record<string, unknown>;
      return Response.json({ revision: { revision: 3 } }, { status: 201 });
    }) as unknown as typeof fetch;

    await saveRevision("node-1", {
      baseRevision: 2,
      content: { type: "doc", content: [{ type: "paragraph" }] },
      source: "manual",
      annotationAnchors: [{ id: "annotation-1", anchor }],
    });

    expect(body).toEqual({
      baseRevision: 2,
      content: { type: "doc", content: [{ type: "paragraph" }] },
      source: "manual",
      annotationAnchors: [{ id: "annotation-1", anchor }],
    });
  });
});
