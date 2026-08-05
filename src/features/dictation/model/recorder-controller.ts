import {
  createDictation,
  type DictationStatus,
  getSTTCapabilities,
  type LiveTranscriptEvent,
  stopDictation,
  transitionDictation,
  uploadAudioChunk,
} from "~entities/dictation";
import { webSocketURL } from "~shared/api";
import {
  createClientId,
  type LocalAudioChunk,
  type LocalRecording,
  listAudioChunks,
  markAudioChunkUploaded,
  saveAudioChunk,
  saveLocalRecording,
  sha256Hex,
} from "~shared/lib";

import { type LivePCMChunk, LivePCMStream } from "./live-pcm-stream";

export interface RecorderSnapshot {
  localId?: string;
  serverSessionId?: string;
  status: "idle" | DictationStatus | "local_only";
  totalDurationMs: number;
  recordedDurationMs: number;
  partial: string;
  stableSegments: Array<{
    id: string;
    text: string;
    startMs: number;
    endMs: number;
    command?: LiveTranscriptEvent["command"];
  }>;
  connection: "offline" | "connecting" | "online" | "degraded";
  uploadedChunks: number;
  totalChunks: number;
  projectId?: string;
  projectTitle?: string;
  destinationNodeId?: string;
  destinationTitle?: string;
  language: "ru" | "en";
  commandsEnabled: boolean;
  commandPrefix: string;
  error?: string;
}

export type QuickDictationCommandKind =
  | "new_paragraph"
  | "dialogue_start"
  | "scene_break"
  | "undo_command";

export interface StartRecorderInput {
  projectId: string;
  projectTitle: string;
  destinationNodeId?: string;
  destinationTitle?: string;
  language: "ru" | "en";
  commandsEnabled: boolean;
  commandPrefix: string;
  liveTranscript: boolean;
  stream: MediaStream;
}

export interface StopResult {
  sessionId?: string;
  localOnly: boolean;
}

const initialSnapshot: RecorderSnapshot = {
  status: "idle",
  totalDurationMs: 0,
  recordedDurationMs: 0,
  partial: "",
  stableSegments: [],
  connection: navigator.onLine ? "connecting" : "offline",
  uploadedChunks: 0,
  totalChunks: 0,
  language: "ru",
  commandsEnabled: true,
  commandPrefix: "Команда",
};

class RecorderController {
  private snapshot: RecorderSnapshot = initialSnapshot;
  private listeners = new Set<() => void>();
  private recorder?: MediaRecorder;
  private stream?: MediaStream;
  private socket?: WebSocket;
  private localRecording?: LocalRecording;
  private sequence = 0;
  private startedAt = 0;
  private recordedStartedAt = 0;
  private recordedBeforeCurrentTurn = 0;
  private timer?: number;
  private processing = Promise.resolve();
  private stopPromise?: Promise<StopResult>;
  private stopResolver?: () => void;
  private uploadInFlight = new Map<number, Promise<boolean>>();
  private commandsEnabled = true;
  private commandPrefix = "Команда";
  private liveTranscriptEnabled = true;
  private liveTransport: "unknown" | "fixture" | "pcm_wav" | "pcm_stream" | "unavailable" =
    "unknown";
  private livePCM?: LivePCMStream;
  private liveSetup?: Promise<void>;
  private livePCMReady?: Promise<void>;
  private pendingLiveChunks: LivePCMChunk[] = [];
  private liveFlushResolver?: () => void;
  private liveFlushTimer?: number;
  private recentAudio?: HTMLAudioElement;
  private recentAudioURL?: string;

  constructor() {
    window.addEventListener("offline", () => {
      if (this.isActive()) {
        this.setSnapshot({
          connection: "offline",
          status: this.snapshot.status === "paused" ? "paused" : "recording_degraded",
        });
      }
    });
    window.addEventListener("online", () => {
      if (this.isActive()) {
        this.setSnapshot({ connection: "connecting" });
        this.connectLive();
        void this.flushUploads();
      }
    });
    window.addEventListener("beforeunload", (event) => {
      if (this.isActive()) {
        event.preventDefault();
      }
    });
  }

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  getSnapshot = (): RecorderSnapshot => this.snapshot;

