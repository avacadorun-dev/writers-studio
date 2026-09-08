export interface ScrollGeometry {
  scrollHeight: number;
  scrollTop: number;
  clientHeight: number;
}

export interface AssistantTranscriptScrollSnapshot {
  scrollTop: number;
  followsActivity: boolean;
}

export const assistantTranscriptEndThreshold = 96;

export function assistantTranscriptDistanceFromEnd(geometry: ScrollGeometry): number {
  return Math.max(0, geometry.scrollHeight - geometry.scrollTop - geometry.clientHeight);
}

export function isAssistantTranscriptNearEnd(
  geometry: ScrollGeometry,
  threshold = assistantTranscriptEndThreshold,
): boolean {
  return assistantTranscriptDistanceFromEnd(geometry) <= threshold;
}

export function assistantTranscriptScrollStorageKey(runId: string): string {
  return `writers-studio-assistant-scroll:${runId}`;
}

export function captureAssistantTranscriptScroll(
  geometry: ScrollGeometry,
): AssistantTranscriptScrollSnapshot {
  return {
    scrollTop: Math.max(0, geometry.scrollTop),
    followsActivity: isAssistantTranscriptNearEnd(geometry),
  };
}

export function normalizeAssistantTranscriptScrollSnapshot(
  value: unknown,
): AssistantTranscriptScrollSnapshot | null {
  if (!value || typeof value !== "object") return null;
  const candidate = value as { scrollTop?: unknown; followsActivity?: unknown };
  if (
    typeof candidate.scrollTop !== "number" ||
    !Number.isFinite(candidate.scrollTop) ||
    typeof candidate.followsActivity !== "boolean"
  ) {
    return null;
  }
  return {
    scrollTop: Math.max(0, candidate.scrollTop),
    followsActivity: candidate.followsActivity,
  };
}

export function restoredAssistantTranscriptScrollTop(
  geometry: Pick<ScrollGeometry, "scrollHeight" | "clientHeight">,
  snapshot: AssistantTranscriptScrollSnapshot | null,
): number {
  const maximum = Math.max(0, geometry.scrollHeight - geometry.clientHeight);
  if (!snapshot || snapshot.followsActivity) return maximum;
  return Math.min(maximum, Math.max(0, snapshot.scrollTop));
}
