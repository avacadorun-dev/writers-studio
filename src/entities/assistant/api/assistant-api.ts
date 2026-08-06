import { apiRequest, webSocketURL } from "~shared/api";
import { projectMemoryUpdatePayload } from "../model/project-memory";
import type {
  AssistantCapabilities,
  AssistantChangeSet,
  AssistantGuide,
  AssistantInputMode,
  AssistantRun,
  AssistantSelectionAttachment,
  AssistantTargetKind,
  AssistantTurn,
  AssistantWriterPreferences,
  ProjectMemoryRecord,
  ProjectMemoryRevision,
  ProviderConnection,
  ProviderRef,
} from "../model/types";

export function getAssistantCapabilities(): Promise<AssistantCapabilities> {
  return apiRequest("/api/assistant/capabilities");
}

export function createAssistantRun(
  projectId: string,
  input: {
    title?: string;
    targetKind: AssistantTargetKind;
    targetNodeId?: string;
    targetCanvasId?: string;
    canvasSelectionIds?: string[];
    canvasViewport?: { x: number; y: number; zoom: number };
    inputMode: AssistantInputMode;
    silenceCheckpointMs: number;
    providerConnectionId?: string;
    writerPreferences: AssistantWriterPreferences;
    initialInstruction?: string;
    execute?: boolean;
    selectionAttachment?: AssistantSelectionAttachment;
  },
): Promise<{ run: AssistantRun; turn?: AssistantTurn }> {
  return apiRequest(`/api/projects/${projectId}/assistant/runs`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function listAssistantRuns(projectId: string): Promise<{ runs: AssistantRun[] }> {
  return apiRequest(`/api/projects/${projectId}/assistant/runs`);
}

export function getAssistantRun(runId: string): Promise<{ run: AssistantRun }> {
  return apiRequest(`/api/assistant/runs/${runId}`);
}

export function markAssistantRunViewed(runId: string): Promise<{ run: AssistantRun }> {
  return apiRequest(`/api/assistant/runs/${runId}/view`, { method: "POST" });
}

export function deleteAssistantRunHistory(runId: string): Promise<{ deleted: boolean }> {
  return apiRequest(`/api/assistant/runs/${runId}`, { method: "DELETE" });
}

export function listAssistantTurns(runId: string): Promise<{ turns: AssistantTurn[] }> {
  return apiRequest(`/api/assistant/runs/${runId}/turns`);
}

export function createAssistantTurn(
  runId: string,
  input: {
    instruction: string;
    inputMode: AssistantInputMode;
    requireConfirmation: boolean;
    execute: boolean;
    selectionAttachment?: AssistantSelectionAttachment;
  },
): Promise<{ turn: AssistantTurn }> {
  return apiRequest(`/api/assistant/runs/${runId}/turns`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function markAssistantChangeSetStale(
  runId: string,
  changeSetId: string,
): Promise<{ changeSet: AssistantChangeSet }> {
  return apiRequest(`/api/assistant/runs/${runId}/change-sets/${changeSetId}/stale`, {
    method: "POST",
  });
}

export function confirmAssistantTurn(
  runId: string,
  turnId: string,
  instruction: string,
  execute = true,
): Promise<{ turn: AssistantTurn }> {
  return apiRequest(`/api/assistant/runs/${runId}/turns/${turnId}`, {
    method: "PUT",
    body: JSON.stringify({ instruction, execute }),
  });
}

export function executeAssistantTurn(runId: string, turnId: string): Promise<{ started: boolean }> {
  return apiRequest(`/api/assistant/runs/${runId}/turns/${turnId}/execute`, {
    method: "POST",
  });
}

export function stopAssistantRun(runId: string): Promise<{ stopped: boolean }> {
  return apiRequest(`/api/assistant/runs/${runId}/stop`, { method: "POST" });
}

export function applyAssistantChangeSet(
  runId: string,
  changeSetId: string,
  selectedGroups: string[],
  annotationResolutions?: Record<string, boolean>,
): Promise<{ changeSet: AssistantChangeSet }> {
  return apiRequest(`/api/assistant/runs/${runId}/change-sets/${changeSetId}/apply`, {
    method: "POST",
    body: JSON.stringify({ selectedGroups, annotationResolutions: annotationResolutions ?? {} }),
  });
}

export function getAssistantChangeSet(
  runId: string,
  changeSetId: string,
): Promise<{ changeSet: AssistantChangeSet }> {
  return apiRequest(`/api/assistant/runs/${runId}/change-sets/${changeSetId}`);
}

export function rejectAssistantChangeSet(
  runId: string,
  changeSetId: string,
): Promise<{ changeSet: AssistantChangeSet }> {
  return apiRequest(`/api/assistant/runs/${runId}/change-sets/${changeSetId}/reject`, {
    method: "POST",
  });
}

export function undoAssistantChangeSet(
  runId: string,
  changeSetId: string,
): Promise<{ changeSet: AssistantChangeSet }> {
  return apiRequest(`/api/assistant/runs/${runId}/change-sets/${changeSetId}/undo`, {
    method: "POST",
  });
}

export function assistantVoiceURL(runId: string): string {
  return webSocketURL(`/api/assistant/runs/${runId}/voice`);
}

export function assistantEventsURL(runId: string, after?: number): string {
  const path = `/api/assistant/runs/${runId}/events`;
  return after && after > 0 ? `${path}?after=${after}` : path;
}

export function getAssistantGuide(
  projectId: string,
): Promise<{ exists: boolean; guide?: AssistantGuide }> {
  return apiRequest(`/api/projects/${projectId}/assistant-guide`);
}

export function saveAssistantGuide(
  projectId: string,
  content: string,
  baseRevision?: number,
): Promise<{ guide: AssistantGuide }> {
  return apiRequest(`/api/projects/${projectId}/assistant-guide`, {
    method: "PUT",
    body: JSON.stringify({ content, baseRevision }),
  });
}

export function restoreAssistantGuide(
  projectId: string,
  revision: number,
): Promise<{ guide: AssistantGuide }> {
  return apiRequest(`/api/projects/${projectId}/assistant-guide/revisions/${revision}/restore`, {
    method: "POST",
  });
}

export function listProjectMemory(
  projectId: string,
  includeInactive = false,
): Promise<{ records: ProjectMemoryRecord[] }> {
  return apiRequest(`/api/projects/${projectId}/memory?includeInactive=${String(includeInactive)}`);
}

export function listProjectMemoryRevisions(
  projectId: string,
  memoryId: string,
): Promise<{ revisions: ProjectMemoryRevision[] }> {
  return apiRequest(`/api/projects/${projectId}/memory/${memoryId}/revisions`);
}

export function restoreProjectMemoryRevision(
  projectId: string,
  memoryId: string,
  revision: number,
): Promise<{ record: ProjectMemoryRecord }> {
  return apiRequest(`/api/projects/${projectId}/memory/${memoryId}/revisions/${revision}/restore`, {
    method: "POST",
  });
}

export function saveProjectMemory(
  projectId: string,
  input: Omit<
    ProjectMemoryRecord,
    "id" | "projectId" | "revision" | "source" | "createdAt" | "updatedAt"
  >,
): Promise<{ record: ProjectMemoryRecord }> {
  return apiRequest(`/api/projects/${projectId}/memory`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function updateProjectMemory(
  projectId: string,
  record: ProjectMemoryRecord,
): Promise<{ record: ProjectMemoryRecord }> {
  return apiRequest(`/api/projects/${projectId}/memory/${record.id}`, {
    method: "PUT",
    body: JSON.stringify(projectMemoryUpdatePayload(record)),
  });
}

export function deactivateProjectMemory(
  projectId: string,
  memoryId: string,
): Promise<{ record: ProjectMemoryRecord }> {
  return apiRequest(`/api/projects/${projectId}/memory/${memoryId}`, { method: "DELETE" });
}

export function listAssistantProviders(): Promise<{
  system?: ProviderRef;
  systemStatus: "unchecked" | "compatible" | "limited" | "unavailable" | "";
  systemStatusDetail: string;
  connections: ProviderConnection[];
  connectionsEnabled: boolean;
}> {
  return apiRequest("/api/assistant/providers");
}

export function probeSystemAssistantProvider(): Promise<{
  system: ProviderRef;
  status: "compatible" | "limited" | "unavailable";
  statusDetail: string;
}> {
  return apiRequest("/api/assistant/providers/system/probe", { method: "POST" });
}

export function createAssistantProvider(input: {
  name: string;
  baseUrl: string;
  dialect: "responses" | "chat_completions" | "auto";
  model: string;
  apiKey: string;
  headers: Record<string, string>;
  contextWindow?: number;
  allowPrivateEndpoint: boolean;
  isDefault: boolean;
}): Promise<{ connection: ProviderConnection }> {
  return apiRequest("/api/assistant/providers", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function probeAssistantProvider(
  connectionId: string,
): Promise<{ connection: ProviderConnection }> {
  return apiRequest(`/api/assistant/providers/${connectionId}/probe`, { method: "POST" });
}

export function setDefaultAssistantProvider(
  connectionId: string,
): Promise<{ connection: ProviderConnection }> {
  return apiRequest(`/api/assistant/providers/${connectionId}/default`, { method: "POST" });
}

export function disableAssistantProvider(connectionId: string): Promise<{ disabled: boolean }> {
  return apiRequest(`/api/assistant/providers/${connectionId}`, { method: "DELETE" });
}
