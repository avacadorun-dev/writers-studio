export interface ScrollGeometry {
  scrollHeight: number;
  scrollTop: number;
  clientHeight: number;
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
