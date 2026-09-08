import { afterEach, describe, expect, mock, test } from "bun:test";

import {
  getProjectAssistantSettings,
  updateNode,
  updateProject,
  updateProjectAssistantSettings,
  uploadProjectImage,
} from "./project-api";

afterEach(() => mock.restore());

describe("project metadata API", () => {
  test("renames a node through the canonical mutation without touching content", async () => {
    let request: { url: string; method: string; body: unknown } | undefined;
    globalThis.fetch = mock(async (input: RequestInfo | URL, init?: RequestInit) => {
      request = {
        url: String(input),
        method: init?.method ?? "GET",
        body: JSON.parse(String(init?.body)),
      };
      return Response.json({ node: { id: "node-1", title: "Новая сцена" } });
    }) as unknown as typeof fetch;

    await updateNode("node-1", { title: "Новая сцена" });
    expect(request).toEqual({
      url: "/api/nodes/node-1",
      method: "PATCH",
      body: { title: "Новая сцена" },
    });
  });

  test("updates editable project fields through the project endpoint", async () => {
    let request: { url: string; method: string; body: unknown } | undefined;
    globalThis.fetch = mock(async (input: RequestInfo | URL, init?: RequestInit) => {
      request = {
        url: String(input),
        method: init?.method ?? "GET",
        body: JSON.parse(String(init?.body)),
      };
      return Response.json({ project: { id: "project-1" } });
    }) as unknown as typeof fetch;

    await updateProject("project-1", {
      title: "Новое название",
      language: "ru",
      synopsis: "Обновлённый замысел",
      wordGoal: 90_000,
    });

    expect(request).toEqual({
      url: "/api/projects/project-1",
      method: "PATCH",
      body: {
        title: "Новое название",
        language: "ru",
        synopsis: "Обновлённый замысел",
        wordGoal: 90_000,
      },
    });
  });

  test("omits wordGoal to clear the optional target", async () => {
    let body: unknown;
    globalThis.fetch = mock(async (_input: RequestInfo | URL, init?: RequestInit) => {
      body = JSON.parse(String(init?.body));
      return Response.json({ project: { id: "project-1" } });
    }) as unknown as typeof fetch;

    await updateProject("project-1", {
      title: "Без цели",
      language: "en",
      synopsis: "",
    });

    expect(body).toEqual({ title: "Без цели", language: "en", synopsis: "" });
  });

  test("reads and updates the optional assistant step limit", async () => {
    const requests: Array<{ url: string; method: string; body?: unknown }> = [];
    globalThis.fetch = mock(async (input: RequestInfo | URL, init?: RequestInit) => {
      requests.push({
        url: String(input),
        method: init?.method ?? "GET",
        body: init?.body ? JSON.parse(String(init.body)) : undefined,
      });
      return Response.json({ settings: { stepLimit: null } });
    }) as unknown as typeof fetch;

    await getProjectAssistantSettings("project-1");
    await updateProjectAssistantSettings("project-1", { stepLimit: 40 });
    await updateProjectAssistantSettings("project-1", {});

    expect(requests).toEqual([
      { url: "/api/projects/project-1/assistant-settings", method: "GET" },
      {
        url: "/api/projects/project-1/assistant-settings",
        method: "PUT",
        body: { stepLimit: 40 },
      },
      {
        url: "/api/projects/project-1/assistant-settings",
        method: "PUT",
        body: { stepLimit: null },
      },
    ]);
  });

  test("uploads a project image as an unchanged binary body", async () => {
    let request: { url: string; body: BodyInit | null | undefined; headers: Headers } | undefined;
    globalThis.fetch = mock(async (input: RequestInfo | URL, init?: RequestInit) => {
      request = {
        url: String(input),
        body: init?.body,
        headers: new Headers(init?.headers),
      };
      return Response.json({ media: { id: "media-1" } }, { status: 201 });
    }) as unknown as typeof fetch;
    const file = new File([new Uint8Array([1, 2, 3])], "карта уровня.png", {
      type: "image/png",
    });

    await uploadProjectImage("project-1", file);

    expect(request?.url).toBe("/api/projects/project-1/media");
    expect(request?.body).toBe(file);
    expect(request?.headers.get("Content-Type")).toBe("image/png");
    expect(decodeURIComponent(request?.headers.get("X-File-Name") ?? "")).toBe("карта уровня.png");
  });
});
