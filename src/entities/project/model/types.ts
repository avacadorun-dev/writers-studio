export type ProjectKind =
  | "novel"
  | "story"
  | "screenplay"
  | "nonfiction"
  | "game_design"
  | "free"
  | "custom";
export type ProjectStatus = "active" | "archived" | "trash";
export type DocumentNodeKind = string;

export interface ProjectNodeType {
  key: string;
  label: string;
  pluralLabel: string;
  icon: string;
  contentBearing: boolean;
  allowedChildren: string[];
  canBeRoot: boolean;
}

export interface ProjectStructure {
  schemaVersion: number;
  template: string;
  rootLabel: string;
  nodeTypes: ProjectNodeType[];
}

export interface Project {
  id: string;
  title: string;
  kind: ProjectKind;
  language: "ru" | "en";
  synopsis: string;
  wordGoal?: number;
  status: ProjectStatus;
  structureRevision: number;
  structure: ProjectStructure;
  trashedAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface TiptapNode {
  type: string;
  attrs?: Record<string, unknown>;
  marks?: Array<{
    type: string;
    attrs?: Record<string, unknown>;
  }>;
  text?: string;
  content?: TiptapNode[];
}

export interface DocumentNode {
  id: string;
  projectId: string;
  parentId?: string;
  kind: DocumentNodeKind;
  title: string;
  status: "idea" | "draft" | "editing" | "ready";
  position: number;
  currentRevision: number;
  content?: TiptapNode;
  plainText?: string;
  trashedAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface ProjectMedia {
  id: string;
  projectId: string;
  mediaType: "image/png" | "image/jpeg" | "image/gif";
  originalName: string;
  sourcePath?: string;
  byteSize: number;
  width: number;
  height: number;
  sha256: string;
  contentUrl: string;
  createdAt: string;
}

export type ExportFormat = "docx" | "epub" | "pdf" | "markdown" | "markdown_tree" | "txt";
export type ExportPreset = "book" | "modern" | "manuscript" | "custom";
export type ExportFontFamily = "literata" | "source-serif" | "inter" | "ibm-plex-mono";

export interface ExportProfile {
  schemaVersion: 1;
  metadata: {
    title: string;
    subtitle: string;
    author: string;
    annotation: string;
    coverAssetId?: string;
  };
  style: {
    preset: ExportPreset;
    fontFamily: ExportFontFamily;
    fontSizePt: number;
    lineHeight: number;
    paragraphMode: "indent" | "spaced";
    firstLineIndentEm: number;
    paragraphSpaceEm: number;
    textAlign: "left" | "justify";
    pageSize: "a4" | "a5" | "letter";
    marginMm: number;
    sceneSeparator: "asterisks" | "dots" | "line";
  };
  titlePage: boolean;
  tableOfContents: boolean;
  chapterTitles: boolean;
  chapterPageBreak: boolean;
  includeNotes: boolean;
  lastFormat: ExportFormat;
}

export interface ProjectWorkspace {
  project: Project;
  nodes: DocumentNode[];
}

export interface ProjectAssistantSettings {
  stepLimit?: number;
}

export interface ProjectImportResult {
  projectId: string;
  firstNodeId: string;
  title: string;
}

export interface MarkdownTreeImportWarning {
  path?: string;
  code: string;
  message: string;
}

export interface MarkdownTreeImportReport {
  directories: number;
  documents: number;
  images: number;
  resolvedLinks: number;
  skipped: number;
  warningsCount: number;
  warnings: MarkdownTreeImportWarning[];
}

export interface MarkdownTreeImportResult {
  project: ProjectImportResult;
  report: MarkdownTreeImportReport;
  notice: string;
}

export interface DocumentRevision {
  id: string;
  nodeId: string;
  revision: number;
  baseRevision: number;
  schemaVersion: number;
  content: TiptapNode;
  plainText: string;
  source: string;
  operationId?: string;
  createdAt: string;
}

export interface NamedVersion {
  id: string;
  nodeId: string;
  revision: number;
  name: string;
  note: string;
  createdAt: string;
}

export interface SearchResult {
  nodeId: string;
  nodeKind: DocumentNodeKind;
  breadcrumb: string;
  snippet: string;
  matchCount: number;
  revision: number;
  sourceScope: string;
}

export interface ProjectReplaceDocument {
  nodeId: string;
  title: string;
  revision: number;
  matchCount: number;
  snippet: string;
  replacement: string;
}

export interface ProjectReplacePreview {
  token: string;
  search: string;
  replace: string;
  scope: string;
  totalCount: number;
  documents: ProjectReplaceDocument[];
}

export interface ProjectReplaceResult {
  operationId: string;
  documents: number;
  replacements: number;
}

export interface DictionaryTerm {
  id: string;
  projectId: string;
  canonical: string;
  variants: string[];
  kind: "character" | "place" | "term" | "other";
  language: "ru" | "en";
  pronunciation?: string;
  revision?: number;
  source?: string;
  active: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface AnnotationAnchor {
  blockId: string;
  start: number;
  end: number;
  exactText: string;
  prefix: string;
  suffix: string;
}

export interface AnnotationTerm {
  id?: string;
  canonical: string;
  kind: DictionaryTerm["kind"];
  language: "ru" | "en";
  active: boolean;
}

export interface DocumentAnnotation {
  id: string;
  projectId: string;
  nodeId: string;
  nodeTitle: string;
  kind: "dictionary_link" | "note";
  status: "open" | "resolved" | "needs_attention";
  anchorStatus?: "attached" | "detached";
  dictionaryTermId?: string;
  dictionaryTerm?: AnnotationTerm;
  anchor: AnnotationAnchor;
  note?: string;
  baseRevision: number;
  revision?: number;
  resolvedAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface AnnotationAnchorUpdate {
  id: string;
  anchor: AnnotationAnchor;
  detached?: boolean;
}

export interface ExportJob {
  id: string;
  projectId: string;
  format: ExportFormat;
  status: "queued" | "running" | "ready" | "failed";
  downloadUrl?: string;
  errorCode?: string;
  createdAt: string;
  updatedAt: string;
}
