import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";

import type { DocumentNode, Project } from "~entities/project";
import { cn } from "~shared/lib";
import { type SaveState, SaveStatus } from "~shared/ui";
import { ProjectTree } from "~widgets/project-tree";
import { RequireSession } from "~widgets/require-session";
import { StudioHeader } from "~widgets/studio-header";

interface ProjectUtilityShellProps {
  project: Project;
  nodes: DocumentNode[];
  activeNodeId?: string;
  expandedTree?: boolean;
  saveState?: SaveState;
  mobileSaveStatus?: boolean;
  children: React.ReactNode;
  contentClassName?: string;
}

export function ProjectUtilityShell({
  project,
  nodes,
  activeNodeId,
  expandedTree = false,
  saveState = "saved",
  mobileSaveStatus = false,
  children,
  contentClassName,
}: ProjectUtilityShellProps) {
  const navigate = useNavigate();
  const firstNode = nodes.find((node) => !node.trashedAt);
  const activeNode = nodes.find((node) => node.id === activeNodeId && !node.trashedAt) ?? firstNode;
  const [treeCollapsed, setTreeCollapsed] = useState(!expandedTree);

  if (!activeNode) {
    return (
      <RequireSession>
        <main className="grid min-h-full place-items-center p-6 text-sm text-[var(--ink-secondary)]">
          Сначала создайте документ в проекте.
        </main>
      </RequireSession>
    );
  }

  return (
    <RequireSession>
      <div className="flex h-full min-h-0 flex-col bg-[var(--surface-app)]">
        <StudioHeader
          project={project}
          nodeId={activeNode.id}
          nodeTitle={activeNode.title}
          saveState={saveState}
          onDictate={() => {
            void navigate({
              to: "/projects/$projectId/studio/$nodeId",
              params: { projectId: project.id, nodeId: activeNode.id },
              search: { dictate: true },
            });
          }}
          onAssistant={() => {
            void navigate({
              to: "/projects/$projectId/studio/$nodeId",
              params: { projectId: project.id, nodeId: activeNode.id },
              search: { assistant: true },
            });
          }}
        />
        {mobileSaveStatus ? (
          <div className="flex min-h-10 items-center border-b border-[var(--border-subtle)] bg-[var(--surface-panel)] px-3 md:hidden">
            <SaveStatus state={saveState} />
          </div>
        ) : null}
        <div className="flex min-h-0 flex-1">
          <ProjectTree
            projectId={project.id}
            activeNodeId={activeNode.id}
            nodes={nodes}
            structure={project.structure}
            collapsed={treeCollapsed}
            onToggle={() => setTreeCollapsed((current) => !current)}
          />
          <main className={cn("min-w-0 flex-1 overflow-y-auto", contentClassName)}>{children}</main>
        </div>
      </div>
    </RequireSession>
  );
}
