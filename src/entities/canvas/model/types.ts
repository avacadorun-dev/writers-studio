import type { DocumentNode, ProjectMedia } from "~entities/project";

export type CanvasSide = "top" | "right" | "bottom" | "left";
export type CanvasEnd = "none" | "arrow";
export type CanvasColor = "1" | "2" | "3" | "4" | "5" | "6" | `#${string}`;
export type CanvasElementColor = CanvasColor | "transparent";
export type CanvasStrokePoint = [x: number, y: number, pressure?: number];

export interface CanvasResourceRef {
  kind: "document" | "media" | "canvas";
  id: string;
}

export interface CanvasStrokeElement {
  kind: "stroke";
  points: CanvasStrokePoint[];
  brush: "pen" | "highlighter";
  color: CanvasColor;
  size: number;
  opacity: number;
}

export interface CanvasShapeElement {
  kind: "shape";
  shape: "rectangle" | "ellipse" | "diamond" | "line" | "arrow";
  color: CanvasColor;
  fill: CanvasElementColor;
  size: number;
  opacity: number;
  rotation: number;
  dash: "solid" | "dashed" | "dotted";
  label?: string;
}

export interface CanvasFrameElement {
  kind: "frame";
  order: number;
  label?: string;
}

export type CanvasElementExtension = CanvasStrokeElement | CanvasShapeElement | CanvasFrameElement;

export interface CanvasCardMetadata {
  typeId: string;
  fields: Record<string, unknown>;
  promotedDocumentId?: string;
}

export interface CanvasRelationMetadata {
  typeId: string;
  fields?: Record<string, unknown>;
}

export type CanvasFieldType =
  | "short-text"
  | "markdown"
  | "number"
  | "checkbox"
  | "single-select"
  | "multi-select"
  | "date"
  | "duration"
  | "document"
  | "media"
  | "canvas";

export interface CanvasFieldDefinition {
  id: string;
  name: string;
  type: CanvasFieldType;
  required?: boolean;
  options?: string[];
}

export interface CanvasTypeDefinition {
  id: string;
  name: string;
  category: "general" | "narrative" | "game-design" | "custom";
  color: CanvasColor;
  icon: string;
  fields: CanvasFieldDefinition[];
  allowedRelations?: string[];
  documentKind?: string;
}

export interface CanvasRelationType {
  id: string;
  name: string;
  color: CanvasColor;
  directed: boolean;
  fromTypes?: string[];
  toTypes?: string[];
}

export interface CanvasTypeSchema {
  revision: number;
  cardTypes: CanvasTypeDefinition[];
  relationTypes: CanvasRelationType[];
}

export interface CanvasLinkPreview {
  title?: string;
  description?: string;
  siteName?: string;
  fetchedAt?: string;
}

export interface CanvasResolvedLinkPreview extends CanvasLinkPreview {
  url: string;
}

export interface CanvasNodeExtension {
  resource?: CanvasResourceRef;
  element?: CanvasElementExtension;
  card?: CanvasCardMetadata;
  preview?: CanvasLinkPreview;
  locked?: boolean;
  [key: string]: unknown;
}

export interface CanvasNodeBase {
  id: string;
  type: "text" | "file" | "link" | "group";
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
  "x-writers-studio"?: CanvasNodeExtension;
}

export interface CanvasFileNode extends CanvasNodeBase {
  type: "file";
  file: string;
  subpath?: string;
  "x-writers-studio"?: CanvasNodeExtension & { resource?: CanvasResourceRef };
}

export interface CanvasLinkNode extends CanvasNodeBase {
  type: "link";
  url: string;
  "x-writers-studio"?: CanvasNodeExtension;
}

export interface CanvasGroupNode extends CanvasNodeBase {
  type: "group";
  label?: string;
  background?: string;
  backgroundStyle?: "cover" | "ratio" | "repeat";
  "x-writers-studio"?: CanvasNodeExtension;
}

export type CanvasNode = CanvasTextNode | CanvasFileNode | CanvasLinkNode | CanvasGroupNode;

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
  "x-writers-studio"?: {
    relation?: CanvasRelationMetadata;
    [key: string]: unknown;
  };
  [key: string]: unknown;
}

export interface CanvasDocument {
  nodes: CanvasNode[];
  edges: CanvasEdge[];
  "x-writers-studio"?: {
    schemaVersion?: 1 | 2;
    activeView?: "free" | "mind-map" | "timeline" | "swimlanes" | "dependency" | "filtered";
    viewFilterTypeId?: string;
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

export interface CanvasPromotion {
  document: DocumentNode;
  revision: CanvasRevision;
}
