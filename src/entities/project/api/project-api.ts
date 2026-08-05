import { APIError, type APIErrorPayload, apiRequest } from "~shared/api";
import { API_BASE_URL } from "~shared/config";

import type {
  AnnotationAnchor,
  AnnotationAnchorUpdate,
  DictionaryTerm,
  DocumentAnnotation,
  DocumentNode,
  DocumentNodeKind,
  DocumentRevision,
  ExportJob,
  ExportProfile,
  MarkdownTreeImportResult,
  NamedVersion,
  Project,
  ProjectAssistantSettings,
  ProjectImportResult,
  ProjectKind,
  ProjectMedia,
  ProjectReplacePreview,
  ProjectReplaceResult,
  ProjectStatus,
  ProjectStructure,
  ProjectWorkspace,
  SearchResult,
  TiptapNode,
} from "../model/types";

export interface CreateProjectInput {
  title: string;
  kind: ProjectKind;
  language: "ru" | "en";
  synopsis: string;
  wordGoal?: number;
}

export interface UpdateProjectInput {
  title: string;
  language: "ru" | "en";
  synopsis: string;
  wordGoal?: number;
}

export function listProjects(status: ProjectStatus = "active"): Promise<{ projects: Project[] }> {
  return apiRequest(`/api/projects?status=${status}`);
}

