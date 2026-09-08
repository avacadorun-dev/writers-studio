import { describe, expect, test } from "bun:test";

import type { AssistantChangeOperation } from "~entities/assistant";

import {
  annotationFixes,
  canvasGhostPreview,
  changeGroupLabel,
  exactTextEdits,
  operationLabel,
  resolvedTarget,
  reviewPrimaryAction,
  reviewStatusLabel,
  selectedChangeGroupDependents,
  snapshotText,
  withRequiredChangeGroups,
} from "./assistant-review-presentation";

function operation(overrides: Partial<AssistantChangeOperation> = {}): AssistantChangeOperation {
  return {
    id: "operation",
    position: 0,
    groupKey: "document:node",
    selected: true,
    dependencies: [],
    toolName: "document.replace_range",
    targetKind: "document",
    targetId: "node",
    baseRevision: 2,
    arguments: {},
    proposedSnapshot: {},
    riskLevel: "low",
    applied: false,
    ...overrides,
  };
}

describe("assistant review presentation", () => {
  test("derives one primary action and an author-facing status", () => {
    expect(reviewStatusLabel("staged")).toBe("Не применено");
    expect(reviewStatusLabel("applied")).toBe("Применено");
    expect(reviewStatusLabel("undone")).toBe("Отменено");
    expect(reviewPrimaryAction("staged", false)).toEqual({
      action: "apply",
      label: "Применить изменения",
    });
    expect(reviewPrimaryAction("staged", true).action).toBe("send-feedback");
    expect(reviewPrimaryAction("applied", false)).toEqual({
      action: "undo",
      label: "Отменить набор",
    });
    expect(reviewPrimaryAction("rejected", false).action).toBe("return");
  });
  test("uses author-facing operation and group labels", () => {
    expect(operationLabel("document.replace_range")).toBe("Точная правка текста");
    expect(operationLabel("document.append_chunk")).toBe("Большой фрагмент документа");
    expect(operationLabel("note.update")).toBe("Изменение документа");
    expect(operationLabel("annotation.propose_fixes")).toBe("Исправления по заметкам");
    expect(operationLabel("tree.unknown")).toBe("Изменение структуры");
    expect(operationLabel("dictionary.unknown")).toBe("Изменение словаря");
    expect(operationLabel("project_memory.unknown")).toBe("Изменение памяти проекта");
    expect(operationLabel("assistant_guide.unknown")).toBe("Изменение руководства");
    expect(operationLabel("private.unknown")).toBe("Предложенное изменение");
    expect(operationLabel("canvas.propose_patch")).toBe("Предложение для доски");

    expect(changeGroupLabel([operation({ groupKey: "dictionary" })], "dictionary")).toBe("Словарь");
    expect(
      changeGroupLabel([operation({ beforeSnapshot: { title: "Сцена у воды" } })], "document:node"),
    ).toBe("Текст · Сцена у воды");
    expect(
      changeGroupLabel([operation({ trace: { targetTitle: "Финал" } })], "document:node"),
    ).toBe("Текст · Финал");
    expect(changeGroupLabel([operation()], "document:node")).toBe("Отдельный документ");
    expect(changeGroupLabel([operation()], "custom")).toBe("Связанное изменение");
  });

  test("builds a Canvas ghost preview from immutable snapshots", () => {
    const preview = canvasGhostPreview(
      {
        nodes: [
          { id: "moved", type: "text", text: "До", x: 0, y: 0, width: 100, height: 80 },
          { id: "deleted", type: "text", text: "Удалить", x: 0, y: 100, width: 100, height: 80 },
        ],
        edges: [{ id: "old" }],
      },
      {
        nodes: [
          { id: "moved", type: "text", text: "До", x: 20, y: 0, width: 100, height: 80 },
          { id: "added", type: "text", text: "Новое", x: 150, y: 0, width: 100, height: 80 },
        ],
        edges: [{ id: "new" }],
      },
    );
    expect(preview?.changedNodes).toBe(3);
    expect(preview?.addedEdges).toBe(1);
    expect(preview?.deletedEdges).toBe(1);
    expect(preview?.nodes.map((node) => node.state)).toEqual(["moved", "added", "deleted"]);
    expect(canvasGhostPreview({}, {})).toBeUndefined();
  });

  test("shows note intent and resolution state for annotation fixes", () => {
    const value = operation({
      toolName: "annotation.propose_fixes",
      arguments: {
        annotationFixes: [
          {
            annotationId: "note-1",
            note: "Уточнить действие",
            originalText: "шёл",
            replacement: "торопился",
            resolve: true,
          },
          { annotationId: 3 },
        ],
      },
    });
    expect(annotationFixes(value)).toEqual([
      {
        annotationId: "note-1",
        note: "Уточнить действие",
        originalText: "шёл",
        replacement: "торопился",
        resolve: true,
      },
    ]);
    expect(annotationFixes(operation())).toEqual([]);
    expect(
      annotationFixes(
        operation({
          toolName: "annotation.propose_fixes",
          arguments: { annotationFixes: "invalid" },
        }),
      ),
    ).toEqual([]);
  });

  test("extracts exact range diffs without exposing transport details", () => {
    const value = operation({
      arguments: {
        edits: [
          {
            blockId: "block",
            start: 4,
            end: 10,
            expectedText: "старое",
            replacement: "новое",
          },
          { blockId: 1, start: "bad" },
        ],
      },
    });
    expect(exactTextEdits(value)).toEqual([
      {
        blockId: "block",
        start: 4,
        end: 10,
        before: "старое",
        after: "новое",
      },
    ]);
    expect(exactTextEdits(operation({ toolName: "document.append" }))).toEqual([]);
    expect(exactTextEdits(operation({ arguments: { edits: "bad" } }))).toEqual([]);
  });

  test("selects transitive prerequisites and protects required groups", () => {
    const operations = [
      operation({ id: "text", groupKey: "document:node" }),
      operation({ id: "dictionary", groupKey: "dictionary", dependencies: ["document:node"] }),
      operation({ id: "memory", groupKey: "memory", dependencies: ["dictionary"] }),
      operation({ id: "independent", groupKey: "guide" }),
    ];
    expect([...withRequiredChangeGroups(operations, ["memory"])]).toEqual([
      "memory",
      "dictionary",
      "document:node",
    ]);
    expect([...withRequiredChangeGroups(operations, ["missing", "guide"])]).toEqual(["guide"]);
    expect(
      selectedChangeGroupDependents(
        operations,
        "document:node",
        new Set(["document:node", "dictionary", "memory", "guide"]),
      ),
    ).toEqual(["dictionary", "memory"]);
    expect(selectedChangeGroupDependents(operations, "guide", new Set(["guide"]))).toEqual([]);
  });

  test("treats legacy null dependency lists as empty", () => {
    const legacy = operation({ dependencies: null as unknown as string[] });
    expect([...withRequiredChangeGroups([legacy], [legacy.groupKey])]).toEqual([legacy.groupKey]);
    expect(
      selectedChangeGroupDependents([legacy], legacy.groupKey, new Set([legacy.groupKey])),
    ).toEqual([]);
  });

  test("reads direct and wrapped resolved targets", () => {
    const target = {
      nodeId: "node",
      baseRevision: 2,
      ranges: [{ blockId: "block", start: 0, end: 4, reason: "фраза" }],
      protectedRanges: [],
      sourcePhrase: "измени фразу",
    };
    expect(resolvedTarget(operation({ trace: target }))).toEqual(target);
    expect(resolvedTarget(operation({ trace: { resolvedTarget: target } }))).toEqual(target);
    expect(resolvedTarget(operation({ trace: { nodeId: "node" } }))).toBeUndefined();
    expect(resolvedTarget(operation())).toBeUndefined();
  });

  test("renders structured snapshots with block and inline separators", () => {
    expect(snapshotText({ plainText: "готовый текст" })).toBe("готовый текст");
    expect(snapshotText({ statement: "факт" })).toBe("факт");
    expect(snapshotText({ canonical: "Элина" })).toBe("Элина");
    expect(snapshotText({ content: "руководство" })).toBe("руководство");
    expect(snapshotText({ content: { type: "text", text: "вложенный" } })).toBe("вложенный");
    expect(
      snapshotText({
        type: "doc",
        content: [
          {
            type: "paragraph",
            content: [
              { type: "text", text: "Первая " },
              { type: "text", text: "строка" },
            ],
          },
          { type: "paragraph", content: [{ type: "text", text: "Вторая" }] },
        ],
      }),
    ).toBe("Первая строка\n\nВторая");
    expect(
      snapshotText({
        type: "bulletList",
        content: [
          { type: "listItem", content: [{ type: "text", text: "Один" }] },
          { type: "listItem", content: [{ type: "text", text: "Два" }] },
        ],
      }),
    ).toBe("Один\nДва");
    expect(snapshotText(null)).toBe("");
    expect(snapshotText({ type: "paragraph" })).toBe("");
  });
});
