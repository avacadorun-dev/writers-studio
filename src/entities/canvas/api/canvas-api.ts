import { apiRequest } from "~shared/api";

import type {
  CanvasDocument,
  CanvasPromotion,
  CanvasResolvedLinkPreview,
  CanvasRevision,
  CanvasSummary,
  CanvasTypeSchema,
  CanvasWorkspace,
} from "../model/types";

export function getCanvasTypeSchema(projectId: string): Promise<{ schema: CanvasTypeSchema }> {
  return apiRequest(`/api/projects/${projectId}/canvas-schema`);
}

export function saveCanvasTypeSchema(
  projectId: string,
  schema: CanvasTypeSchema,
): Promise<{ schema: CanvasTypeSchema }> {
  return apiRequest(`/api/projects/${projectId}/canvas-schema`, {
    method: "PUT",
    body: JSON.stringify({
      baseRevision: schema.revision,
      cardTypes: schema.cardTypes,
      relationTypes: schema.relationTypes,
      source: "manual",
    }),
  });
}

export function promoteCanvasNode(
  canvasId: string,
  input: {
    baseRevision: number;
    nodeId: string;
    title: string;
    kind: string;
    parentId?: string;
  },
): Promise<CanvasPromotion> {
  return apiRequest(`/api/canvases/${canvasId}/promotions`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function fetchCanvasLinkPreviews(
  projectId: string,
  urls: string[],
): Promise<{ previews: CanvasResolvedLinkPreview[] }> {
  return apiRequest(`/api/projects/${projectId}/link-previews`, {
    method: "POST",
    body: JSON.stringify({ urls }),
  });
}

export function listCanvases(
  projectId: string,
  includeTrashed = true,
): Promise<{ canvases: CanvasSummary[] }> {
  return apiRequest(`/api/projects/${projectId}/canvases?includeTrashed=${String(includeTrashed)}`);
}

export function createCanvas(projectId: string, title: string): Promise<{ canvas: CanvasSummary }> {
  return apiRequest(`/api/projects/${projectId}/canvases`, {
    method: "POST",
    body: JSON.stringify({ title }),
  });
}

export function importCanvas(
  projectId: string,
  file: Blob,
  title = "Импортированная доска",
): Promise<{ canvas: CanvasSummary }> {
  return apiRequest(
    `/api/projects/${projectId}/canvases/import?title=${encodeURIComponent(title)}`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: file,
    },
  );
}

export function getCanvas(canvasId: string): Promise<CanvasWorkspace> {
  return apiRequest(`/api/canvases/${canvasId}`);
}

export function updateCanvas(
  canvasId: string,
  input: { title?: string; position?: number },
): Promise<{ canvas: CanvasSummary }> {
  return apiRequest(`/api/canvases/${canvasId}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  });
}

export function duplicateCanvas(canvasId: string): Promise<{ canvas: CanvasSummary }> {
  return apiRequest(`/api/canvases/${canvasId}/duplicate`, { method: "POST" });
}

export function trashCanvas(canvasId: string): Promise<{ canvas: CanvasSummary }> {
  return apiRequest(`/api/canvases/${canvasId}`, { method: "DELETE" });
}

export function restoreCanvas(canvasId: string): Promise<{ canvas: CanvasSummary }> {
  return apiRequest(`/api/canvases/${canvasId}/restore`, { method: "POST" });
}

export function saveCanvasRevision(
  canvasId: string,
  input: { baseRevision: number; content: CanvasDocument; source?: string },
): Promise<{ revision: CanvasRevision }> {
  return apiRequest(`/api/canvases/${canvasId}/revisions`, {
    method: "PUT",
    body: JSON.stringify(input),
  });
}
