import type { TiptapNode } from "~entities/project";

export type DictationStatus =
  | "preparing"
  | "recording_online"
  | "recording_degraded"
  | "paused"
  | "stopping"
  | "finalizing"
  | "review_ready"
  | "finalization_failed"
  | "missing_chunks"
  | "trashed";

export interface TranscriptWord {
  text: string;
  startMs: number;
  endMs: number;
  confidence?: number;
}

export interface CommandEvent {
  kind:
    | "new_paragraph"
    | "dialogue_start"
    | "scene_break"
    | "marker"
    | "undo_command"
    | "navigate_paragraph"
    | "navigate_sentence"
    | "navigate_text"
    | "navigate_end";
  rawText: string;
  executed: boolean;
  target?: string;
  ordinal?: number;
}

export interface TranscriptSegment {
  id: string;
  text: string;
  startMs: number;
  endMs: number;
  confidence?: number;
  words?: TranscriptWord[];
  command?: CommandEvent;
}

export interface TranscriptArtifact {
  id: string;
  sessionId: string;
  kind: "live_final" | "batch_source" | "cleaned_suggestion";
  pipelineVersion: string;
  provider: string;
  model: string;
  language: string;
  text: string;
  segments: TranscriptSegment[];
  audioSha256?: string;
  createdAt: string;
}

export interface TranscriptDraft {
  id: string;
  sessionId: string;
  sourceArtifactId?: string;
  revision: number;
  baseSourceText: string;
  content: TiptapNode;
  plainText: string;
  manuallyEdited: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface TranscriptDraftRevision {
  id: string;
  sessionId: string;
  revision: number;
  content: TiptapNode;
  plainText: string;
  source: string;
  createdAt: string;
}

export interface DictationSession {
  id: string;
  projectId: string;
  destinationNodeId?: string;
  clientSessionId: string;
  language: "ru" | "en";
  status: DictationStatus;
  durationMs: number;
  recordedDurationMs: number;
  manifest: {
    format?: string;
    chunks: Array<{ sequence: number; sha256: string }>;
  };
  liveArtifact?: TranscriptArtifact;
  batchArtifact?: TranscriptArtifact;
  suggestionArtifact?: TranscriptArtifact;
  retranscriptionStatus?: "queued" | "running" | "ready" | "failed";
  draft?: TranscriptDraft;
  draftRevisions?: TranscriptDraftRevision[];
  createdAt: string;
  updatedAt: string;
}

export interface LiveTranscriptEvent {
  type:
    | "partial"
    | "segment_final"
    | "command_updated"
    | "segment_updated"
    | "segments_replaced"
    | "stream_degraded"
    | "live_flushed"
    | "speech_started"
    | "speech_stopped";
  sessionId: string;
  providerTurnId?: string;
  segmentId?: string;
  segmentIds?: string[];
  text?: string;
  startMs?: number;
  endMs?: number;
  words?: TranscriptWord[];
  command?: CommandEvent;
  executed?: boolean;
  reason?: string;
}

export interface STTCapabilities {
  available: boolean;
  reasonCode?: "provider_not_configured" | string;
  modes: string[];
  deployment: string;
  streamingProtocol: "fixture" | "wav_chunks" | "pcm16" | "unavailable";
  languages: string[];
  partialResults: boolean;
  segmentTimestamps: boolean;
  wordTimestamps: boolean;
  confidence: string;
  vocabularyHints: string;
  supportsLongAudio: boolean;
}

export interface RecoveryItem {
  sessionId: string;
  projectId: string;
  projectTitle: string;
  destinationTitle?: string;
  status: DictationStatus;
  durationMs: number;
  localChunkCount: number;
  remoteChunkCount: number;
  createdAt: string;
}