  async start(input: StartRecorderInput): Promise<string> {
    if (this.isActive()) {
      throw new Error("Запись уже идёт");
    }
    const mimeType = supportedMimeType();
    const localId = createClientId();
    this.stream = input.stream;
    this.commandsEnabled = input.commandsEnabled;
    this.commandPrefix = input.commandPrefix;
    this.liveTranscriptEnabled = input.liveTranscript;
    this.liveTransport = "unknown";
    this.livePCM = undefined;
    this.liveSetup = undefined;
    this.livePCMReady = undefined;
    this.pendingLiveChunks = [];
    this.sequence = 0;
    this.startedAt = performance.now();
    this.recordedStartedAt = this.startedAt;
    this.recordedBeforeCurrentTurn = 0;
    this.stopPromise = undefined;
    this.processing = Promise.resolve();
    this.localRecording = {
      localId,
      projectId: input.projectId,
      projectTitle: input.projectTitle,
      destinationNodeId: input.destinationNodeId,
      destinationTitle: input.destinationTitle,
      language: input.language,
      commandsEnabled: input.commandsEnabled,
      commandPrefix: input.commandPrefix,
      liveTranscript: input.liveTranscript,
      format: mimeType.includes("ogg") ? "ogg" : "webm",
      status: "recording",
      startedAt: Date.now(),
      durationMs: 0,
      recordedDurationMs: 0,
      lastAcknowledgedSequence: -1,
    };
    await saveLocalRecording(this.localRecording);

    this.recorder = new MediaRecorder(input.stream, mimeType ? { mimeType } : undefined);
    this.recorder.addEventListener("dataavailable", (event) => {
      if (event.data.size === 0) {
        return;
      }
      this.processing = this.processing.then(() => this.processChunk(event.data));
    });
    this.recorder.addEventListener("stop", () => {
      this.stopResolver?.();
      this.stopResolver = undefined;
    });
    this.recorder.start(1_000);
    this.setSnapshot({
      localId,
      serverSessionId: undefined,
      status: navigator.onLine ? "recording_online" : "recording_degraded",
      totalDurationMs: 0,
      recordedDurationMs: 0,
      partial: "",
      stableSegments: [],
      connection: navigator.onLine ? "connecting" : "offline",
      uploadedChunks: 0,
      totalChunks: 0,
      projectId: input.projectId,
      projectTitle: input.projectTitle,
      destinationNodeId: input.destinationNodeId,
      destinationTitle: input.destinationTitle,
      language: input.language,
      commandsEnabled: input.commandsEnabled,
      commandPrefix: input.commandPrefix,
      error: undefined,
    });
    this.startTimer();
    this.liveSetup = this.configureLiveCapture(input.stream);
    void this.createServerSession();
    return localId;
  }

  async pause(): Promise<void> {
    if (this.recorder?.state !== "recording") {
      return;
    }
    this.recorder.requestData();
    this.recorder.pause();
    await this.liveSetup;
    await this.livePCM?.pause().catch(() => undefined);
    this.recordedBeforeCurrentTurn += performance.now() - this.recordedStartedAt;
    this.localRecording = this.localRecording
      ? { ...this.localRecording, status: "paused" }
      : undefined;
    if (this.localRecording) {
      await saveLocalRecording(this.localRecording);
    }
    this.setSnapshot({ status: "paused" });
    if (this.snapshot.serverSessionId) {
      await transitionDictation(this.snapshot.serverSessionId, "paused").catch(() => undefined);
    }
  }

  async resume(): Promise<void> {
    if (this.recorder?.state !== "paused") {
      return;
    }
    this.recordedStartedAt = performance.now();
    this.recorder.resume();
    await this.liveSetup;
    await this.livePCM?.resume().catch(() => undefined);
    this.localRecording = this.localRecording
      ? { ...this.localRecording, status: "recording" }
      : undefined;
    if (this.localRecording) {
      await saveLocalRecording(this.localRecording);
    }
    const status =
      navigator.onLine && this.socket?.readyState === WebSocket.OPEN
        ? "recording_online"
        : "recording_degraded";
    this.setSnapshot({ status });
    if (this.snapshot.serverSessionId) {
      await transitionDictation(this.snapshot.serverSessionId, status).catch(() => undefined);
    }
  }

