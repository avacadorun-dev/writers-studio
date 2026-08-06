import type { ProjectMedia } from "~entities/project";

export type CanvasSide = "top" | "right" | "bottom" | "left";
export type CanvasEnd = "none" | "arrow";
export type CanvasColor = "1" | "2" | "3" | "4" | "5" | "6" | `#${string}`;

export interface CanvasResourceRef {
  kind: "document" | "media";
  id: string;
}

export interface CanvasNodeBase {
  id: string;
  type: "text" | "file" | "group";
  x: number;
  y: number;
  width: number;
  height: number;
  color?: CanvasColor;
  [key: string]: unknown;
}

export interface CanvasTextNode extends CanvasNodeBase {
  type: "text";
  text: string;
}

export interface CanvasFileNode extends CanvasNodeBase {
  type: "file";
  file: string;
  subpath?: string;
  "x-writers-studio": {
    resource: CanvasResourceRef;
    [key: string]: unknown;
  };
}

export interface CanvasGroupNode extends CanvasNodeBase {
  type: "group";
  label?: string;
  background?: string;
  backgroundStyle?: "cover" | "ratio" | "repeat";
}

export type CanvasNode = CanvasTextNode | CanvasFileNode | CanvasGroupNode;

export interface CanvasEdge {
  id: string;
  fromNode: string;
  fromSide?: CanvasSide;
  fromEnd?: CanvasEnd;
  toNode: string;
  toSide?: CanvasSide;
  toEnd?: CanvasEnd;
  color?: CanvasColor;
  label?: string;
  [key: string]: unknown;
}

export interface CanvasDocument {
  nodes: CanvasNode[];
  edges: CanvasEdge[];
  "x-writers-studio"?: {
    schemaVersion?: number;
    [key: string]: unknown;
  };
  [key: string]: unknown;
}

export interface CanvasSummary {
  id: string;
  projectId: string;
  title: string;
  position: number;
  currentRevision: number;
  trashedAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface CanvasRevision {
  id: string;
  canvasId: string;
  revision: number;
  baseRevision: number;
  schemaVersion: number;
  content: CanvasDocument;
  source: string;
  createdAt: string;
}

export interface CanvasDocumentPreview {
  id: string;
  title: string;
  kind: string;
  status: "idea" | "draft" | "editing" | "ready";
  currentRevision: number;
  plainText: string;
  trashedAt?: string;
}

export interface CanvasWorkspace {
  canvas: CanvasSummary;
  revision: CanvasRevision;
  documents: CanvasDocumentPreview[];
  media: ProjectMedia[];
}
