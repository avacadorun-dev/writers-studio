import type { TiptapNode } from "~entities/project";
import { apiRequest } from "~shared/api";

import type { DictationSession, RecoveryItem, STTCapabilities } from "../model/types";

export function getSTTCapabilities(): Promise<STTCapabilities> {
  return apiRequest("/api/stt/capabilities");
}

export function createDictation(input: {
  projectId: string;
  destinationNodeId?: string;
  clientSessionId: string;
  language: "ru" | "en";
}): Promise<{ session: DictationSession }> {
  return apiRequest("/api/dictations", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function getDictation(sessionId: string): Promise<{ session: DictationSession }> {
  return apiRequest(`/api/dictations/${sessionId}`);
}

export function transitionDictation(
  sessionId: string,
  status: DictationSession["status"],
): Promise<{ session: DictationSession }> {
  return apiRequest(`/api/dictations/${sessionId}/transition`, {
    method: "POST",
    body: JSON.stringify({ status }),
  });
}

export function uploadAudioChunk(
  sessionId: string,
  sequence: number,
  blob: Blob,
  sha256: string,
  durationMs: number,
): Promise<{ sequence: number; sha256: string; duplicate: boolean; durable: boolean }> {
  return apiRequest(`/api/dictations/${sessionId}/chunks/${sequence}`, {
    method: "PUT",
    headers: {
      "Content-Type": blob.type || "application/octet-stream",
      "X-Content-SHA256": sha256,
      "X-Audio-Duration-Ms": String(durationMs),
    },
    body: blob,
  });
}

export function stopDictation(
  sessionId: string,
  input: {
    durationMs: number;
    recordedDurationMs: number;
    manifest: {
      format: string;
      chunks: Array<{ sequence: number; sha256: string }>;
    };
  },
): Promise<{ session: DictationSession }> {
  return apiRequest(`/api/dictations/${sessionId}/stop`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function retryDictation(sessionId: string): Promise<{ status: "finalizing" }> {
  return apiRequest(`/api/dictations/${sessionId}/retry`, { method: "POST" });
}

export function retranscribeDictation(
  sessionId: string,
): Promise<{ status: "queued"; requestId: string }> {
  return apiRequest(`/api/dictations/${sessionId}/retranscribe`, { method: "POST" });
}

export function saveTranscriptDraft(
  sessionId: string,
  content: TiptapNode,
  source: "manual" | "final_batch_apply" | "final_batch_undo" = "manual",
): Promise<{ draft: DictationSession["draft"] }> {
  return apiRequest(`/api/dictations/${sessionId}/draft`, {
    method: "PUT",
    body: JSON.stringify({ content, source }),
  });
}

export function transferTranscript(
  sessionId: string,
  input: { nodeId: string; baseRevision: number; content: TiptapNode },
): Promise<{
  revision: { revision: number };
  operationId: string;
  undoable: boolean;
}> {
  return apiRequest(`/api/dictations/${sessionId}/transfer`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function getDictationAudio(
  sessionId: string,
): Promise<{ url: string; durationMs: number; sha256: string }> {
  return apiRequest(`/api/dictations/${sessionId}/audio`);
}

export function getSegmentAudioSource(
  segmentId: string,
): Promise<{ url: string; startMs: number; endMs: number }> {
  return apiRequest(`/api/source-segments/${encodeURIComponent(segmentId)}/audio`);
}

export function getRecoveryItems(): Promise<{ items: RecoveryItem[] }> {
  return apiRequest("/api/recovery");
}