  addMarker(): void {
    if (!this.isActive()) {
      return;
    }
    const marker = {
      id: createClientId(),
      text: "Метка",
      startMs: this.snapshot.recordedDurationMs,
      endMs: this.snapshot.recordedDurationMs,
      command: {
        kind: "marker" as const,
        rawText: "Команда: метка",
        executed: true,
      },
    };
    this.setSnapshot({ stableSegments: [...this.snapshot.stableSegments, marker] });
    this.sendJSON({
      type: "client_marker",
      segmentId: marker.id,
      text: marker.text,
      startMs: marker.startMs,
      endMs: marker.endMs,
    });
  }

  executeCommand(kind: QuickDictationCommandKind): boolean {
    if (!this.isActive()) {
      return false;
    }
    const label = quickCommandLabel(kind);
    let stableSegments = this.snapshot.stableSegments;
    if (kind === "undo_command") {
      const targetIndex = stableSegments.findLastIndex(
        (segment) => segment.command?.executed && segment.command.kind !== "undo_command",
      );
      if (targetIndex < 0) {
        return false;
      }
      stableSegments = stableSegments.map((segment, index) =>
        index === targetIndex && segment.command
          ? { ...segment, command: { ...segment.command, executed: false } }
          : segment,
      );
    }
    const command = {
      id: createClientId(),
      text: label,
      startMs: this.snapshot.recordedDurationMs,
      endMs: this.snapshot.recordedDurationMs,
      command: {
        kind,
        rawText: `Кнопка: ${label}`,
        executed: true,
      },
    };
    this.setSnapshot({ stableSegments: [...stableSegments, command] });
    this.sendJSON({
      type: "client_command",
      segmentId: command.id,
      commandKind: kind,
      startMs: command.startMs,
      endMs: command.endMs,
    });
    return true;
  }

  setCommandExecution(segmentID: string, executed: boolean): void {
    this.setSnapshot({
      stableSegments: this.snapshot.stableSegments.map((segment) =>
        segment.id === segmentID && segment.command
          ? { ...segment, command: { ...segment.command, executed } }
          : segment,
      ),
    });
    this.sendJSON({ type: "set_command", segmentId: segmentID, executed });
  }

  editSegment(segmentID: string, text: string): void {
    const normalized = text.trim();
    if (!normalized) {
      return;
    }
    this.setSnapshot({
      stableSegments: this.snapshot.stableSegments.map((segment) =>
        segment.id === segmentID ? { ...segment, text: normalized } : segment,
      ),
    });
    this.sendJSON({ type: "edit_segment", segmentId: segmentID, text: normalized });
  }

  replaceSegments(segmentIDs: string[], text: string): void {
    const normalized = text.trim();
    if (segmentIDs.length === 0) {
      return;
    }
    this.setSnapshot({
      stableSegments: replaceStableSegments(this.snapshot.stableSegments, segmentIDs, normalized),
    });
    this.sendJSON({
      type: "replace_segments",
      segmentIds: segmentIDs,
      text: normalized,
    });
  }

  async playRecent(seconds = 10): Promise<void> {
    if (!this.localRecording) {
      return;
    }
    const chunks = await listAudioChunks(this.localRecording.localId);
    const count = Math.max(1, Math.ceil(seconds));
    const recent = chunks.slice(-count);
    if (recent.length === 0) {
      throw new Error("Аудиофрагменты ещё не готовы");
    }
    this.recentAudio?.pause();
    if (this.recentAudioURL) {
      URL.revokeObjectURL(this.recentAudioURL);
    }
    const blob = new Blob(
      recent.map((chunk) => chunk.blob),
      { type: recent[0]?.blob.type || this.recorder?.mimeType || "audio/webm" },
    );
    this.recentAudioURL = URL.createObjectURL(blob);
    this.recentAudio = new Audio(this.recentAudioURL);
    await this.recentAudio.play();
  }

  stop(): Promise<StopResult> {
    if (this.stopPromise) {
      return this.stopPromise;
    }
    this.stopPromise = this.performStop();
    return this.stopPromise;
  }

