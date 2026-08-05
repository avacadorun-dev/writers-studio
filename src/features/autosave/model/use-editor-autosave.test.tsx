import { afterEach, describe, expect, mock, test } from "bun:test";
import { act, cleanup, render, screen } from "@testing-library/react";
import { Editor } from "@tiptap/core";

import { editorExtensions } from "~features/editor";

import { useEditorAutosave } from "./use-editor-autosave";

afterEach(() => {
  cleanup();
  mock.restore();
});

describe("editor autosave", () => {
  test("reports success only after the server confirms durable persistence", async () => {
    let resolveRequest: ((response: Response) => void) | undefined;
    globalThis.fetch = mock(
      () =>
        new Promise<Response>((resolve) => {
          resolveRequest = resolve;
        }),
    ) as unknown as typeof fetch;
    const editor = new Editor({
      extensions: editorExtensions,
      content: { type: "doc", content: [{ type: "paragraph" }] },
    });
    const nodeId = crypto.randomUUID();
    const projectId = crypto.randomUUID();
    const onRevision = () => undefined;

    function Harness() {
      const autosave = useEditorAutosave({
        editor,
        nodeId,
        projectId,
        revision: 1,
        onRevision,
      });
      return <output>{autosave.saveState}</output>;
    }

    render(<Harness />);
    act(() => {
      editor.commands.insertContent("Новая строка");
    });
    expect(screen.getByText("saving")).toBeTruthy();

    await act(async () => {
      await new Promise((resolve) => window.setTimeout(resolve, 1_700));
    });
    expect(resolveRequest).toBeDefined();
    expect(screen.queryByText("saved")).toBeNull();

    await act(async () => {
      resolveRequest?.(
        new Response(
          JSON.stringify({
            revision: {
              id: crypto.randomUUID(),
              nodeId: crypto.randomUUID(),
              revision: 2,
              baseRevision: 1,
              schemaVersion: 1,
              content: editor.getJSON(),
              plainText: editor.getText(),
              source: "manual",
              createdAt: new Date().toISOString(),
            },
          }),
          { status: 201, headers: { "Content-Type": "application/json" } },
        ),
      );
      await new Promise((resolve) => window.setTimeout(resolve, 20));
    });
    expect(screen.getByText("saved")).toBeTruthy();
    editor.destroy();
  });

  test("saves mapped annotation anchors in the same revision request", async () => {
    let requestBody: Record<string, unknown> | undefined;
    globalThis.fetch = mock(async (_input: RequestInfo | URL, init?: RequestInit) => {
      requestBody = JSON.parse(String(init?.body)) as Record<string, unknown>;
      return new Response(
        JSON.stringify({
          revision: {
            id: crypto.randomUUID(),
            nodeId: crypto.randomUUID(),
            revision: 2,
            baseRevision: 1,
            schemaVersion: 1,
            content: requestBody.content,
            plainText: "Новый текст",
            source: "manual",
            createdAt: new Date().toISOString(),
          },
        }),
        { status: 201, headers: { "Content-Type": "application/json" } },
      );
    }) as unknown as typeof fetch;
    const editor = new Editor({
      extensions: editorExtensions,
      content: {
        type: "doc",
        content: [
          {
            type: "paragraph",
            attrs: { blockId: "block-1", sourceRefs: [] },
            content: [{ type: "text", text: "Текст" }],
          },
        ],
      },
    });
    const expectedAnchors = [
      {
        id: "note-1",
        anchor: {
          blockId: "block-1",
          start: 0,
          end: 5,
          exactText: "Текст",
          prefix: "",
          suffix: "",
        },
      },
    ];
    const nodeId = crypto.randomUUID();
    const projectId = crypto.randomUUID();
    let flush: (() => Promise<void>) | undefined;

    function Harness() {
      const autosave = useEditorAutosave({
        editor,
        nodeId,
        projectId,
        revision: 1,
        onRevision: () => undefined,
        getAnnotationAnchors: () => expectedAnchors,
      });
      flush = autosave.flush;
      return <output>{autosave.saveState}</output>;
    }

    render(<Harness />);
    act(() => {
      editor.commands.insertContent(" новый");
    });
    await act(async () => flush?.());

    expect(requestBody?.annotationAnchors).toEqual(expectedAnchors);
    expect(requestBody?.baseRevision).toBe(1);
    expect(screen.getByText("saved")).toBeTruthy();
    editor.destroy();
  });

  test("surfaces a durable revision conflict without retrying over it", async () => {
    let calls = 0;
    globalThis.fetch = mock(async () => {
      calls += 1;
      return Response.json(
        {
          error: {
            code: "revision_conflict",
            message: "Мы сохранили оба варианта",
            currentRevision: 3,
            conflictNodeId: crypto.randomUUID(),
          },
        },
        { status: 409 },
      );
    }) as unknown as typeof fetch;
    const editor = new Editor({
      extensions: editorExtensions,
      content: { type: "doc", content: [{ type: "paragraph" }] },
    });
    let flush: (() => Promise<void>) | undefined;

    function Harness() {
      const autosave = useEditorAutosave({
        editor,
        nodeId: crypto.randomUUID(),
        projectId: crypto.randomUUID(),
        revision: 1,
        onRevision: () => undefined,
      });
      flush = autosave.flush;
      return <output>{autosave.saveState}</output>;
    }

    const view = render(<Harness />);
    act(() => editor.commands.insertContent("Конфликт"));
    await act(async () => flush?.());
    expect(screen.getByText("conflict")).toBeTruthy();
    await act(async () => {
      window.dispatchEvent(new Event("online"));
      await new Promise((resolve) => window.setTimeout(resolve, 20));
    });
    expect(calls).toBe(1);
    view.unmount();
    editor.destroy();
  });

  test("keeps a failed save local and retries when connectivity returns", async () => {
    let calls = 0;
    globalThis.fetch = mock(async () => {
      calls += 1;
      if (calls === 1) throw new Error("offline");
      return Response.json(
        {
          revision: {
            id: crypto.randomUUID(),
            nodeId: crypto.randomUUID(),
            revision: 2,
            baseRevision: 1,
            schemaVersion: 1,
            content: { type: "doc", content: [{ type: "paragraph" }] },
            plainText: "Локальная правка",
            source: "manual",
            createdAt: new Date().toISOString(),
          },
        },
        { status: 201 },
      );
    }) as unknown as typeof fetch;
    const editor = new Editor({
      extensions: editorExtensions,
      content: { type: "doc", content: [{ type: "paragraph" }] },
    });
    let flush: (() => Promise<void>) | undefined;

    function Harness() {
      const autosave = useEditorAutosave({
        editor,
        nodeId: "offline-node",
        projectId: "offline-project",
        revision: 1,
        onRevision: () => undefined,
      });
      flush = autosave.flush;
      return <output>{autosave.saveState}</output>;
    }

    const view = render(<Harness />);
    act(() => editor.commands.insertContent("Локальная правка"));
    await act(async () => flush?.());
    expect(screen.getByText(/pending|local/)).toBeTruthy();
    await act(async () => {
      window.dispatchEvent(new Event("online"));
      await new Promise((resolve) => window.setTimeout(resolve, 30));
    });
    expect(calls).toBe(2);
    expect(screen.getByText("saved")).toBeTruthy();
    view.unmount();
    editor.destroy();
  });

  test("is a safe no-op before the editor is mounted", async () => {
    let flush: (() => Promise<void>) | undefined;
    function Harness() {
      const autosave = useEditorAutosave({
        editor: null,
        nodeId: "node",
        projectId: "project",
        revision: 1,
        onRevision: () => undefined,
      });
      flush = autosave.flush;
      return <output>{autosave.saveState}</output>;
    }
    render(<Harness />);
    await act(async () => flush?.());
    expect(screen.getByText("saved")).toBeTruthy();
  });
});
