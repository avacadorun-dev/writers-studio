import { describe, expect, test } from "bun:test";

import type { DocumentNode, ProjectWorkspace } from "~entities/project";

import { selectProjectLandingNode, selectProjectRootKind } from "./open-project-page";

function node(id: string, trashedAt?: string): DocumentNode {
  return { id, trashedAt } as DocumentNode;
}

describe("project landing", () => {
  test("opens the first active material and ignores the trash", () => {
    expect(
      selectProjectLandingNode([
        node("trashed", "2026-08-07T00:00:00Z"),
        node("active"),
        node("later"),
      ])?.id,
    ).toBe("active");
    expect(selectProjectLandingNode([node("trashed", "2026-08-07T00:00:00Z")])).toBeUndefined();
  });

  test("chooses a content-bearing root type before structural fallbacks", () => {
    const workspace = {
      project: {
        structure: {
          nodeTypes: [
            { key: "part", canBeRoot: true, contentBearing: false },
            { key: "document", canBeRoot: true, contentBearing: true },
          ],
        },
      },
    } as ProjectWorkspace;
    expect(selectProjectRootKind(workspace)).toBe("document");

    workspace.project.structure.nodeTypes = [
      {
        key: "part",
        label: "Часть",
        pluralLabel: "Части",
        icon: "folder",
        allowedChildren: [],
        canBeRoot: true,
        contentBearing: false,
      },
    ];
    expect(selectProjectRootKind(workspace)).toBe("part");
    workspace.project.structure.nodeTypes = [];
    expect(selectProjectRootKind(workspace)).toBeUndefined();
  });
});
