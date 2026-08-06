import { apiRequest } from "~shared/api";

import type {
  CanvasDocument,
  CanvasRevision,
  CanvasSummary,
  CanvasWorkspace,
} from "../model/types";

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
