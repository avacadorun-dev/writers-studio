export type AssistantInputMode = "voice" | "chat" | "mcp";
export type AssistantTargetKind = "project" | "node" | "canvas";

export interface AssistantWriterPreferences {
  language: "ru" | "en";
  commandPrefix: string;
  commandsEnabled: boolean;
  liveTranscript: boolean;
  autoPlaySource: boolean;
  assistantSilenceCheckpointMs: 0 | 3000 | 5000 | 8000;
  fontFamily: "sofia" | "inter" | "source-serif";
  fontSize: 17 | 19 | 21;
  lineHeight: 1.5 | 1.75 | 2;
  manuscriptWidth: 60 | 70 | 80;
  theme: "light" | "dark" | "system";
  spellcheck: boolean;
  wordGoal: number;
}
export type AssistantRunStatus =
  | "active"
  | "working"
  | "awaiting_review"
  | "completed"
  | "cancelled"
  | "failed";
export type AssistantTurnState =
  | "collecting"
  | "awaiting_confirmation"
  | "queued"
  | "working"
  | "awaiting_review"
  | "completed"
  | "failed"
  | "cancelled";

export interface AssistantCapabilities {
  voiceAvailable: boolean;
  chatAvailable: boolean;
  systemProvider?: ProviderRef;
  systemProviderAvailable: boolean;
  systemProviderConfigured: boolean;
  systemProviderStatus: "unchecked" | "compatible" | "limited" | "unavailable";
  systemProviderStatusDetail: string;
  customProvidersEnabled: boolean;
  changeReviewRequired: boolean;
  partialApply: boolean;
  groupedUndo: boolean;
}

export interface ProviderCapabilities {
  text: boolean;
  strictTools: boolean;
  toolResults: boolean;
  sequentialToolCalls: boolean;
  streaming: boolean;
  parallelToolCalls: boolean;
  supportedDialects?: string[];
  limitations?: string[];
  checkedWithModel?: string;
}

export interface ProviderRef {
  kind: "system" | "connection" | "mcp";
  connectionId?: string;
  name: string;
  model: string;
  dialect: "responses" | "chat_completions" | "mcp";
  capabilities: ProviderCapabilities;
}

export interface AssistantTurn {
  id: string;
  runId: string;
  sequence: number;
  inputMode: AssistantInputMode;
  state: AssistantTurnState;
  transcript: string;
  instruction: string;
  assistantText: string;
  selectionAttachment?: AssistantSelectionAttachment;
  guideRevision?: number;
  errorCode?: string;
  createdAt: string;
  confirmedAt?: string;
  startedAt?: string;
  completedAt?: string;
}

export interface AssistantSelectionRange {
  blockId: string;
  start: number;
  end: number;
  expectedText: string;
}

export interface AssistantSelectionAttachment {
  nodeId: string;
  baseRevision: number;
  ranges: AssistantSelectionRange[];
  quote: string;
}

export interface AssistantLivePreview {
  operationId: string;
  targetNodeId: string;
  baseRevision?: number;
  toolName: string;
  arguments: Record<string, unknown>;
  beforeSnapshot?: Record<string, unknown>;
  proposedSnapshot: Record<string, unknown>;
  trace?: Record<string, unknown>;
  canonical: false;
  stale?: boolean;
}

export interface AssistantTextRange {
  blockId: string;
  start: number;
  end: number;
  reason: string;
}

export interface AssistantResolvedTarget {
  nodeId: string;
  baseRevision: number;
  ranges: AssistantTextRange[];
  protectedRanges: AssistantTextRange[];
  sourcePhrase: string;
}

export interface AssistantChangeOperation {
  id: string;
  position: number;
  groupKey: string;
  selected: boolean;
  dependencies: string[];
  toolName: string;
  targetKind: string;
  targetId?: string;
  baseRevision?: number;
  arguments: Record<string, unknown>;
  beforeSnapshot?: { content?: Record<string, unknown>; plainText?: string; title?: string };
  proposedSnapshot: Record<string, unknown>;
  trace?:
    | AssistantResolvedTarget
    | { resolvedTarget?: AssistantResolvedTarget }
    | Record<string, unknown>;
  riskLevel: "low" | "medium" | "high";
  applied: boolean;
}

export interface AssistantChangeSet {
  id: string;
  runId: string;
  turnId: string;
  state: "staged" | "applied" | "rejected" | "stale" | "failed" | "undone";
  title: string;
  summary: string;
  riskLevel: "low" | "medium" | "high";
  operations: AssistantChangeOperation[];
  operationId?: string;
  appliedAt?: string;
  rejectedAt?: string;
  undoneAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface AssistantRun {
  id: string;
  projectId: string;
  title: string;
  targetKind: AssistantTargetKind;
  targetNodeId?: string;
  targetCanvasId?: string;
  inputMode: AssistantInputMode;
  status: AssistantRunStatus;
  silenceCheckpointMs: number;
  origin: "internal" | "remote_mcp";
  agent?: { grantId: string; clientName: string };
  provider: ProviderRef;
  guideRevision?: number;
  readContext: Record<string, unknown>;
  errorCode?: string;
  startedAt: string;
  updatedAt: string;
  lastViewedAt: string;
  unread: boolean;
  endedAt?: string;
  latestTurn?: AssistantTurn;
  pendingChangeSet?: AssistantChangeSet;
  latestChangeSet?: AssistantChangeSet;
}

export interface AssistantEvent {
  id: string;
  runId: string;
  turnId?: string;
  sequence: number;
  type: string;
  data: Record<string, unknown>;
  createdAt: string;
}

export interface AssistantGuideRevision {
  id: string;
  revision: number;
  content: string;
  source: string;
  restoredFromRevision?: number;
  assistantRunId?: string;
  createdAt: string;
}

export interface AssistantGuide {
  projectId: string;
  revision: number;
  content: string;
  source: string;
  history?: AssistantGuideRevision[];
  createdAt: string;
  updatedAt: string;
}

export interface ProjectMemoryRecord {
  id: string;
  projectId: string;
  kind:
    | "fact"
    | "character"
    | "world_rule"
    | "plot"
    | "style"
    | "constraint"
    | "decision"
    | "other";
  statement: string;
  scope: "project" | "node";
  scopeNodeId?: string;
  sourceReferences: Array<{
    kind: string;
    id: string;
    label?: string;
    quote?: string;
    start?: number;
    end?: number;
  }>;
  tags: string[];
  active: boolean;
  alwaysRelevant: boolean;
  revision: number;
  source: string;
  createdAt: string;
  updatedAt: string;
}

export interface ProjectMemoryRevision {
  id: string;
  memoryId: string;
  revision: number;
  snapshot: ProjectMemoryRecord;
  source: string;
  operationId?: string;
  assistantRunId?: string;
  createdAt: string;
}

export interface ProviderConnection {
  id: string;
  name: string;
  baseUrl: string;
  dialect: "responses" | "chat_completions" | "auto";
  model: string;
  apiKeyLast4: string;
  requestTimeoutMs: number;
  contextWindow?: number;
  allowPrivateEndpoint: boolean;
  capabilities: ProviderCapabilities;
  status: "checking" | "compatible" | "limited" | "unavailable";
  statusDetail: string;
  isDefault: boolean;
  checkedAt?: string;
  createdAt: string;
  updatedAt: string;
}
