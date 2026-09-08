import { describe, expect, test } from "bun:test";

import { canvasTemplates, createCanvasTemplate } from "./templates";

describe("canvas templates", () => {
  test("builds every template with unique IDs and v2 content", () => {
    for (const template of canvasTemplates) {
      let sequence = 0;
      const document = createCanvasTemplate(template.id, () => `${template.id}-${sequence++}`, {
        x: 100,
        y: 200,
      });
      const ids = [
        ...document.nodes.map((node) => node.id),
        ...document.edges.map((edge) => edge.id),
      ];
      expect(new Set(ids).size).toBe(ids.length);
      expect(document["x-writers-studio"]?.schemaVersion).toBe(2);
      expect(document.nodes.every((node) => node.x >= 60 && node.y >= 130)).toBe(true);
      for (const edge of document.edges) {
        expect(document.nodes.some((node) => node.id === edge.fromNode)).toBe(true);
        expect(document.nodes.some((node) => node.id === edge.toNode)).toBe(true);
      }
    }
  });

  test("creates an actually blank board", () => {
    expect(createCanvasTemplate("blank", () => "unused")).toEqual({
      nodes: [],
      edges: [],
      "x-writers-studio": { schemaVersion: 2 },
    });
  });

  test("builds author and game-design templates as typed analyzable graphs", () => {
    for (const id of [
      "story-map",
      "character-network",
      "chronology",
      "quest-dependency",
      "dialogue-tree",
      "state-machine",
      "gameplay-core-loop",
      "level-flow",
    ] as const) {
      let sequence = 0;
      const document = createCanvasTemplate(id, () => `${id}-${sequence++}`);
      expect(document.nodes.every((node) => node["x-writers-studio"]?.card?.typeId)).toBe(true);
      expect(document.edges.length).toBeGreaterThan(0);
      expect(document.edges.every((edge) => edge["x-writers-studio"]?.relation?.typeId)).toBe(true);
    }
  });
});