  reset(): void {
    if (this.isActive()) {
      return;
    }
    this.setSnapshot(initialSnapshot);
    this.stopPromise = undefined;
  }

  private async performStop(): Promise<StopResult> {
    if (!this.recorder || !this.localRecording) {
      return { localOnly: true };
    }
    const wasRecording = this.recorder.state === "recording";
    if (wasRecording) {
      this.recordedBeforeCurrentTurn += performance.now() - this.recordedStartedAt;
    }
    this.setSnapshot({ status: "stopping" });
    this.localRecording = { ...this.localRecording, status: "stopping" };
    await saveLocalRecording(this.localRecording);
    await this.liveSetup;
    await this.livePCM?.stop().catch(() => undefined);
    this.flushPendingLiveChunks();
    await this.waitForLiveFlush();

    const stopped = new Promise<void>((resolve) => {
      this.stopResolver = resolve;
    });
    if (this.recorder.state !== "inactive") {
      if (this.recorder.state === "recording") {
        this.recorder.requestData();
      }
      this.recorder.stop();
    } else {
      this.stopResolver?.();
    }
    await stopped;
    await this.processing;
    window.clearInterval(this.timer);
    this.socket?.close(1000, "recording finished");
    this.socket = undefined;
    this.pendingLiveChunks = [];
    for (const track of this.stream?.getTracks() ?? []) {
      track.stop();
    }

    const chunks = await listAudioChunks(this.localRecording.localId);
    const allUploaded = await this.flushUploads(chunks);
    const serverSessionId = this.snapshot.serverSessionId;
    const durationMs = Math.round(performance.now() - this.startedAt);
    const recordedDurationMs = Math.round(this.recordedBeforeCurrentTurn);
    this.localRecording = {
      ...this.localRecording,
      status: serverSessionId && allUploaded ? "finalizing" : "stopping",
      durationMs,
      recordedDurationMs,
    };
    await saveLocalRecording(this.localRecording);

    if (!serverSessionId || !allUploaded) {
      this.setSnapshot({
        status: "local_only",
        connection: navigator.onLine ? "degraded" : "offline",
        error: "Запись сохранена на этом устройстве и будет синхронизирована позже.",
      });
      return { localOnly: true };
    }

    const response = await stopDictation(serverSessionId, {
      durationMs,
      recordedDurationMs,
      manifest: {
        format: this.localRecording.format,
        chunks: chunks.map((chunk) => ({
          sequence: chunk.sequence,
          sha256: chunk.sha256,
        })),
      },
    });
    this.localRecording = { ...this.localRecording, status: "uploaded" };
    await saveLocalRecording(this.localRecording);
    this.setSnapshot({ status: response.session.status });
    return { sessionId: serverSessionId, localOnly: false };
  }

  private async createServerSession(): Promise<void> {
    if (!this.localRecording) {
      return;
    }
    try {
      const response = await createDictation({
        projectId: this.localRecording.projectId,
        destinationNodeId: this.localRecording.destinationNodeId,
        clientSessionId: this.localRecording.localId,
        language: this.localRecording.language,
      });
      if (!this.localRecording) {
        return;
      }
      this.localRecording = {
        ...this.localRecording,
        serverSessionId: response.session.id,
      };
      await saveLocalRecording(this.localRecording);
      this.setSnapshot({ serverSessionId: response.session.id });
      this.connectLive();
      await this.flushUploads();
    } catch {
      this.setSnapshot({
        status: this.snapshot.status === "paused" ? "paused" : "recording_degraded",
        connection: navigator.onLine ? "degraded" : "offline",
      });
    }
  }

