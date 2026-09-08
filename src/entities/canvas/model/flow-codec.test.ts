import { describe, expect, test } from "bun:test";
import { MarkerType, Position } from "@xyflow/react";

import {
  type CanvasFlowEdge,
  canvasFileResource,
  canvasToFlow,
  flowToCanvas,
  sideToPosition,
} from "./flow-codec";
import type { CanvasDocument } from "./types";

describe("JSON Canvas codec", () => {
  test("round-trips extensions while updating geometry", () => {
    const source: CanvasDocument = {
      nodes: [
        { id: "n", type: "text", x: 0, y: 0, width: 200, height: 100, text: "Идея", custom: true },
      ],
      edges: [],
      "x-writers-studio": { schemaVersion: 1, drawings: [{ opaque: true }] },
    };
    const flow = canvasToFlow(source);
    const node = flow.nodes[0];
    if (!node) throw new Error("fixture node is missing");
    node.position = { x: 45, y: 85 };
    const result = flowToCanvas(flow.nodes, flow.edges, source);
    expect(result.nodes[0]).toMatchObject({ x: 45, y: 85, custom: true });
    expect(result["x-writers-studio"]).toEqual(source["x-writers-studio"]);
  });

  test("maps every card, resource, edge and handle variant", () => {
    const source: CanvasDocument = {
      nodes: [
        { id: "empty", type: "text", x: 0, y: 0, width: 100, height: 80, text: "" },
        { id: "group", type: "group", x: -20, y: -20, width: 600, height: 300 },
        {
          id: "document",
          type: "file",
          x: 120,
          y: 0,
          width: 200,
          height: 120,
          file: "document",
          "x-writers-studio": { resource: { kind: "document", id: "doc" } },
        },
        {
          id: "image",
          type: "file",
          x: 340,
          y: 0,
          width: 200,
          height: 120,
          file: "image",
          "x-writers-studio": { resource: { kind: "media", id: "media" } },
        },
        {
          id: "external",
          type: "file",
          x: 560,
          y: 0,
          width: 200,
          height: 120,
          file: "research/map.pdf",
        },
        {
          id: "pdf",
          type: "file",
          x: 780,
          y: 0,
          width: 200,
          height: 260,
          file: "pdf",
          "x-writers-studio": { resource: { kind: "media", id: "pdf" } },
        },
      ],
      edges: [
        {
          id: "labelled",
          fromNode: "document",
          fromSide: "right",
          fromEnd: "arrow",
          toNode: "image",
          toSide: "left",
          toEnd: "none",
          label: "reference",
        },
        { id: "default", fromNode: "empty", toNode: "document" },
      ],
    };
    const document = {
      id: "doc",
      title: "Chapter",
      kind: "chapter",
      status: "draft" as const,
      currentRevision: 2,
      plainText: "Text",
    };
    const media = {
      id: "media",
      projectId: "project",
      mediaType: "image/png" as const,
      originalName: "map.png",
      byteSize: 10,
      width: 10,
      height: 10,
      sha256: "hash",
      contentUrl: "/image",
      createdAt: "now",
    };
    const pdf = {
      ...media,
      id: "pdf",
      mediaType: "application/pdf" as const,
      originalName: "rules.pdf",
      width: null,
      height: null,
    };
    const flow = canvasToFlow(source, [document], [media, pdf]);
    expect(flow.nodes.map((item) => item.type)).toEqual([
      "canvas-text",
      "canvas-group",
      "canvas-document",
      "canvas-image",
      "canvas-external-file",
      "canvas-pdf",
    ]);
    expect(flow.nodes.map((item) => item.ariaLabel)).toEqual([
      "Пустая заметка",
      "Группа без названия",
      "Документ Chapter",
      "Изображение map.png",
      "Файл research/map.pdf",
      "PDF rules.pdf",
    ]);
    expect(flow.edges[0]).toMatchObject({
      markerStart: { type: MarkerType.ArrowClosed },
      markerEnd: undefined,
      ariaLabel: "Связь: reference",
    });
    expect(flow.edges[1]?.markerEnd).toEqual({ type: MarkerType.ArrowClosed });
    expect(
      canvasFileResource(
        source.nodes[2] as Extract<(typeof source.nodes)[number], { type: "file" }>,
      ),
    ).toEqual({
      kind: "document",
      id: "doc",
    });
    expect(
      canvasFileResource(
        source.nodes[4] as Extract<(typeof source.nodes)[number], { type: "file" }>,
      ),
    ).toBeUndefined();
    expect(
      ["top", "right", "bottom", "left"].map((side) =>
        sideToPosition(side as "top" | "right" | "bottom" | "left"),
      ),
    ).toEqual([Position.Top, Position.Right, Position.Bottom, Position.Left]);

    const group = flow.nodes.find((item) => item.id === "group");
    const card = flow.nodes.find((item) => item.id === "document");
    if (!group || !card) throw new Error("fixture nodes are missing");
    group.zIndex = 50;
    card.zIndex = 1;
    card.position = { x: 12.4, y: 24.8 };
    card.measured = { width: 222.2, height: 133.7 };
    const edgeWithoutData: CanvasFlowEdge = {
      id: "raw",
      type: "canvas-edge",
      source: "empty",
      target: "document",
      sourceHandle: "invalid",
      targetHandle: null,
    };
    const roundTrip = flowToCanvas(flow.nodes, [...flow.edges, edgeWithoutData], source);
    expect(roundTrip.nodes[0]?.type).toBe("group");
    expect(roundTrip.nodes.find((item) => item.id === "document")).toMatchObject({
      x: 12,
      y: 25,
      width: 222,
      height: 134,
    });
    expect(roundTrip.edges.at(-1)).toEqual({
      id: "raw",
      fromNode: "empty",
      fromSide: undefined,
      toNode: "document",
      toSide: undefined,
    });
  });

  test("labels missing resources without throwing", () => {
    const missing: CanvasDocument = {
      nodes: [
        {
          id: "document",
          type: "file",
          x: 0,
          y: 0,
          width: 100,
          height: 100,
          file: "missing",
          "x-writers-studio": { resource: { kind: "document", id: "missing" } },
        },
        {
          id: "media",
          type: "file",
          x: 120,
          y: 0,
          width: 100,
          height: 100,
          file: "missing",
          "x-writers-studio": { resource: { kind: "media", id: "missing" } },
        },
      ],
      edges: [],
    };
    expect(canvasToFlow(missing).nodes.map((item) => item.ariaLabel)).toEqual([
      "Документ недоступен",
      "Изображение недоступно",
    ]);
  });
});
