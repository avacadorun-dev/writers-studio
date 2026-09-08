import { afterEach, describe, expect, test } from "bun:test";

import { applyCanvasLayout, buildCanvasLayoutGraph, requestCanvasLayout } from "./layout";
import type { CanvasDocument } from "./types";

const originalWorker = globalThis.Worker;
afterEach(() => {
  globalThis.Worker = originalWorker;
});

const document: CanvasDocument = {
  nodes: [
    { id: "a", type: "text", x: 0, y: 0, width: 100, height: 80, text: "A" },
    {
      id: "b",
      type: "text",
      x: 200,
      y: 100,
      width: 120,
      height: 80,
      text: "B",
      "x-writers-studio": { locked: true },
    },
    {
      id: "stroke",
      type: "group",
      x: 0,
      y: 200,
      width: 100,
      height: 100,
      "x-writers-studio": {
        element: {
          kind: "stroke",
          points: [
            [1, 1],
            [2, 2],
          ],
          brush: "pen",
          color: "1",
          size: 2,
          opacity: 1,
        },
      },
    },
  ],
  edges: [
    { id: "ab", fromNode: "a", toNode: "b" },
    { id: "ignored", fromNode: "a", toNode: "stroke" },
  ],
};

describe("canvas layouts", () => {
  test("builds deterministic ELK graphs for hierarchy, radial and organic modes", () => {
    const horizontal = buildCanvasLayoutGraph(document, [], "hierarchy-horizontal");
    expect(horizontal.children.map((node) => node.id)).toEqual(["a", "b"]);
    expect(horizontal.edges.map((edge) => edge.id)).toEqual(["ab"]);
    expect(horizontal.layoutOptions).toMatchObject({
      "elk.algorithm": "layered",
      "elk.direction": "RIGHT",
    });
    expect(buildCanvasLayoutGraph(document, ["a", "stroke"], "hierarchy-vertical")).toMatchObject({
      children: [{ id: "a" }],
      layoutOptions: { "elk.direction": "DOWN" },
    });
    expect(buildCanvasLayoutGraph(document, [], "mind-map").layoutOptions["elk.algorithm"]).toBe(
      "radial",
    );
    expect(buildCanvasLayoutGraph(document, [], "organic").layoutOptions["elk.algorithm"]).toBe(
      "force",
    );
  });

  test("applies finite positions while preserving pinned and unknown nodes", () => {
    expect(applyCanvasLayout(document, {})).toBe(document);
    const result = applyCanvasLayout(document, {
      children: [
        { id: "a", width: 100, height: 80, x: 42.6, y: 19.2 },
        { id: "b", width: 120, height: 80, x: 500, y: 600 },
        { id: "stroke", width: 100, height: 100, x: Number.NaN, y: 2 },
      ],
    });
    expect(result.nodes[0]).toMatchObject({ x: 43, y: 19 });
    expect(result.nodes[1]).toBe(document.nodes[1]);
    expect(result.nodes[2]).toBe(document.nodes[2]);
  });

  test("runs layout work in a disposable worker and handles server and worker errors", async () => {
    let mode: "success" | "message-error" | "worker-error" = "success";
    let terminated = 0;
    class FakeWorker {
      private message?: (event: MessageEvent<{ result?: unknown; error?: string }>) => void;
      private error?: () => void;

      addEventListener(type: string, listener: EventListenerOrEventListenerObject) {
        const callable = listener as (
          event: MessageEvent<{ result?: unknown; error?: string }>,
        ) => void;
        if (type === "message") this.message = callable;
        if (type === "error") this.error = () => callable(new MessageEvent("error"));
      }

      postMessage() {
        queueMicrotask(() => {
          if (mode === "worker-error") this.error?.();
          else if (mode === "message-error")
            this.message?.(new MessageEvent("message", { data: { error: "ELK failed" } }));
          else {
            this.message?.(
              new MessageEvent("message", {
                data: { result: { children: [{ id: "a", x: 55, y: 66 }] } },
              }),
            );
          }
        });
      }

      terminate() {
        terminated += 1;
      }
    }
    globalThis.Worker = FakeWorker as unknown as typeof Worker;

    await expect(requestCanvasLayout(document, ["a"], "organic")).resolves.toBe(document);
    const laidOut = await requestCanvasLayout(document, [], "organic");
    expect(laidOut.nodes[0]).toMatchObject({ x: 55, y: 66 });
    mode = "message-error";
    await expect(requestCanvasLayout(document, [], "mind-map")).rejects.toThrow("ELK failed");
    mode = "worker-error";
    await expect(requestCanvasLayout(document, [], "hierarchy-horizontal")).rejects.toThrow(
      "Не удалось запустить алгоритм раскладки",
    );
    expect(terminated).toBe(3);
  });
});