  private connectLive(): void {
    const sessionId = this.snapshot.serverSessionId;
    if (
      !this.isActive() ||
      !sessionId ||
      !navigator.onLine ||
      this.socket?.readyState === WebSocket.OPEN
    ) {
      return;
    }
    this.socket?.close();
    this.setSnapshot({ connection: "connecting" });
    const socket = new WebSocket(webSocketURL(`/api/dictations/${sessionId}/live`));
    socket.binaryType = "arraybuffer";
    this.socket = socket;
    socket.addEventListener("open", () => {
      if (this.socket === socket) {
        this.setSnapshot({
          connection: "online",
          status: this.snapshot.status === "paused" ? "paused" : "recording_online",
        });
        this.sendJSON({
          type: "configure",
          commandsEnabled: this.commandsEnabled,
          commandPrefix: this.commandPrefix,
        });
        this.flushPendingLiveChunks();
      }
    });
    socket.addEventListener("message", (event) => {
      if (typeof event.data !== "string") {
        return;
      }
      let message: LiveTranscriptEvent;
      try {
        message = JSON.parse(event.data) as LiveTranscriptEvent;
      } catch {
        return;
      }
      if (message.type === "partial") {
        if (this.liveTranscriptEnabled) {
          this.setSnapshot({
            partial: message.text ?? "",
            connection: "online",
            status:
              this.snapshot.status === "paused" || this.snapshot.status === "stopping"
                ? this.snapshot.status
                : "recording_online",
          });
        }
      } else if (message.type === "segment_final" && message.segmentId && message.text) {
        if (!this.liveTranscriptEnabled) {
          return;
        }
        const exists = this.snapshot.stableSegments.some(
          (segment) => segment.id === message.segmentId,
        );
        this.setSnapshot({
          partial: "",
          connection: "online",
          status:
            this.snapshot.status === "paused" || this.snapshot.status === "stopping"
              ? this.snapshot.status
              : "recording_online",
          stableSegments: exists
            ? this.snapshot.stableSegments
            : [
                ...this.snapshot.stableSegments,
                {
                  id: message.segmentId,
                  text: message.text,
                  startMs: message.startMs ?? 0,
                  endMs: message.endMs ?? 0,
                  command: message.command,
                },
              ],
        });
      } else if (message.type === "live_flushed") {
        this.resolveLiveFlush();
      } else if (message.type === "command_updated" && message.segmentId) {
        this.setSnapshot({
          stableSegments: this.snapshot.stableSegments.map((segment) =>
            segment.id === message.segmentId && segment.command
              ? {
                  ...segment,
                  command: {
                    ...segment.command,
                    executed: message.executed ?? segment.command.executed,
                  },
                }
              : segment,
          ),
        });
      } else if (message.type === "segment_updated" && message.segmentId && message.text) {
        this.setSnapshot({
          stableSegments: this.snapshot.stableSegments.map((segment) =>
            segment.id === message.segmentId
              ? { ...segment, text: message.text ?? segment.text }
              : segment,
          ),
        });
      } else if (message.type === "segments_replaced" && message.segmentIds) {
        this.setSnapshot({
          stableSegments: replaceStableSegments(
            this.snapshot.stableSegments,
            message.segmentIds,
            message.text?.trim() ?? "",
          ),
        });
      } else if (message.type === "stream_degraded") {
        this.setSnapshot({
          connection: "degraded",
          status: this.snapshot.status === "paused" ? "paused" : "recording_degraded",
        });
      }
    });
    socket.addEventListener("close", () => {
      if (this.socket !== socket || !this.isActive()) {
        return;
      }
      if (this.snapshot.status === "stopping") {
        this.resolveLiveFlush();
        return;
      }
      this.setSnapshot({
        connection: navigator.onLine ? "degraded" : "offline",
        status: this.snapshot.status === "paused" ? "paused" : "recording_degraded",
      });
      if (navigator.onLine) {
        window.setTimeout(() => this.connectLive(), 1_000);
      }
    });
  }

  private async processChunk(blob: Blob): Promise<void> {
    if (!this.localRecording) {
      return;
    }
    const sequence = this.sequence++;
    const sha256 = await sha256Hex(blob);
    const chunk: LocalAudioChunk = {
      localId: this.localRecording.localId,
      sequence,
      blob,
      sha256,
      durationMs: 1_000,
      uploaded: false,
      createdAt: Date.now(),
    };
    await saveAudioChunk(chunk);
    this.setSnapshot({ totalChunks: this.snapshot.totalChunks + 1 });
    if (this.liveTransport === "fixture" && this.socket?.readyState === WebSocket.OPEN) {
      this.socket.send(await blob.arrayBuffer());
    }
    await this.uploadChunk(chunk);
  }

