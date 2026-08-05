import { describe, expect, test } from "bun:test";

import type { DocumentNode, ProjectStructure } from "~entities/project";

import {
  buildProjectOutline,
  canMoveNodeToParent,
  canPlaceNodeAtParent,
  flattenOutlineIds,
  getValidParentNodes,
  moveNodeInOutline,
  nextNodePosition,
  siblingIndex,
} from "./project-outline";

const nodes = [
  node("chapter", "chapter", undefined, 1_000),
  node("draft", "draft", undefined, 2_000),
  node("nested-draft", "draft", "chapter", 1_000),
  node("scene", "scene", "chapter", 2_000),
  node("part", "part", undefined, 3_000),
] satisfies DocumentNode[];

const structure: ProjectStructure = {
  schemaVersion: 1,
  template: "manuscript",
  rootLabel: "Рукопись",
  nodeTypes: [
    {
      key: "part",
      label: "Часть",
      pluralLabel: "Части",
      icon: "folder",
      contentBearing: false,
      allowedChildren: ["chapter", "scene", "draft"],
      canBeRoot: true,
    },
    {
      key: "chapter",
      label: "Глава",
      pluralLabel: "Главы",
      icon: "book-open",
      contentBearing: true,
      allowedChildren: ["scene", "draft"],
      canBeRoot: true,
    },
    {
      key: "scene",
      label: "Сцена",
      pluralLabel: "Сцены",
      icon: "file-text",
      contentBearing: true,
      allowedChildren: [],
      canBeRoot: true,
    },
    {
      key: "draft",
      label: "Черновик",
      pluralLabel: "Черновики",
      icon: "notebook-pen",
      contentBearing: true,
      allowedChildren: [],
      canBeRoot: true,
    },
  ],
};

describe("project outline", () => {
  test("renders a draft dropped on a chapter inside the manuscript hierarchy", () => {
    const outline = buildProjectOutline(nodes);

    expect(outline.roots.map((item) => item.node.id)).toEqual(["chapter", "draft", "part"]);
    expect(outline.roots[0]?.children.map((item) => item.node.id)).toEqual([
      "nested-draft",
      "scene",
    ]);
  });

  test("offers only parts and chapters outside the moved node subtree", () => {
    const nested = [...nodes, node("nested-chapter", "chapter", "part", 1_000)];

    expect(getValidParentNodes(nested, "part", structure).map((item) => item.id)).toEqual([]);
    expect(canMoveNodeToParent(nested, "draft", "chapter", structure)).toBe(true);
    expect(canMoveNodeToParent(nested, "chapter", "scene", structure)).toBe(false);
    expect(canMoveNodeToParent(nested, "chapter", "chapter", structure)).toBe(false);
  });

  test("places a dropped node after existing siblings", () => {
    expect(nextNodePosition(nodes, "chapter")).toBe(3_000);
    expect(nextNodePosition(nodes)).toBe(4_000);
  });

  test("reorders siblings without changing their parent", () => {
    expect(canPlaceNodeAtParent(nodes, "draft", undefined, structure)).toBe(true);
    const moved = moveNodeInOutline(nodes, "draft", undefined, 0);
    expect(buildProjectOutline(moved).roots.map((item) => item.node.id)).toEqual([
      "draft",
      "chapter",
      "part",
    ]);
    expect(siblingIndex(moved, "draft")).toBe(0);
    expect(flattenOutlineIds(buildProjectOutline(moved).roots)).toEqual([
      "draft",
      "chapter",
      "nested-draft",
      "scene",
      "part",
    ]);
  });

  test("treats legacy null allowedChildren as an empty leaf collection", () => {
    const legacyStructure: ProjectStructure = {
      ...structure,
      nodeTypes: structure.nodeTypes.map((nodeType) =>
        nodeType.key === "scene"
          ? { ...nodeType, allowedChildren: null as unknown as string[] }
          : nodeType,
      ),
    };

    expect(getValidParentNodes(nodes, "draft", legacyStructure).map((item) => item.id)).toEqual([
      "chapter",
      "part",
    ]);
    expect(canMoveNodeToParent(nodes, "draft", "scene", legacyStructure)).toBe(false);
  });

  test("builds a 20,000-node outline without recursive ancestor copies", () => {
    const large = Array.from({ length: 20_000 }, (_, index) =>
      node(`document-${index}`, "draft", undefined, index * 1_000),
    );
    const outline = buildProjectOutline(large);
    expect(outline.roots).toHaveLength(20_000);
    expect(outline.roots[19_999]?.node.id).toBe("document-19999");
  });

  test("cuts malformed parent cycles while preserving their children", () => {
    const cyclic = [
      node("a", "chapter", "b", 1_000),
      node("b", "chapter", "a", 2_000),
      node("child", "draft", "a", 1_000),
    ];
    const outline = buildProjectOutline(cyclic);
    expect(outline.roots.map((item) => item.node.id)).toEqual(["a", "b"]);
    expect(outline.roots[0]?.children.map((item) => item.node.id)).toEqual(["child"]);
  });
});

function node(
  id: string,
  kind: DocumentNode["kind"],
  parentId: string | undefined,
  position: number,
): DocumentNode {
  return {
    id,
    projectId: "project",
    parentId,
    kind,
    title: id,
    status: "draft",
    position,
    currentRevision: 1,
    createdAt: "2026-07-30T00:00:00Z",
    updatedAt: "2026-07-30T00:00:00Z",
  };
}
