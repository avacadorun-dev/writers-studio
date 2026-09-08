import { afterEach, describe, expect, mock, test } from "bun:test";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";

import type { CanvasDocument } from "~entities/canvas";
import { getCanvasSnapshot } from "~shared/lib";

import { runLocalRecoveryOperation, useCanvasAutosave } from "./use-canvas-autosave";

const content: CanvasDocument = {
  nodes: [{ id: "note", type: "text", x: 0, y: 0, width: 200, height: 120, text: "Идея" }],
  edges: [],
};

afterEach(() => {
  cleanup();
  mock.restore();
});

function revisionResponse(revision = 2) {
  return Response.json(
    {
      revision: {
        id: crypto.randomUUID(),
        canvasId: "canvas",
        revision,
        baseRevision: revision - 1,
        schemaVersion: 1,
        content,
        source: "manual",
        createdAt: new Date().toISOString(),
      },
    },
    { status: 201 },
  );
}

describe("canvas autosave", () => {
  test("treats unavailable local recovery as best-effort", async () => {
    expect(await runLocalRecoveryOperation(async () => undefined)).toBeTrue();
    expect(
      await runLocalRecoveryOperation(async () => {
        throw new Error("IndexedDB unavailable");
      }),
    ).toBeFalse();
  });

  test("keeps queue stable when a parent recreates the revision callback", () => {
    let queue: ((value: CanvasDocument) => void) | undefined;
    function Harness({ tick }: { tick: number }) {
      const autosave = useCanvasAutosave({
        canvasId: "canvas-stable-callback",
        projectId: "project",
        revision: 1,
        onRevision: () => tick,
      });
      queue = autosave.queue;
      return <output>{tick}</output>;
    }
    const view = render(<Harness tick={1} />);
    const firstQueue = queue;
    view.rerender(<Harness tick={2} />);
    expect(queue).toBe(firstQueue);
  });

  test("persists locally and reports success after the server revision", async () => {
    let body: Record<string, unknown> | undefined;
    globalThis.fetch = mock(async (_input: RequestInfo | URL, init?: RequestInit) => {
      body = JSON.parse(String(init?.body)) as Record<string, unknown>;
      return revisionResponse();
    }) as unknown as typeof fetch;
    const revisions: number[] = [];
    let queue: ((value: CanvasDocument) => void) | undefined;
    let flush: (() => Promise<void>) | undefined;

    function Harness() {
      const autosave = useCanvasAutosave({
        canvasId: "canvas-success",
        projectId: "project",
        revision: 1,
        onRevision: (value) => revisions.push(value),
      });
      queue = autosave.queue;
      flush = autosave.flush;
      return <output>{autosave.saveState}</output>;
    }

    render(<Harness />);
    act(() => queue?.(content));
    expect(screen.getByText("saving")).toBeTruthy();
    await act(async () => flush?.());

    expect(body).toMatchObject({ baseRevision: 1, content, source: "manual" });
    expect(revisions).toEqual([2]);
    expect(screen.getByText("saved")).toBeTruthy();
    expect(await getCanvasSnapshot("canvas-success")).toMatchObject({
      baseRevision: 2,
      synced: true,
    });
  });

  test("creates a durable conflict copy once and acknowledges the local snapshot", async () => {
    let calls = 0;
    globalThis.fetch = mock(async () => {
      calls += 1;
      return Response.json(
        {
          error: {
            code: "canvas_revision_conflict",
            message: "Сохранены оба варианта",
            currentRevision: 4,
            conflictCanvasId: "conflict-canvas",
          },
        },
        { status: 409 },
      );
    }) as unknown as typeof fetch;
    let queue: ((value: CanvasDocument) => void) | undefined;
    let flush: (() => Promise<void>) | undefined;

    function Harness() {
      const autosave = useCanvasAutosave({
        canvasId: "canvas-conflict",
        projectId: "project",
        revision: 1,
        onRevision: () => undefined,
      });
      queue = autosave.queue;
      flush = autosave.flush;
      return <output>{`${autosave.saveState}:${autosave.conflictCanvasId ?? ""}`}</output>;
    }

    render(<Harness />);
    act(() => queue?.(content));
    await act(async () => flush?.());
    expect(screen.getByText("conflict:conflict-canvas")).toBeTruthy();
    expect(await getCanvasSnapshot("canvas-conflict")).toMatchObject({
      baseRevision: 4,
      synced: true,
    });
    await act(async () => {
      window.dispatchEvent(new Event("online"));
      await new Promise((resolve) => window.setTimeout(resolve, 20));
    });
    expect(calls).toBe(1);
  });

  test("keeps a failed revision local and retries when online", async () => {
    let calls = 0;
    globalThis.fetch = mock(async () => {
      calls += 1;
      if (calls === 1) throw new Error("offline");
      return revisionResponse();
    }) as unknown as typeof fetch;
    let queue: ((value: CanvasDocument) => void) | undefined;
    let flush: (() => Promise<void>) | undefined;

    function Harness() {
      const autosave = useCanvasAutosave({
        canvasId: "canvas-retry",
        projectId: "project",
        revision: 1,
        onRevision: () => undefined,
      });
      queue = autosave.queue;
      flush = autosave.flush;
      return <output>{autosave.saveState}</output>;
    }

    render(<Harness />);
    act(() => queue?.(content));
    await act(async () => flush?.());
    expect(screen.getByText(/pending|local/)).toBeTruthy();
    act(() => {
      window.dispatchEvent(new Event("online"));
    });
    await waitFor(() => {
      expect(calls).toBe(2);
      expect(screen.getByText("saved")).toBeTruthy();
    });
  });

  test("serializes rapid flushes and advances baseRevision before saving the latest snapshot", async () => {
    const bases: number[] = [];
    let releaseFirst: (() => void) | undefined;
    let calls = 0;
    globalThis.fetch = mock(async (_input: RequestInfo | URL, init?: RequestInit) => {
      calls += 1;
      const request = JSON.parse(String(init?.body)) as { baseRevision: number };
      bases.push(request.baseRevision);
      if (calls === 1) {
        await new Promise<void>((resolve) => {
          releaseFirst = resolve;
        });
        return revisionResponse(2);
      }
      return revisionResponse(3);
    }) as unknown as typeof fetch;
    let queue: ((value: CanvasDocument) => void) | undefined;
    let flush: (() => Promise<void>) | undefined;

    function Harness() {
      const autosave = useCanvasAutosave({
        canvasId: "canvas-serialized",
        projectId: "project",
        revision: 1,
        onRevision: () => undefined,
      });
      queue = autosave.queue;
      flush = autosave.flush;
      return <output>{autosave.saveState}</output>;
    }

    render(<Harness />);
    act(() => queue?.(content));
    let firstFlush: Promise<void> | undefined;
    await act(async () => {
      firstFlush = flush?.();
      for (let attempt = 0; attempt < 20 && calls === 0; attempt += 1) {
        await new Promise((resolve) => window.setTimeout(resolve, 5));
      }
    });
    expect(calls).toBe(1);
    const originalNode = content.nodes[0];
    if (originalNode?.type !== "text") throw new Error("fixture note is missing");
    act(() => queue?.({ ...content, nodes: [{ ...originalNode, text: "Новая идея" }] }));
    await act(async () => flush?.());
    expect(calls).toBe(1);

    releaseFirst?.();
    await act(async () => firstFlush);
    await act(async () => flush?.());
    expect(bases).toEqual([1, 2]);
    expect(screen.getByText("saved")).toBeTruthy();
  });

  test("flush is a no-op until content is queued", async () => {
    let flush: (() => Promise<void>) | undefined;
    function Harness() {
      const autosave = useCanvasAutosave({
        canvasId: "canvas-empty",
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