  private async configureLiveCapture(stream: MediaStream): Promise<void> {
    if (!this.liveTranscriptEnabled) {
      this.liveTransport = "unavailable";
      return;
    }
    try {
      const capabilities = await getSTTCapabilities();
      if (capabilities.modes.includes("realtime") && capabilities.streamingProtocol === "pcm16") {
        this.liveTransport = "pcm_stream";
        const livePCM = new LivePCMStream((chunk) => this.sendLivePCMChunk(chunk), 100, "linear16");
        this.livePCM = livePCM;
        this.livePCMReady = livePCM.start(stream).catch(() => {
          this.liveTransport = "unavailable";
          this.setSnapshot({
            connection: navigator.onLine ? "degraded" : "offline",
            status: this.snapshot.status === "paused" ? "paused" : "recording_degraded",
          });
        });
        await this.livePCMReady;
        return;
      }
      if (
        capabilities.modes.includes("chunked_live") &&
        capabilities.streamingProtocol === "wav_chunks"
      ) {
        this.liveTransport = "pcm_wav";
        const livePCM = new LivePCMStream((chunk) => this.sendLivePCMChunk(chunk));
        this.livePCM = livePCM;
        this.livePCMReady = livePCM.start(stream).catch(() => {
          this.liveTransport = "unavailable";
          this.setSnapshot({
            connection: navigator.onLine ? "degraded" : "offline",
            status: this.snapshot.status === "paused" ? "paused" : "recording_degraded",
          });
        });
        await this.livePCMReady;
        return;
      }
      if (capabilities.streamingProtocol === "fixture") {
        this.liveTransport = "fixture";
        return;
      }
      this.liveTransport = "unavailable";
    } catch {
      this.liveTransport = "unavailable";
      this.setSnapshot({
        connection: navigator.onLine ? "degraded" : "offline",
        status: this.snapshot.status === "paused" ? "paused" : "recording_degraded",
      });
    }
  }

  private sendLivePCMChunk(chunk: LivePCMChunk): void {
    if (!navigator.onLine || !this.isPCMTransport()) {
      return;
    }
    if (this.socket?.readyState !== WebSocket.OPEN) {
      this.pendingLiveChunks.push(chunk);
      this.pendingLiveChunks = this.pendingLiveChunks.slice(-100);
      return;
    }
    this.sendPCMChunk(chunk);
  }

  private flushPendingLiveChunks(): void {
    if (!this.isPCMTransport() || this.socket?.readyState !== WebSocket.OPEN) {
      return;
    }
    for (const chunk of this.pendingLiveChunks) {
      this.sendPCMChunk(chunk);
    }
    this.pendingLiveChunks = [];
  }

  private sendPCMChunk(chunk: LivePCMChunk): void {
    this.sendJSON({
      type: "audio_chunk",
      sequence: chunk.sequence,
      startMs: chunk.startMs,
      endMs: chunk.endMs,
      format: chunk.format,
      sampleRate: chunk.sampleRate,
    });
    this.socket?.send(chunk.data);
  }

  private async waitForLiveFlush(): Promise<void> {
    if (!this.isPCMTransport() || this.socket?.readyState !== WebSocket.OPEN) {
      return;
    }
    await new Promise<void>((resolve) => {
      this.liveFlushResolver = resolve;
      this.liveFlushTimer = window.setTimeout(() => this.resolveLiveFlush(), 35_000);
      this.sendJSON({ type: "flush_live" });
    });
  }

  private resolveLiveFlush(): void {
    window.clearTimeout(this.liveFlushTimer);
    this.liveFlushTimer = undefined;
    this.liveFlushResolver?.();
    this.liveFlushResolver = undefined;
  }

  private isPCMTransport(): boolean {
    return this.liveTransport === "pcm_wav" || this.liveTransport === "pcm_stream";
  }

