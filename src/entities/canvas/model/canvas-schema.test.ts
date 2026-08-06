import { describe, expect, test } from "bun:test";

import {
  builtInCanvasCardTypes,
  builtInCanvasRelationTypes,
  defaultCanvasTypeSchema,
  resolveCanvasTypeSchema,
  validateCanvasTypedDocument,
} from "./canvas-schema";
import type { CanvasDocument, CanvasTypeSchema } from "./types";

describe("canvas type schema", () => {
  test("exposes the complete author and game-design catalog", () => {
    expect(builtInCanvasCardTypes.map((type) => type.id)).toEqual([
      "idea",
      "question",
      "source",
      "character",
      "location",
      "beat",
      "conflict",
      "theme",
      "mechanic",
      "quest",
      "dialogue",
      "state",
      "level",
      "resource",
    ]);
    expect(builtInCanvasRelationTypes).toHaveLength(11);
    expect(defaultCanvasTypeSchema.revision).toBe(0);
    expect(resolveCanvasTypeSchema()).toBe(defaultCanvasTypeSchema);
    expect(resolveCanvasTypeSchema({ revision: 0, cardTypes: [], relationTypes: [] })).toBe(
      defaultCanvasTypeSchema,
    );
    const saved = { revision: 2, cardTypes: [], relationTypes: [] };
    expect(resolveCanvasTypeSchema(saved)).toBe(saved);
  });

  test("validates field values and allowed typed relations", () => {
    const schema: CanvasTypeSchema = {
      revision: 2,
      cardTypes: [
        {
          id: "custom",
          name: "Custom",
          category: "custom",
          color: "5",
          icon: "Box",
          fields: [
            { id: "name", name: "Name", type: "short-text", required: true },
            { id: "count", name: "Count", type: "number" },
            { id: "enabled", name: "Enabled", type: "checkbox" },
            { id: "date", name: "Date", type: "date" },
            { id: "tags", name: "Tags", type: "multi-select" },
            { id: "choice", name: "Choice", type: "single-select", options: ["a", "b"] },
            { id: "duration", name: "Duration", type: "duration" },
            { id: "document", name: "Document", type: "document" },
          ],
        },
      ],
      relationTypes: [
        {
          id: "only-custom",
          name: "Only",
          color: "5",
          directed: true,
          fromTypes: ["custom"],
          toTypes: ["custom"],
        },
      ],
    };
    const node = (id: string, fields: Record<string, unknown>, typeId = "custom") => ({
      id,
      type: "text" as const,
      x: 0,
      y: 0,
      width: 100,
      height: 100,
      text: id,
      "x-writers-studio": { card: { typeId, fields } },
    });
    const valid: CanvasDocument = {
      nodes: [
        node("a", {
          name: "A",
          count: 1,
          enabled: true,
          date: "2026-01-01",
          tags: ["x"],
          choice: "a",
          duration: 5,
          document: "doc",
        }),
        node("b", { name: "B" }),
      ],
      edges: [
        {
          id: "ab",
          fromNode: "a",
          toNode: "b",
          "x-writers-studio": { relation: { typeId: "only-custom" } },
        },
      ],
    };
    expect(validateCanvasTypedDocument(valid, schema)).toEqual([]);

    const invalid: CanvasDocument = {
      nodes: [
        node("bad", {
          count: Number.NaN,
          enabled: "yes",
          date: "no",
          tags: [1],
          choice: "z",
          duration: -1,
          document: "",
        }),
        node("unknown", {}, "missing"),
        { id: "plain", type: "text", x: 0, y: 0, width: 100, height: 100, text: "plain" },
      ],
      edges: [
        {
          id: "unknown-relation",
          fromNode: "bad",
          toNode: "plain",
          "x-writers-studio": { relation: { typeId: "missing" } },
        },
        {
          id: "bad-target",
          fromNode: "bad",
          toNode: "plain",
          "x-writers-studio": { relation: { typeId: "only-custom" } },
        },
      ],
    };
    const issues = validateCanvasTypedDocument(invalid, schema);
    expect(issues.map((issue) => issue.id)).toContain("unknown");
    expect(issues.filter((issue) => issue.id === "bad")).toHaveLength(8);
    expect(issues.map((issue) => issue.id)).toContain("unknown-relation");
    expect(issues.map((issue) => issue.id)).toContain("bad-target");
  });
});