export function createProject(input: CreateProjectInput): Promise<ProjectWorkspace> {
  return apiRequest("/api/projects", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function importProjectFile(
  file: File,
): Promise<{ project: ProjectImportResult; notice: string }> {
  return apiRequest("/api/projects/import", {
    method: "POST",
    headers: { "Content-Type": "application/vnd.writers-studio.project+zip" },
    body: file,
  });
}

export function importMarkdownTree(
  form: FormData,
  onProgress?: (progress: { phase: "upload" | "processing"; percent?: number }) => void,
): Promise<MarkdownTreeImportResult> {
  return new Promise((resolve, reject) => {
    const request = new XMLHttpRequest();
    request.open("POST", `${API_BASE_URL}/api/projects/import/markdown-tree`);
    request.withCredentials = true;
    request.upload.addEventListener("progress", (event) => {
      onProgress?.({
        phase: "upload",
        percent: event.lengthComputable
          ? Math.round((event.loaded / event.total) * 100)
          : undefined,
      });
    });
    request.upload.addEventListener("load", () => onProgress?.({ phase: "processing" }));
    request.addEventListener("load", () => {
      let payload: unknown;
      try {
        payload = JSON.parse(request.responseText);
      } catch {
        payload = {};
      }
      if (request.status >= 200 && request.status < 300) {
        resolve(payload as MarkdownTreeImportResult);
      } else {
        reject(new APIError(request.status, payload as APIErrorPayload));
      }
    });
    request.addEventListener("error", () =>
      reject(
        new APIError(0, {
          error: { code: "network_error", message: "Не удалось загрузить Markdown-хранилище" },
        }),
      ),
    );
    onProgress?.({ phase: "upload", percent: 0 });
    request.send(form);
  });
}

export function getProject(projectId: string): Promise<ProjectWorkspace> {
  return apiRequest(`/api/projects/${projectId}/outline`);
}

export function getProjectOutline(projectId: string): Promise<ProjectWorkspace> {
  return getProject(projectId);
}

export function updateProject(
  projectId: string,
  input: UpdateProjectInput,
): Promise<{ project: Project }> {
  return apiRequest(`/api/projects/${projectId}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  });
}

export function getProjectAssistantSettings(
  projectId: string,
): Promise<{ settings: ProjectAssistantSettings }> {
  return apiRequest(`/api/projects/${projectId}/assistant-settings`);
}

export function updateProjectAssistantSettings(
  projectId: string,
  settings: ProjectAssistantSettings,
): Promise<{ settings: ProjectAssistantSettings }> {
  return apiRequest(`/api/projects/${projectId}/assistant-settings`, {
    method: "PUT",
    body: JSON.stringify({ stepLimit: settings.stepLimit ?? null }),
  });
}

export function getProjectStructure(
  projectId: string,
): Promise<{ structure: ProjectStructure; revision: number }> {
  return apiRequest(`/api/projects/${projectId}/structure`);
}

export function saveProjectStructure(
  projectId: string,
  input: { baseRevision: number; structure: ProjectStructure },
): Promise<{ structure: ProjectStructure; revision: number }> {
  return apiRequest(`/api/projects/${projectId}/structure`, {
    method: "PUT",
    body: JSON.stringify(input),
  });
}

export function setProjectStatus(
  projectId: string,
  status: ProjectStatus,
): Promise<{ project: Project }> {
  return apiRequest(`/api/projects/${projectId}/status`, {
    method: "PATCH",
    body: JSON.stringify({ status }),
  });
}

export function purgeProject(
  projectId: string,
  confirmation: string,
): Promise<{ purged: boolean; notice: string }> {
  return apiRequest(`/api/projects/${projectId}/purge`, {
    method: "DELETE",
    body: JSON.stringify({ confirmation }),
  });
}

export function createNode(
  projectId: string,
  input: { parentId?: string; kind: DocumentNodeKind; title: string },
): Promise<{ node: DocumentNode }> {
  return apiRequest(`/api/projects/${projectId}/nodes`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function getNode(nodeId: string): Promise<{ node: DocumentNode }> {
  return apiRequest(`/api/nodes/${nodeId}`);
}

export function uploadProjectImage(
  projectId: string,
  file: File,
): Promise<{ media: ProjectMedia }> {
  return apiRequest(`/api/projects/${projectId}/media`, {
    method: "POST",
    headers: {
      "Content-Type": file.type,
      "X-File-Name": encodeURIComponent(file.name),
    },
    body: file,
  });
}

export function listProjectMedia(projectId: string): Promise<{ media: ProjectMedia[] }> {
  return apiRequest(`/api/projects/${projectId}/media`);
}

export function getExportProfile(
  projectId: string,
): Promise<{ profile: ExportProfile; revision: number }> {
  return apiRequest(`/api/projects/${projectId}/export-profile`);
}

export function saveExportProfile(
  projectId: string,
  input: { baseRevision: number; profile: ExportProfile },
): Promise<{ profile: ExportProfile; revision: number }> {
  return apiRequest(`/api/projects/${projectId}/export-profile`, {
    method: "PUT",
    body: JSON.stringify(input),
  });
}

export function updateNode(
  nodeId: string,
  input: { title?: string; status?: DocumentNode["status"] },
): Promise<{ node: DocumentNode }> {
  return apiRequest(`/api/nodes/${nodeId}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  });
}

export function duplicateNode(nodeId: string): Promise<{ node: DocumentNode }> {
  return apiRequest(`/api/nodes/${nodeId}/duplicate`, { method: "POST" });
}

export function moveNode(
  nodeId: string,
  input: { parentId?: string; position?: number; targetIndex?: number },
): Promise<{ operationId: string; undoable: boolean }> {
  return apiRequest(`/api/nodes/${nodeId}/move`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function trashNode(nodeId: string): Promise<{ operationId: string; undoable: boolean }> {
  return apiRequest(`/api/nodes/${nodeId}`, { method: "DELETE" });
}

export function restoreNode(nodeId: string): Promise<{ restored: boolean }> {
  return apiRequest(`/api/nodes/${nodeId}/restore`, { method: "POST" });
}

export function saveRevision(
  nodeId: string,
  input: {
    baseRevision: number;
    content: TiptapNode;
    source?: string;
    operationId?: string;
    annotationAnchors?: AnnotationAnchorUpdate[];
  },
): Promise<{ revision: DocumentRevision }> {
  return apiRequest(`/api/nodes/${nodeId}/revisions`, {
    method: "PUT",
    body: JSON.stringify(input),
  });
}

export function listProjectAnnotations(
  projectId: string,
): Promise<{ annotations: DocumentAnnotation[] }> {
  return apiRequest(`/api/projects/${projectId}/annotations`);
}

export function listDocumentAnnotations(
  nodeId: string,
): Promise<{ annotations: DocumentAnnotation[] }> {
  return apiRequest(`/api/nodes/${nodeId}/annotations`);
}

export function createDocumentAnnotation(
  nodeId: string,
  input: {
    kind: DocumentAnnotation["kind"];
    dictionaryTermId?: string;
    anchor: AnnotationAnchor;
    note?: string;
    baseRevision: number;
  },
): Promise<{ annotation: DocumentAnnotation }> {
  return apiRequest(`/api/nodes/${nodeId}/annotations`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function updateDocumentAnnotation(
  annotationId: string,
  input: { note?: string; status?: DocumentAnnotation["status"]; anchor?: AnnotationAnchor },
): Promise<{ annotation: DocumentAnnotation }> {
  return apiRequest(`/api/annotations/${annotationId}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  });
}

export function deleteDocumentAnnotation(annotationId: string): Promise<void> {
  return apiRequest(`/api/annotations/${annotationId}`, { method: "DELETE" });
}

export function getVersions(
  nodeId: string,
): Promise<{ versions: NamedVersion[]; revisions: DocumentRevision[] }> {
  return apiRequest(`/api/nodes/${nodeId}/versions`);
}

export function createVersion(
  nodeId: string,
  input: { name: string; note: string },
): Promise<{ version: NamedVersion }> {
  return apiRequest(`/api/nodes/${nodeId}/versions`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function restoreVersion(versionId: string): Promise<{
  revision: DocumentRevision;
  undoable: boolean;
}> {
  return apiRequest(`/api/versions/${versionId}/restore`, { method: "POST" });
}

export function searchProject(
  projectId: string,
  query: string,
  scope = "all",
): Promise<{ results: SearchResult[] }> {
  const params = new URLSearchParams({ q: query, scope });
  return apiRequest(`/api/projects/${projectId}/search?${params.toString()}`);
}

export function previewProjectReplace(
  projectId: string,
  input: { search: string; replace: string; scope: string },
): Promise<{ preview: ProjectReplacePreview }> {
  return apiRequest(`/api/projects/${projectId}/replace/preview`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function applyProjectReplace(
  projectId: string,
  input: { search: string; replace: string; scope: string; previewToken: string },
): Promise<{ result: ProjectReplaceResult; undoable: boolean }> {
  return apiRequest(`/api/projects/${projectId}/replace/apply`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function undoProjectReplace(
  projectId: string,
  operationId: string,
): Promise<{ result: ProjectReplaceResult }> {
  return apiRequest(`/api/projects/${projectId}/replace/${operationId}/undo`, {
    method: "POST",
  });
}

export function listDictionary(projectId: string): Promise<{ terms: DictionaryTerm[] }> {
  return apiRequest(`/api/projects/${projectId}/dictionary`);
}

export function createDictionaryTerm(
  projectId: string,
  input: Omit<DictionaryTerm, "id" | "projectId" | "active" | "createdAt" | "updatedAt">,
): Promise<{ term: DictionaryTerm; notice: string }> {
  return apiRequest(`/api/projects/${projectId}/dictionary`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function updateDictionaryTerm(
  termId: string,
  input: Pick<DictionaryTerm, "canonical" | "variants" | "kind" | "language" | "active">,
): Promise<{ term: DictionaryTerm }> {
  return apiRequest(`/api/dictionary/${termId}`, {
    method: "PUT",
    body: JSON.stringify(input),
  });
}

export function deleteDictionaryTerm(
  termId: string,
): Promise<{ operationId: string; undoable: boolean }> {
  return apiRequest(`/api/dictionary/${termId}`, { method: "DELETE" });
}

export function createExport(
  projectId: string,
  input: {
    format: ExportJob["format"];
    nodeIds: string[];
    titlePage: boolean;
    chapterTitles: boolean;
    includeNotes: boolean;
    profile: ExportProfile;
  },
): Promise<{ export: ExportJob }> {
  return apiRequest(`/api/projects/${projectId}/exports`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function createMarkdownTreeExport(projectId: string): Promise<{ export: ExportJob }> {
  return apiRequest(`/api/projects/${projectId}/exports`, {
    method: "POST",
    body: JSON.stringify({ format: "markdown_tree" }),
  });
}

export function getExport(exportId: string): Promise<{ export: ExportJob }> {
  return apiRequest(`/api/exports/${exportId}`);
}