  private async uploadChunk(chunk: LocalAudioChunk): Promise<boolean> {
    const sessionId = this.snapshot.serverSessionId;
    if (!sessionId || !navigator.onLine) {
      return false;
    }
    const existing = this.uploadInFlight.get(chunk.sequence);
    if (existing) {
      return existing;
    }
    const upload = uploadAudioChunk(
      sessionId,
      chunk.sequence,
      chunk.blob,
      chunk.sha256,
      chunk.durationMs,
    )
      .then(async () => {
        await markAudioChunkUploaded(chunk.localId, chunk.sequence);
        if (this.localRecording) {
          this.localRecording = {
            ...this.localRecording,
            lastAcknowledgedSequence: Math.max(
              this.localRecording.lastAcknowledgedSequence,
              chunk.sequence,
            ),
          };
          await saveLocalRecording(this.localRecording);
        }
        this.setSnapshot({ uploadedChunks: this.snapshot.uploadedChunks + 1 });
        return true;
      })
      .catch(() => false)
      .finally(() => {
        this.uploadInFlight.delete(chunk.sequence);
      });
    this.uploadInFlight.set(chunk.sequence, upload);
    return upload;
  }

  private async flushUploads(prefetched?: LocalAudioChunk[]): Promise<boolean> {
    if (!this.localRecording || !this.snapshot.serverSessionId) {
      return false;
    }
    const chunks = prefetched ?? (await listAudioChunks(this.localRecording.localId));
    const results = await Promise.all(
      chunks.filter((chunk) => !chunk.uploaded).map((chunk) => this.uploadChunk(chunk)),
    );
    return results.every(Boolean);
  }

  private startTimer(): void {
    window.clearInterval(this.timer);
    this.timer = window.setInterval(() => {
      if (!this.recorder || this.recorder.state === "inactive") {
        return;
      }
      const now = performance.now();
      const recordedCurrent =
        this.recorder.state === "recording" ? now - this.recordedStartedAt : 0;
      this.setSnapshot({
        totalDurationMs: Math.round(now - this.startedAt),
        recordedDurationMs: Math.round(this.recordedBeforeCurrentTurn + recordedCurrent),
      });
      if (this.localRecording) {
        this.localRecording = {
          ...this.localRecording,
          durationMs: Math.round(now - this.startedAt),
          recordedDurationMs: Math.round(this.recordedBeforeCurrentTurn + recordedCurrent),
        };
        void saveLocalRecording(this.localRecording);
      }
    }, 250);
  }

  private sendJSON(value: Record<string, unknown>): void {
    if (this.socket?.readyState === WebSocket.OPEN) {
      this.socket.send(JSON.stringify(value));
    }
  }

  private isActive(): boolean {
    return ["preparing", "recording_online", "recording_degraded", "paused", "stopping"].includes(
      this.snapshot.status,
    );
  }

  private setSnapshot(patch: Partial<RecorderSnapshot>): void {
    this.snapshot = { ...this.snapshot, ...patch };
    for (const listener of this.listeners) {
      listener();
    }
  }
}

function quickCommandLabel(kind: QuickDictationCommandKind): string {
  return {
    new_paragraph: "Новый абзац",
    dialogue_start: "Начало диалога",
    scene_break: "Разделитель сцены",
    undo_command: "Отменить команду",
  }[kind];
}

function replaceStableSegments(
  segments: RecorderSnapshot["stableSegments"],
  segmentIDs: string[],
  text: string,
): RecorderSnapshot["stableSegments"] {
  const selectedIDs = new Set(segmentIDs);
  const selected = segments.filter((segment) => selectedIDs.has(segment.id));
  if (selected.length === 0) {
    return segments;
  }
  const firstID = segmentIDs.find((segmentID) =>
    selected.some((segment) => segment.id === segmentID),
  );
  if (!firstID) {
    return segments;
  }
  const startMs = Math.min(...selected.map((segment) => segment.startMs));
  const endMs = Math.max(...selected.map((segment) => segment.endMs));
  const replacement = selected.find((segment) => segment.id === firstID);

  return segments.flatMap((segment) => {
    if (!selectedIDs.has(segment.id)) {
      return [segment];
    }
    if (segment.id !== firstID || !replacement || !text) {
      return [];
    }
    return [{ ...replacement, text, startMs, endMs, command: undefined }];
  });
}

function supportedMimeType(): string {
  const candidates = ["audio/webm;codecs=opus", "audio/ogg;codecs=opus", "audio/webm"];
  return candidates.find((candidate) => MediaRecorder.isTypeSupported(candidate)) ?? "";
}

export const recorderController = new RecorderController();
