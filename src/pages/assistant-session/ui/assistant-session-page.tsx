import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  ArrowDown,
  ArrowLeft,
  ArrowRight,
  Bot,
  Check,
  ChevronDown,
  FileSearch,
  GitCompareArrows,
  History,
  LoaderCircle,
  MessageCircle,
  Mic2,
  Paperclip,
  Pause,
  Plus,
  RefreshCw,
  Send,
  ShieldCheck,
  Square,
  TriangleAlert,
  X,
} from "lucide-react";
import {
  type FormEvent,
  type ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";

import {
  type AssistantEvent,
  type AssistantInputMode,
  type AssistantLivePreview,
  type AssistantSelectionAttachment,
  applyAssistantChangeSet,
  assistantEventsURL,
  assistantVoiceURL,
  createAssistantTurn,
  getAssistantRun,
  listAssistantTurns,
  markAssistantChangeSetStale,
  markAssistantRunViewed,
  refreshAssistantChangedDocuments,
  stopAssistantRun,
  undoAssistantChangeSet,
} from "~entities/assistant";
import { getProject } from "~entities/project";
import { LivePCMStream } from "~features/dictation";
import { APIError } from "~shared/api";
import { cn } from "~shared/lib";
import { AssistantMarkdown, Banner, Button, Spinner, useToast } from "~shared/ui";
import { RequireSession } from "~widgets/require-session";
import { assistantLivePreview } from "../model/assistant-live-preview";
import {
  type AssistantNativeAction,
  assistantFailureMessage,
  buildAssistantActivity,
  buildAssistantNativeActions,
} from "../model/assistant-presentation";
import { isAssistantTranscriptNearEnd } from "../model/assistant-transcript-scroll";

interface AssistantSessionPageProps {
  projectId: string;
  runId: string;
  embedded?: boolean;
  targetLabel?: string;
  selectionAttachment?: AssistantSelectionAttachment;
  previewStale?: boolean;
  onClose?: () => void;
  onOpenThreads?: () => void;
  onNewThread?: () => void;
  onReview?: (changeSetId: string) => void;
  onSelectionConsumed?: () => void;
  onLivePreviewChange?: (preview?: AssistantLivePreview) => void;
}

type VoiceState = "idle" | "connecting" | "listening" | "paused" | "confirming";

export function AssistantSessionPage({
  projectId,
  runId,
  embedded = false,
  targetLabel,
  selectionAttachment,
  previewStale = false,
  onClose,
  onOpenThreads,
  onNewThread,
  onReview,
  onSelectionConsumed,
  onLivePreviewChange,
}: AssistantSessionPageProps) {
  const queryClient = useQueryClient();
  const { showToast } = useToast();
  const assistantModeKey = `writers-studio-assistant-mode:${runId}`;
  const storedAssistantMode = localStorage.getItem(assistantModeKey);
  const [mode, setMode] = useState<Exclude<AssistantInputMode, "mcp">>(() =>
    storedAssistantMode === "chat" || storedAssistantMode === "voice"
      ? storedAssistantMode
      : "voice",
  );
  const chatDraftKey = `writers-studio-assistant-chat-draft:${runId}`;
  const [message, setMessage] = useState(() => localStorage.getItem(chatDraftKey) ?? "");
  const [voiceState, setVoiceState] = useState<VoiceState>("idle");
  const [transcript, setTranscript] = useState("");
  const [partial, setPartial] = useState("");
  const [events, setEvents] = useState<AssistantEvent[]>([]);
  const [voiceError, setVoiceError] = useState<string>();
  const socketRef = useRef<WebSocket | undefined>(undefined);
  const streamRef = useRef<MediaStream | undefined>(undefined);
  const pcmRef = useRef<LivePCMStream | undefined>(undefined);
  const voiceFlushPromiseRef = useRef<Promise<void> | undefined>(undefined);
  const voiceFlushResolverRef = useRef<(() => void) | undefined>(undefined);
  const voiceFlushTimerRef = useRef<number | undefined>(undefined);
  const voiceEditTimerRef = useRef<number | undefined>(undefined);
  const restoredAwaitingConfirmationRef = useRef(false);
  const voiceAttemptRef = useRef(0);
  const transcriptViewportRef = useRef<HTMLDivElement | null>(null);
  const transcriptFollowsActivityRef = useRef(true);
  const [showLatestActivity, setShowLatestActivity] = useState(false);
  const voiceClientAvailable =
    window.isSecureContext && Boolean(navigator.mediaDevices?.getUserMedia);

  const run = useQuery({
    queryKey: ["assistant-run", runId],
    queryFn: () => getAssistantRun(runId),
    refetchInterval: ({ state }) => (state.data?.run.status === "working" ? 1_200 : false),
  });
  const turns = useQuery({
    queryKey: ["assistant-turns", runId],
    queryFn: () => listAssistantTurns(runId),
    refetchInterval: run.data?.run.status === "working" ? 1_200 : false,
  });
  const project = useQuery({
    queryKey: ["project", projectId],
    queryFn: () => getProject(projectId),
  });

  useEffect(() => {
    if (message) localStorage.setItem(chatDraftKey, message);
    else localStorage.removeItem(chatDraftKey);
  }, [chatDraftKey, message]);

  useEffect(() => {
    if (!embedded || !run.data?.run.updatedAt) return;
    void markAssistantRunViewed(runId).then(() =>
      queryClient.invalidateQueries({ queryKey: ["assistant-runs", projectId] }),
    );
  }, [embedded, projectId, queryClient, run.data?.run.updatedAt, runId]);

  const latestTurn = turns.data?.turns.at(-1);
  const transcriptRevision = [
    mode,
    turns.data?.turns.length ?? 0,
    latestTurn?.id ?? "",
    latestTurn?.state ?? "",
    latestTurn?.assistantText?.length ?? 0,
    events.length,
    run.data?.run.status ?? "",
    run.data?.run.pendingChangeSet?.id ?? "",
    transcript.length,
    partial.length,
  ].join(":");

  const scrollTranscriptToEnd = useCallback((smooth = true) => {
    const viewport = transcriptViewportRef.current;
    if (!viewport) return;
    transcriptFollowsActivityRef.current = true;
    setShowLatestActivity(false);
    const reduceMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false;
    viewport.scrollTo({
      top: viewport.scrollHeight,
      behavior: smooth && !reduceMotion ? "smooth" : "auto",
    });
  }, []);

  const handleTranscriptScroll = useCallback(() => {
    const viewport = transcriptViewportRef.current;
    if (!viewport) return;
    const followsActivity = isAssistantTranscriptNearEnd(viewport);
    transcriptFollowsActivityRef.current = followsActivity;
    setShowLatestActivity(!followsActivity);
  }, []);

  useEffect(() => {
    // The revision deliberately re-runs this effect as transcript content grows.
    void transcriptRevision;
    if (!transcriptFollowsActivityRef.current) return;
    const frame = window.requestAnimationFrame(() => scrollTranscriptToEnd(false));
    return () => window.cancelAnimationFrame(frame);
  }, [scrollTranscriptToEnd, transcriptRevision]);

  useEffect(() => {
    const storedMode = localStorage.getItem(assistantModeKey);
    if (storedMode === "chat" || storedMode === "voice") {
      setMode(storedMode);
      return;
    }
    if (run.data?.run.inputMode) {
      setMode(run.data.run.inputMode === "mcp" ? "chat" : run.data.run.inputMode);
    }
  }, [assistantModeKey, run.data?.run.inputMode]);

  useEffect(() => {
    const source = new EventSource(assistantEventsURL(runId), { withCredentials: true });
    source.onmessage = (event) => {
      try {
        const parsed = JSON.parse(event.data) as AssistantEvent;
        setEvents((current) => [...current, parsed].slice(-60));
      } catch {
        // Event-specific listeners below receive named AG-UI events.
      }
    };
    const namedEvents = [
      "RUN_STARTED",
      "RUN_FINISHED",
      "RUN_ERROR",
      "STEP_STARTED",
      "STEP_FINISHED",
      "TOOL_CALL_START",
      "TOOL_CALL_END",
      "TOOL_CALL_RESULT",
      "writers.change_set.staged",
      "writers.change_set.applied",
      "writers.change_set.undone",
      "writers.change_set.rejected",
      "writers.change_set.stale",
      "writers.proposal.updated",
    ];
    const refreshEvents = new Set([
      "RUN_STARTED",
      "RUN_FINISHED",
      "RUN_ERROR",
      "writers.change_set.staged",
      "writers.change_set.applied",
      "writers.change_set.undone",
      "writers.change_set.rejected",
      "writers.change_set.stale",
    ]);
    const listener = (event: MessageEvent<string>) => {
      try {
        const parsed = JSON.parse(event.data) as AssistantEvent;
        setEvents((current) => [...current, parsed].slice(-60));
        if (refreshEvents.has(parsed.type)) {
          void queryClient.invalidateQueries({ queryKey: ["assistant-run", runId] });
          void queryClient.invalidateQueries({ queryKey: ["assistant-turns", runId] });
          void queryClient.invalidateQueries({ queryKey: ["assistant-runs", projectId] });
          if (
            parsed.type === "writers.change_set.applied" ||
            parsed.type === "writers.change_set.undone"
          ) {
            void refreshAssistantChangedDocuments(queryClient, projectId);
          }
        }
      } catch {
        // Ignore a malformed event and keep the durable stream alive.
      }
    };
    for (const type of namedEvents) source.addEventListener(type, listener as EventListener);
    return () => source.close();
  }, [projectId, queryClient, runId]);

  const sendChat = useMutation({
    mutationFn: (instruction: string) =>
      createAssistantTurn(runId, {
        instruction,
        inputMode: "chat",
        requireConfirmation: false,
        execute: true,
        selectionAttachment,
      }),
    onSuccess: () => {
      setMessage("");
      onSelectionConsumed?.();
      void queryClient.invalidateQueries({ queryKey: ["assistant-turns", runId] });
      void queryClient.invalidateQueries({ queryKey: ["assistant-run", runId] });
      void queryClient.invalidateQueries({ queryKey: ["assistant-runs", projectId] });
    },
    onError: (error) => showToast({ title: "Поручение не отправлено", description: error.message }),
  });
  const stopRun = useMutation({
    mutationFn: () => stopAssistantRun(runId),
    onSuccess: () =>
      void Promise.all([
        queryClient.invalidateQueries({ queryKey: ["assistant-run", runId] }),
        queryClient.invalidateQueries({ queryKey: ["assistant-runs", projectId] }),
      ]),
  });
  const staleMutation = useMutation({
    mutationFn: (changeSetId: string) => markAssistantChangeSetStale(runId, changeSetId),
    onSuccess: ({ changeSet }) => {
      void queryClient.invalidateQueries({ queryKey: ["assistant-run", runId] });
      void queryClient.invalidateQueries({ queryKey: ["assistant-runs", projectId] });
      void queryClient.invalidateQueries({
        queryKey: ["assistant-change-set", runId, changeSet.id],
      });
    },
  });
  const undoAppliedChange = useMutation({
    mutationFn: (changeSetId: string) => undoAssistantChangeSet(runId, changeSetId),
    onSuccess: async ({ changeSet }) => {
      await refreshAssistantChangedDocuments(queryClient, projectId, changeSet);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["assistant-run", runId] }),
        queryClient.invalidateQueries({ queryKey: ["assistant-runs", projectId] }),
        queryClient.invalidateQueries({
          queryKey: ["assistant-change-set", runId, changeSet.id],
        }),
      ]);
      showToast({ title: "Изменение отменено" });
    },
    onError: (error) =>
      showToast({ title: "Не удалось отменить изменение", description: error.message }),
  });
  const applyReadyChange = useMutation({
    mutationFn: ({
      changeSetId,
      selectedGroups,
    }: {
      changeSetId: string;
      selectedGroups: string[];
    }) => applyAssistantChangeSet(runId, changeSetId, selectedGroups),
    onSuccess: async ({ changeSet }) => {
      await refreshAssistantChangedDocuments(queryClient, projectId, changeSet);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["assistant-run", runId] }),
        queryClient.invalidateQueries({ queryKey: ["assistant-runs", projectId] }),
        queryClient.invalidateQueries({
          queryKey: ["assistant-change-set", runId, changeSet.id],
        }),
      ]);
      showToast({
        title: "Изменение применено",
        description: "Рукопись обновлена без выхода из Studio.",
        actionLabel: "Отменить",
        onAction: () => undoAppliedChange.mutate(changeSet.id),
      });
    },
    onError: (error) => {
      if (error instanceof APIError && error.code === "stale_change_set") {
        void Promise.all([
          queryClient.invalidateQueries({ queryKey: ["assistant-run", runId] }),
          queryClient.invalidateQueries({ queryKey: ["assistant-runs", projectId] }),
          queryClient.invalidateQueries({ queryKey: ["project", projectId] }),
        ]);
        showToast({
          title: "Предложение устарело",
          description: "Другой чат или ручная правка уже изменили рукопись.",
        });
        return;
      }
      showToast({ title: "Изменение не применено", description: error.message });
    },
  });
  const pendingBaseStale = useMemo(() => {
    const pending = run.data?.run.pendingChangeSet;
    if (!pending || !project.data) return false;
    return pending.operations.some((operation) => {
      if (!operation.targetId || operation.baseRevision === undefined) return false;
      const target = project.data.nodes.find((node) => node.id === operation.targetId);
      return Boolean(target && target.currentRevision !== operation.baseRevision);
    });
  }, [project.data, run.data?.run.pendingChangeSet]);
  const latestChangeSet = run.data?.run.latestChangeSet;
  const durableStale = Boolean(
    latestChangeSet?.state === "stale" && latestChangeSet.turnId === run.data?.run.latestTurn?.id,
  );
  const effectivePreviewStale = previewStale || pendingBaseStale || durableStale;
  const refreshStaleProposal = useMutation({
    mutationFn: () => {
      const staleTurnId = latestChangeSet?.turnId;
      const originalTurn =
        turns.data?.turns.find((turn) => turn.id === staleTurnId) ?? turns.data?.turns.at(-1);
      const originalInstruction = originalTurn?.instruction.trim();
      if (!originalInstruction) throw new Error("Исходное поручение недоступно");
      return createAssistantTurn(runId, {
        instruction:
          `Обнови предложение по текущей версии рукописи. Исходное поручение: ${originalInstruction}`.slice(
            0,
            32_000,
          ),
        inputMode: "chat",
        requireConfirmation: false,
        execute: true,
        selectionAttachment: originalTurn?.selectionAttachment,
      });
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["assistant-run", runId] }),
        queryClient.invalidateQueries({ queryKey: ["assistant-turns", runId] }),
        queryClient.invalidateQueries({ queryKey: ["assistant-runs", projectId] }),
      ]);
      showToast({
        title: "Предложение обновляется",
        description: "Другие чаты продолжают работу.",
      });
    },
    onError: (error) =>
      showToast({ title: "Не удалось обновить предложение", description: error.message }),
  });
  const staleChangeSetRef = useRef("");
  useEffect(() => {
    const changeSetId = run.data?.run.pendingChangeSet?.id;
    if (
      !(previewStale || pendingBaseStale) ||
      !changeSetId ||
      staleChangeSetRef.current === changeSetId
    ) {
      return;
    }
    staleChangeSetRef.current = changeSetId;
    staleMutation.mutate(changeSetId);
  }, [pendingBaseStale, previewStale, run.data?.run.pendingChangeSet?.id, staleMutation]);

  const closeVoice = useCallback(async () => {
    await pcmRef.current?.stop().catch(() => undefined);
    pcmRef.current = undefined;
    for (const track of streamRef.current?.getTracks() ?? []) track.stop();
    streamRef.current = undefined;
  }, []);

  const resolveVoiceFlush = useCallback(() => {
    window.clearTimeout(voiceFlushTimerRef.current);
    voiceFlushTimerRef.current = undefined;
    voiceFlushResolverRef.current?.();
    voiceFlushResolverRef.current = undefined;
    voiceFlushPromiseRef.current = undefined;
  }, []);

  useEffect(
    () => () => {
      voiceAttemptRef.current += 1;
      void closeVoice();
      resolveVoiceFlush();
      window.clearTimeout(voiceEditTimerRef.current);
      socketRef.current?.close();
    },
    [closeVoice, resolveVoiceFlush],
  );

  const ensureSocket = useCallback(async () => {
    const existing = socketRef.current;
    if (existing?.readyState === WebSocket.OPEN) {
      existing.send(JSON.stringify({ type: "set_selection", selectionAttachment }));
      return existing;
    }
    const socket = new WebSocket(assistantVoiceURL(runId));
    socketRef.current = socket;
    let resolveReady: (() => void) | undefined;
    const ready = new Promise<void>((resolve) => {
      resolveReady = resolve;
    });
    socket.onmessage = (event) => {
      const payload = JSON.parse(String(event.data)) as {
        type: string;
        text?: string;
        transcript?: string;
        prompt?: string;
        started?: boolean;
        reason?: string;
        state?: string;
        label?: string;
      };
      if (payload.type === "voice_ready") resolveReady?.();
      if (payload.type === "partial") setPartial(payload.text ?? "");
      if (payload.type === "segment_final" || payload.type === "transcript_updated") {
        setTranscript(payload.transcript ?? "");
        setPartial("");
      }
      if (payload.type === "draft_restored") {
        setTranscript(payload.transcript ?? "");
        setPartial("");
        restoredAwaitingConfirmationRef.current = payload.state === "awaiting_confirmation";
        setVoiceState(payload.state === "awaiting_confirmation" ? "confirming" : "paused");
      }
      if (payload.type === "confirmation_required") {
        restoredAwaitingConfirmationRef.current = true;
        setTranscript(payload.transcript ?? "");
        setVoiceState("confirming");
        void closeVoice();
      }
      if (payload.type === "turn_queued") {
        restoredAwaitingConfirmationRef.current = false;
        setVoiceState("idle");
        setTranscript("");
        setPartial("");
        void closeVoice();
        socket.close();
        void queryClient.invalidateQueries({ queryKey: ["assistant-turns", runId] });
        void queryClient.invalidateQueries({ queryKey: ["assistant-run", runId] });
      }
      if (payload.type === "draft_discarded") {
        restoredAwaitingConfirmationRef.current = false;
        setVoiceState("idle");
        setTranscript("");
        setPartial("");
        void queryClient.invalidateQueries({ queryKey: ["assistant-turns", runId] });
      }
      if (payload.type === "stream_degraded") {
        setVoiceError("Распознавание речи временно недоступно. Переключитесь в чат.");
      }
      if (payload.type === "live_flushed") resolveVoiceFlush();
      if (payload.type === "confirmation_dismissed") {
        restoredAwaitingConfirmationRef.current = false;
        setVoiceState("listening");
      }
      if (payload.type === "command_recognized") {
        showToast({ title: payload.label ?? "Голосовая команда распознана" });
      }
      if (payload.type === "command_rejected") {
        showToast({
          title: "Команда не выполнена",
          description: "Сначала надиктуйте поручение.",
        });
      }
    };
    socket.onerror = () => setVoiceError("Не удалось открыть голосовой поток.");
    socket.onclose = () => {
      resolveVoiceFlush();
      if (socketRef.current === socket) socketRef.current = undefined;
    };
    await new Promise<void>((resolve, reject) => {
      socket.onopen = () => resolve();
      const timer = window.setTimeout(() => reject(new Error("voice socket timeout")), 10_000);
      socket.addEventListener("open", () => window.clearTimeout(timer), { once: true });
      socket.addEventListener(
        "error",
        () => {
          window.clearTimeout(timer);
          reject(new Error("voice socket failed"));
        },
        { once: true },
      );
    });
    await Promise.race([
      ready,
      new Promise<never>((_, reject) =>
        window.setTimeout(() => reject(new Error("voice ready timeout")), 10_000),
      ),
    ]);
    socket.send(JSON.stringify({ type: "set_selection", selectionAttachment }));
    return socket;
  }, [closeVoice, queryClient, resolveVoiceFlush, runId, selectionAttachment, showToast]);

  useEffect(() => {
    const socket = socketRef.current;
    if (socket?.readyState === WebSocket.OPEN) {
      socket.send(JSON.stringify({ type: "set_selection", selectionAttachment }));
    }
  }, [selectionAttachment]);

  const flushVoice = useCallback(async () => {
    const socket = socketRef.current;
    if (!socket || socket.readyState !== WebSocket.OPEN) return;
    if (voiceFlushPromiseRef.current) {
      await voiceFlushPromiseRef.current;
      return;
    }
    voiceFlushPromiseRef.current = new Promise<void>((resolve) => {
      voiceFlushResolverRef.current = resolve;
      voiceFlushTimerRef.current = window.setTimeout(resolveVoiceFlush, 5_000);
    });
    socket.send(JSON.stringify({ type: "flush_live" }));
    await voiceFlushPromiseRef.current;
  }, [resolveVoiceFlush]);

  const updateVoiceTranscript = (value: string) => {
    setTranscript(value);
    setPartial("");
    window.clearTimeout(voiceEditTimerRef.current);
    if (!value.trim() || voiceState === "listening" || voiceState === "connecting") return;
    voiceEditTimerRef.current = window.setTimeout(() => {
      void ensureSocket()
        .then((socket) => {
          socket.send(JSON.stringify({ type: "edit_transcript", text: value }));
        })
        .catch(() => {
          setVoiceError("Не удалось сохранить исправление стенограммы.");
        });
    }, 250);
  };

  const startListening = async () => {
    const attempt = voiceAttemptRef.current + 1;
    voiceAttemptRef.current = attempt;
    setVoiceError(undefined);
    if (!voiceClientAvailable) {
      setVoiceError("Для голоса откройте приложение по HTTPS или через localhost.");
      return;
    }
    setVoiceState("connecting");
    try {
      const continuingCheckpoint = voiceState === "confirming";
      const socket = await ensureSocket();
      if (voiceAttemptRef.current !== attempt) return;
      if (restoredAwaitingConfirmationRef.current) {
        if (!continuingCheckpoint) {
          setVoiceState("confirming");
          return;
        }
        restoredAwaitingConfirmationRef.current = false;
        socket.send(JSON.stringify({ type: "continue_collecting" }));
      }
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: {
          channelCount: 1,
          echoCancellation: true,
          noiseSuppression: true,
          autoGainControl: true,
        },
      });
      if (voiceAttemptRef.current !== attempt) {
        for (const track of stream.getTracks()) track.stop();
        return;
      }
      streamRef.current = stream;
      const pcm = new LivePCMStream(
        (chunk) => {
          if (socket.readyState !== WebSocket.OPEN) return;
          socket.send(
            JSON.stringify({
              type: "audio_chunk",
              sequence: chunk.sequence,
              format: chunk.format,
              sampleRate: chunk.sampleRate,
              startMs: chunk.startMs,
              endMs: chunk.endMs,
            }),
          );
          socket.send(chunk.data);
        },
        250,
        "linear16",
      );
      pcmRef.current = pcm;
      await pcm.start(stream);
      if (voiceAttemptRef.current !== attempt) {
        await pcm.stop().catch(() => undefined);
        for (const track of stream.getTracks()) track.stop();
        if (pcmRef.current === pcm) pcmRef.current = undefined;
        if (streamRef.current === stream) streamRef.current = undefined;
        return;
      }
      setVoiceState("listening");
    } catch {
      if (voiceAttemptRef.current !== attempt) return;
      await closeVoice();
      setVoiceState("idle");
      setVoiceError("Разрешите доступ к микрофону или используйте чат.");
    }
  };

  const pauseListening = async () => {
    voiceAttemptRef.current += 1;
    window.clearTimeout(voiceEditTimerRef.current);
    await closeVoice();
    await flushVoice();
    socketRef.current?.close();
    setVoiceState("paused");
  };

  const selectMode = async (nextMode: Exclude<AssistantInputMode, "mcp">) => {
    if (nextMode === mode) return;
    if (nextMode === "chat" && (voiceState === "listening" || voiceState === "connecting")) {
      await pauseListening();
    }
    localStorage.setItem(assistantModeKey, nextMode);
    setMode(nextMode);
  };

  const resolveVoiceDraft = async (action: "transfer" | "discard") => {
    window.clearTimeout(voiceEditTimerRef.current);
    const draft = [transcript.trim(), partial.trim()].filter(Boolean).join(" ");
    try {
      const socket = await ensureSocket();
      socket.send(JSON.stringify({ type: "discard" }));
      if (action === "transfer") setMessage(draft);
      setTranscript("");
      setPartial("");
      setVoiceState("idle");
      void queryClient.invalidateQueries({ queryKey: ["assistant-turns", runId] });
    } catch {
      setVoiceError("Не удалось закрыть голосовой черновик. Вернитесь к голосу и повторите.");
      localStorage.setItem(assistantModeKey, "voice");
      setMode("voice");
    }
  };

  const executeVoice = async () => {
    window.clearTimeout(voiceEditTimerRef.current);
    const text = transcript.trim();
    if (!text) return;
    await closeVoice();
    const socket = await ensureSocket();
    await flushVoice();
    socket.send(JSON.stringify({ type: "set_selection", selectionAttachment }));
    socket.send(JSON.stringify({ type: "edit_transcript", text }));
    socket.send(JSON.stringify({ type: "confirm_execute" }));
    onSelectionConsumed?.();
  };

  const livePreview = useMemo(
    () => assistantLivePreview(run.data?.run, events, effectivePreviewStale),
    [effectivePreviewStale, events, run.data?.run],
  );
  const reportedPreviewKeyRef = useRef("");
  useEffect(() => {
    if (!onLivePreviewChange) return;
    const key = livePreview ? `${livePreview.operationId}:${String(livePreview.stale)}` : "none";
    if (reportedPreviewKeyRef.current === key) return;
    reportedPreviewKeyRef.current = key;
    onLivePreviewChange(livePreview);
  }, [livePreview, onLivePreviewChange]);

  if (run.isPending || project.isPending) return <Spinner label="Открываем ассистента…" />;
  if (run.isError || !run.data || project.isError || !project.data) {
    return (
      <Banner
        variant="danger"
        title="Сессия недоступна"
        body="Вернитесь в проект и начните новую."
      />
    );
  }
  const currentRun = run.data.run;
  const isRemoteMCP = currentRun.origin === "remote_mcp";
  const remoteAgentLabel = isRemoteMCP
    ? `Внешний агент · ${currentRun.agent?.clientName ?? currentRun.provider.name}`
    : undefined;
  const targetNode = currentRun.targetNodeId
    ? project.data.nodes.find((node) => node.id === currentRun.targetNodeId)
    : undefined;
  const isWorking = currentRun.status === "working";
  const pending = currentRun.pendingChangeSet;
  const conflictChangeSet = durableStale
    ? currentRun.latestChangeSet
    : pendingBaseStale
      ? pending
      : undefined;
  const conflictOperation = conflictChangeSet?.operations.find(
    (operation) => operation.targetId && operation.baseRevision !== undefined,
  );
  const conflictTargetId = conflictOperation?.targetId ?? livePreview?.targetNodeId;
  const conflictTarget = conflictTargetId
    ? project.data.nodes.find((node) => node.id === conflictTargetId)
    : targetNode;
  const conflictBaseRevision = conflictOperation?.baseRevision ?? livePreview?.baseRevision;
  const conflictCurrentRevision = conflictTarget?.currentRevision;
  const turnList = turns.data?.turns ?? [];
  const voiceDraft = [transcript.trim(), partial.trim()].filter(Boolean).join(" ");

  const submitChat = (event: FormEvent) => {
    event.preventDefault();
    if (message.trim() && !isWorking) {
      transcriptFollowsActivityRef.current = true;
      setShowLatestActivity(false);
      sendChat.mutate(message.trim());
    }
  };

  if (embedded) {
    return (
      <RequireSession>
        <aside
          aria-label="Панель ассистента"
          data-testid="assistant-docked-panel"
          className="flex h-full min-h-0 w-full flex-col border-l border-[var(--border-subtle)] bg-[var(--surface-panel)] xl:w-[360px] xl:shrink-0"
        >
          <header className="flex h-14 shrink-0 items-center gap-2 border-b border-[var(--border-subtle)] px-3">
            <Bot className="size-4 text-[var(--accent)]" aria-hidden="true" />
            <button
              type="button"
              onClick={onOpenThreads}
              disabled={!onOpenThreads}
              className="min-w-0 flex-1 rounded-[var(--radius-sm)] px-1 py-1 text-left outline-none hover:bg-[var(--surface-subtle)] focus-visible:outline-2 focus-visible:outline-[var(--focus)] disabled:hover:bg-transparent"
              aria-label="Открыть список чатов проекта"
            >
              <span className="block truncate text-[9px] font-semibold tracking-[0.1em] text-[var(--ink-tertiary)] uppercase">
                {remoteAgentLabel ?? "Ассистент"} ·{" "}
                {isWorking ? "работает" : durableStale ? "нужно обновить" : "готов"}
              </span>
              <span className="flex min-w-0 items-center gap-1 text-sm font-semibold">
                <span className="truncate">{currentRun.title || "Новый чат"}</span>
                {onOpenThreads ? (
                  <ChevronDown className="size-3.5 shrink-0" aria-hidden="true" />
                ) : null}
              </span>
            </button>
            {isWorking && !isRemoteMCP ? (
              <Button
                variant="ghost"
                size="icon"
                aria-label="Остановить ассистента"
                onClick={() => stopRun.mutate()}
                disabled={stopRun.isPending}
              >
                <Square className="size-3.5" aria-hidden="true" />
              </Button>
            ) : null}
            {onNewThread ? (
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label="Новый чат"
                onClick={onNewThread}
              >
                <Plus className="size-4" aria-hidden="true" />
              </Button>
            ) : null}
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label="Закрыть панель ассистента"
              onClick={() => {
                if (voiceState === "listening" || voiceState === "connecting") {
                  void pauseListening();
                }
                onClose?.();
              }}
            >
              <X className="size-4" aria-hidden="true" />
            </Button>
          </header>

          <div className="border-b border-[var(--border-subtle)] px-3 py-3">
            <fieldset
              className="grid grid-cols-2 rounded-full border border-[var(--border-subtle)] bg-[var(--surface-subtle)] p-1"
              aria-label="Режим общения"
              disabled={isRemoteMCP}
            >
              <ModeButton
                active={mode === "voice"}
                icon={Mic2}
                label="Голосом"
                disabled={!voiceClientAvailable}
                onClick={() => void selectMode("voice")}
              />
              <ModeButton
                active={mode === "chat"}
                icon={MessageCircle}
                label="Чат"
                onClick={() => void selectMode("chat")}
              />
            </fieldset>
            <p className="mt-2 truncate text-center text-[10px] text-[var(--ink-tertiary)]">
              {targetNode?.title ?? targetLabel ?? "Весь проект"} ·{" "}
              {remoteAgentLabel ?? currentRun.provider.model}
            </p>
            {selectionAttachment ? (
              <div className="mt-3 flex items-start gap-2 rounded-[var(--radius-sm)] bg-[var(--accent-soft)] px-3 py-2 text-[11px]">
                <Paperclip
                  className="mt-0.5 size-3.5 shrink-0 text-[var(--accent)]"
                  aria-hidden="true"
                />
                <div className="min-w-0">
                  <p className="font-semibold text-[var(--accent-ink)]">Фрагмент прикреплён</p>
                  <p className="mt-0.5 line-clamp-2 text-[var(--ink-secondary)]">
                    {selectionAttachment.quote}
                  </p>
                </div>
              </div>
            ) : null}
          </div>

          <section
            aria-label="Стенограмма сессии"
            className="relative min-h-0 flex-1 overflow-hidden bg-[var(--surface-paper)]"
          >
            <div
              ref={transcriptViewportRef}
              data-testid="assistant-transcript-viewport"
              onScroll={handleTranscriptScroll}
              className="h-full min-h-0 overflow-y-auto overscroll-contain px-4 py-4 [scrollbar-gutter:stable]"
            >
              <Conversation
                compact
                turns={turnList}
                working={isWorking}
                events={events}
                variant={mode}
              />
              {mode === "voice" && !isRemoteMCP && !isWorking && !pending ? (
                <div className="mt-5 border-t border-[var(--border-subtle)] pt-5">
                  <VoiceComposer
                    compact
                    state={voiceState}
                    transcript={transcript}
                    partial={partial}
                    error={voiceError}
                    disabled={!voiceClientAvailable}
                    onTranscript={updateVoiceTranscript}
                    onStart={() => void startListening()}
                    onPause={() => void pauseListening()}
                    onExecute={executeVoice}
                  />
                </div>
              ) : null}
              <div className="h-px" aria-hidden="true" />
            </div>
            {showLatestActivity ? (
              <button
                type="button"
                onClick={() => scrollTranscriptToEnd()}
                className="absolute right-3 bottom-3 inline-flex min-h-9 items-center gap-2 rounded-full border border-[var(--border-strong)] bg-[var(--surface-paper)] px-3 text-[11px] font-medium shadow-lg"
              >
                <ArrowDown className="size-3.5" aria-hidden="true" />К новым
              </button>
            ) : null}
          </section>

          <div className="space-y-3 border-t border-[var(--border-subtle)] bg-[var(--surface-panel)] p-3">
            {effectivePreviewStale ? (
              <div
                className="rounded-[var(--radius-md)] border border-[var(--warning)] bg-[var(--warning-soft)] p-3"
                data-testid="assistant-stale-change-set"
              >
                <div className="flex items-center gap-2 text-[10px] font-bold tracking-[0.08em] text-[var(--warning)] uppercase">
                  <GitCompareArrows className="size-3.5" aria-hidden="true" />
                  Конфликт версий
                </div>
                <p className="mt-2 text-sm font-semibold">Предложение устарело</p>
                <p className="mt-1 text-[11px] leading-4 text-[var(--ink-secondary)]">
                  Другой чат или ручная правка уже изменили {conflictTarget?.title ?? "рукопись"}.
                  Старый preview применить нельзя.
                </p>
                <div className="mt-2 flex items-center gap-2 rounded-[var(--radius-sm)] bg-[var(--surface-paper)] px-2 py-1.5 text-[10px] text-[var(--warning)]">
                  <RefreshCw className="size-3" aria-hidden="true" />
                  Основа r{conflictBaseRevision ?? "—"} · сейчас r{conflictCurrentRevision ?? "—"}
                </div>
                <Button
                  type="button"
                  className="mt-3 w-full"
                  onClick={() => refreshStaleProposal.mutate()}
                  disabled={refreshStaleProposal.isPending || isWorking}
                >
                  <RefreshCw className="size-4" aria-hidden="true" />
                  {refreshStaleProposal.isPending ? "Обновляем…" : "Обновить предложение"}
                </Button>
                {onOpenThreads ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="compact"
                    className="mt-1 w-full"
                    onClick={onOpenThreads}
                  >
                    Отложить и открыть другие чаты
                  </Button>
                ) : null}
                <p className="mt-2 text-[10px] leading-4 text-[var(--ink-tertiary)]">
                  Конфликт относится только к этому предложению; остальные чаты продолжают работу.
                </p>
              </div>
            ) : pending ? (
              <div
                className="rounded-[var(--radius-md)] border border-[var(--info)] bg-[var(--info-soft)] p-3"
                data-testid="assistant-change-set-card"
              >
                <p className="text-sm font-semibold">Изменение готово к проверке</p>
                <p className="mt-1 text-[11px] text-[var(--ink-secondary)]">
                  {pending.operations.length} операций · рукопись ещё не изменена
                </p>
                {onReview ? (
                  <div className="mt-3 grid grid-cols-2 gap-2">
                    <Button variant="secondary" onClick={() => onReview(pending.id)}>
                      Проверить изменения
                    </Button>
                    <Button
                      onClick={() =>
                        applyReadyChange.mutate({
                          changeSetId: pending.id,
                          selectedGroups: [
                            ...new Set(pending.operations.map((operation) => operation.groupKey)),
                          ],
                        })
                      }
                      disabled={applyReadyChange.isPending}
                    >
                      <Check className="size-4" aria-hidden="true" />
                      {applyReadyChange.isPending ? "Применяем…" : "Применить сразу"}
                    </Button>
                  </div>
                ) : (
                  <Button asChild className="mt-3 w-full" variant="secondary">
                    <Link
                      to="/projects/$projectId/assistant/$runId/review/$changeSetId"
                      params={{ projectId, runId, changeSetId: pending.id }}
                    >
                      Проверить изменения
                      <ArrowRight className="size-4" aria-hidden="true" />
                    </Link>
                  </Button>
                )}
              </div>
            ) : null}

            {mode === "chat" && !isRemoteMCP ? (
              <form onSubmit={submitChat}>
                {voiceDraft ? (
                  <div className="mb-3 rounded-[var(--radius-md)] border border-[var(--warning)]/35 bg-[var(--warning-soft)] p-3">
                    <p className="text-xs font-semibold">Голосовой черновик сохранён</p>
                    <p className="mt-1 line-clamp-2 text-[11px] text-[var(--ink-secondary)]">
                      {voiceDraft}
                    </p>
                    <div className="mt-2 flex flex-wrap gap-1">
                      <Button
                        type="button"
                        size="compact"
                        onClick={() => void resolveVoiceDraft("transfer")}
                      >
                        Перенести в чат
                      </Button>
                      <Button
                        type="button"
                        size="compact"
                        variant="ghost"
                        onClick={() => void selectMode("voice")}
                      >
                        Вернуться
                      </Button>
                      <Button
                        type="button"
                        size="compact"
                        variant="ghost"
                        onClick={() => void resolveVoiceDraft("discard")}
                      >
                        Удалить
                      </Button>
                    </div>
                  </div>
                ) : null}
                <label htmlFor="assistant-message-docked" className="sr-only">
                  Поручение ассистенту
                </label>
                <textarea
                  id="assistant-message-docked"
                  value={message}
                  onChange={(event) => setMessage(event.target.value)}
                  placeholder="Напишите, что изменить в рукописи…"
                  rows={3}
                  maxLength={32_000}
                  className="w-full resize-none rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] px-3 py-2 text-sm leading-5 outline-none focus:border-[var(--focus)]"
                  disabled={isWorking || Boolean(pending) || Boolean(voiceDraft)}
                  onKeyDown={(event) => {
                    if ((event.ctrlKey || event.metaKey) && event.key === "Enter") {
                      event.preventDefault();
                      event.currentTarget.form?.requestSubmit();
                    }
                  }}
                />
                <div className="mt-2 flex items-center justify-between gap-2">
                  <span className="text-[10px] text-[var(--ink-tertiary)]">⌘/Ctrl + Enter</span>
                  <Button
                    type="submit"
                    size="compact"
                    disabled={
                      !message.trim() ||
                      isWorking ||
                      Boolean(pending) ||
                      Boolean(voiceDraft) ||
                      sendChat.isPending
                    }
                  >
                    <Send className="size-3.5" aria-hidden="true" />
                    Отправить
                  </Button>
                </div>
              </form>
            ) : null}
          </div>
        </aside>
      </RequireSession>
    );
  }

  return (
    <RequireSession>
      <div className="flex h-dvh min-h-0 flex-col overflow-hidden bg-[var(--surface-app)]">
        <header className="flex h-14 shrink-0 items-center gap-3 border-b border-[var(--border-subtle)] bg-[var(--surface-panel)] px-4">
          <Button
            asChild
            variant="ghost"
            size="icon"
            aria-label={targetNode ? "Вернуться в студию" : "Вернуться в проект"}
          >
            <Link
              to={targetNode ? "/projects/$projectId/studio/$nodeId" : "/projects/$projectId"}
              params={targetNode ? { projectId, nodeId: targetNode.id } : { projectId }}
            >
              <ArrowLeft className="size-4" aria-hidden="true" />
            </Link>
          </Button>
          <Bot className="size-5 text-[var(--accent)]" aria-hidden="true" />
          <div className="min-w-0">
            <p className="truncate text-sm font-semibold">
              {remoteAgentLabel ?? "Ассистент"} · {project.data.project.title}
            </p>
            <p className="truncate text-[11px] text-[var(--ink-tertiary)]">
              {targetNode ? `Пишет в «${targetNode.title}»` : "Работает по всему проекту"} · читает{" "}
              {project.data.project.structure.rootLabel.toLocaleLowerCase()}
            </p>
          </div>
          <div className="ml-auto flex items-center gap-2">
            <span className="hidden rounded-full bg-[var(--accent-soft)] px-3 py-1 text-[11px] text-[var(--accent-ink)] sm:inline">
              Guide · r{currentRun.latestTurn?.guideRevision ?? currentRun.guideRevision ?? "—"}
            </span>
            {isWorking && !isRemoteMCP ? (
              <Button
                variant="secondary"
                size="compact"
                onClick={() => stopRun.mutate()}
                disabled={stopRun.isPending}
              >
                <Square className="size-3.5" aria-hidden="true" />
                Остановить
              </Button>
            ) : null}
          </div>
        </header>

        <main className="mx-auto grid min-h-0 w-full max-w-[1120px] flex-1 grid-rows-[auto_minmax(0,1fr)_auto] gap-4 px-4 py-4 sm:px-8 sm:py-5">
          <div className="space-y-3">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <fieldset
                className="inline-flex rounded-full border border-[var(--border-subtle)] bg-[var(--surface-subtle)] p-1"
                aria-label="Режим общения"
                disabled={isRemoteMCP}
              >
                <ModeButton
                  active={mode === "voice"}
                  icon={Mic2}
                  label="Голосом"
                  disabled={!voiceClientAvailable}
                  onClick={() => void selectMode("voice")}
                />
                <ModeButton
                  active={mode === "chat"}
                  icon={MessageCircle}
                  label="Чат"
                  onClick={() => void selectMode("chat")}
                />
              </fieldset>
              <div className="flex gap-2 text-[11px] text-[var(--ink-tertiary)]">
                <span className="rounded-full bg-[var(--surface-subtle)] px-3 py-2">
                  {remoteAgentLabel ?? `${currentRun.provider.name} · ${currentRun.provider.model}`}
                </span>
                <span className="rounded-full bg-[var(--surface-subtle)] px-3 py-2">
                  {currentRun.targetKind === "node" ? "Точная цель" : "Весь проект"}
                </span>
              </div>
            </div>

            {!voiceClientAvailable ? (
              <Banner
                variant="warning"
                title="Голос заблокирован текущим адресом"
                body="Браузер разрешает микрофон только по HTTPS или на localhost. Чат и проверка изменений продолжают работать без микрофона."
                action={
                  <div className="flex flex-wrap gap-2">
                    <Button
                      type="button"
                      variant="secondary"
                      size="compact"
                      onClick={() => {
                        localStorage.setItem(assistantModeKey, "chat");
                        setMode("chat");
                      }}
                    >
                      Открыть чат
                    </Button>
                    <Button asChild variant="secondary" size="compact">
                      <a href={assistantLocalhostURL()}>Открыть localhost</a>
                    </Button>
                  </div>
                }
              />
            ) : null}
          </div>

          <section
            aria-label="Стенограмма сессии"
            className="relative min-h-0 overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-paper)]"
          >
            <div
              ref={transcriptViewportRef}
              data-testid="assistant-transcript-viewport"
              onScroll={handleTranscriptScroll}
              className="h-full min-h-0 overflow-y-auto overscroll-contain px-5 py-5 [scrollbar-gutter:stable] sm:px-8 sm:py-7"
            >
              {mode === "voice" && !isRemoteMCP ? (
                <div className="space-y-7">
                  {turnList.length > 0 ? (
                    <Conversation
                      turns={turnList}
                      working={isWorking}
                      events={events}
                      variant="voice"
                      showEmpty={false}
                    />
                  ) : null}
                  {!isWorking && !pending ? (
                    <div
                      className={cn(
                        turnList.length > 0 && "border-t border-[var(--border-subtle)] pt-7",
                      )}
                    >
                      <VoiceComposer
                        state={voiceState}
                        transcript={transcript}
                        partial={partial}
                        error={voiceError}
                        disabled={!voiceClientAvailable}
                        onTranscript={updateVoiceTranscript}
                        onStart={() => void startListening()}
                        onPause={() => void pauseListening()}
                        onExecute={executeVoice}
                      />
                    </div>
                  ) : null}
                </div>
              ) : (
                <Conversation turns={turnList} working={isWorking} events={events} />
              )}
              <div className="h-px" aria-hidden="true" />
            </div>
            {showLatestActivity ? (
              <button
                type="button"
                onClick={() => scrollTranscriptToEnd()}
                className="absolute right-4 bottom-4 inline-flex min-h-10 items-center gap-2 rounded-full border border-[var(--border-strong)] bg-[var(--surface-paper)] px-4 text-xs font-medium text-[var(--accent-ink)] shadow-lg focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus)]"
              >
                <ArrowDown className="size-4" aria-hidden="true" />К новым сообщениям
              </button>
            ) : null}
          </section>

          <div className="space-y-3">
            {pending ? (
              <div className="flex flex-col gap-4 rounded-[var(--radius-md)] border border-[var(--info)] bg-[var(--info-soft)] p-4 sm:flex-row sm:items-center">
                <span className="grid size-10 shrink-0 place-items-center rounded bg-[var(--surface-paper)] text-[var(--info)]">
                  <FileSearch className="size-5" aria-hidden="true" />
                </span>
                <div className="min-w-0 flex-1">
                  <p className="font-semibold">Изменение готово к проверке</p>
                  <p className="mt-1 text-[12px] text-[var(--ink-secondary)]">
                    {pending.operations.length} операций · пока ничего не применено
                  </p>
                </div>
                <Button asChild variant="secondary">
                  <Link
                    to="/projects/$projectId/assistant/$runId/review/$changeSetId"
                    params={{ projectId, runId, changeSetId: pending.id }}
                  >
                    Проверить
                    <ArrowRight className="size-4" aria-hidden="true" />
                  </Link>
                </Button>
              </div>
            ) : null}

            {mode === "chat" && !isRemoteMCP ? (
              <form
                onSubmit={submitChat}
                className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-4"
              >
                {voiceDraft ? (
                  <div className="mb-4 rounded-[var(--radius-md)] border border-[var(--warning)]/35 bg-[var(--warning-soft)] p-3">
                    <p className="text-sm font-semibold">Голосовой черновик сохранён</p>
                    <p className="mt-1 line-clamp-2 text-[12px] leading-5 text-[var(--ink-secondary)]">
                      {voiceDraft}
                    </p>
                    <div className="mt-3 flex flex-wrap gap-2">
                      <Button
                        type="button"
                        size="compact"
                        onClick={() => void resolveVoiceDraft("transfer")}
                      >
                        Перенести в чат
                      </Button>
                      <Button
                        type="button"
                        size="compact"
                        variant="secondary"
                        onClick={() => void selectMode("voice")}
                      >
                        Вернуться к голосу
                      </Button>
                      <Button
                        type="button"
                        size="compact"
                        variant="ghost"
                        onClick={() => void resolveVoiceDraft("discard")}
                      >
                        Удалить черновик
                      </Button>
                    </div>
                  </div>
                ) : null}
                <label htmlFor="assistant-message" className="sr-only">
                  Поручение ассистенту
                </label>
                <textarea
                  id="assistant-message"
                  value={message}
                  onChange={(event) => setMessage(event.target.value)}
                  placeholder="Напишите поручение: «Сделай диалог менее прямолинейным, но не меняй последнюю реплику»"
                  rows={3}
                  maxLength={32_000}
                  className="w-full resize-none bg-transparent text-[14px] leading-[1.5] outline-none placeholder:text-[var(--ink-tertiary)]"
                  disabled={isWorking || Boolean(pending) || Boolean(voiceDraft)}
                  onKeyDown={(event) => {
                    if ((event.ctrlKey || event.metaKey) && event.key === "Enter") {
                      event.preventDefault();
                      event.currentTarget.form?.requestSubmit();
                    }
                  }}
                />
                <div className="mt-3 flex items-center justify-between">
                  <p className="text-[11px] text-[var(--ink-tertiary)]">
                    Ctrl/⌘ + Enter · изменения всегда через review
                  </p>
                  <Button
                    type="submit"
                    disabled={
                      !message.trim() ||
                      isWorking ||
                      Boolean(pending) ||
                      Boolean(voiceDraft) ||
                      sendChat.isPending
                    }
                  >
                    <Send className="size-4" aria-hidden="true" />
                    Отправить
                  </Button>
                </div>
              </form>
            ) : null}
          </div>
        </main>
      </div>
    </RequireSession>
  );
}

function ModeButton({
  active,
  icon: Icon,
  label,
  disabled = false,
  onClick,
}: {
  active: boolean;
  icon: typeof Mic2;
  label: string;
  disabled?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      aria-pressed={active}
      disabled={disabled}
      onClick={onClick}
      className={cn(
        "flex min-h-9 items-center gap-2 rounded-full px-5 text-[12px] disabled:cursor-not-allowed disabled:opacity-45",
        active
          ? "bg-[var(--surface-paper)] text-[var(--accent-ink)] shadow-sm"
          : "text-[var(--ink-secondary)]",
      )}
    >
      <Icon className="size-3.5" aria-hidden="true" />
      {label}
    </button>
  );
}

function assistantLocalhostURL(): string {
  const port = window.location.port ? `:${window.location.port}` : "";
  return `${window.location.protocol}//localhost${port}${window.location.pathname}${window.location.search}`;
}

function VoiceComposer({
  state,
  transcript,
  partial,
  error,
  disabled,
  onTranscript,
  onStart,
  onPause,
  onExecute,
  compact = false,
}: {
  state: VoiceState;
  transcript: string;
  partial: string;
  error?: string;
  disabled: boolean;
  onTranscript: (value: string) => void;
  onStart: () => void;
  onPause: () => void;
  onExecute: () => void;
  compact?: boolean;
}) {
  const connecting = state === "connecting";
  const listening = state === "listening";
  const capturing = listening || connecting;
  return (
    <div className={cn("flex h-full flex-col", compact ? "min-h-[300px]" : "min-h-[360px]")}>
      <div className="flex items-center gap-2 text-[10px] font-semibold tracking-[0.14em] text-[var(--accent)] uppercase">
        <span
          className={cn(
            "size-2 rounded-full",
            capturing ? "animate-pulse bg-[var(--record)]" : "bg-[var(--accent)]",
          )}
        />
        {connecting
          ? "Подключаю голос"
          : listening
            ? "Слушаю поручение"
            : state === "confirming"
              ? "Проверьте стенограмму"
              : "Голосовой live-режим"}
      </div>
      {error ? <Banner variant="warning" title="Голос недоступен" body={error} /> : null}
      <textarea
        aria-label="Стенограмма поручения"
        value={transcript}
        onChange={(event) => onTranscript(event.target.value)}
        placeholder="Нажмите «Начать» и говорите свободно. Здесь появится точная стенограмма поручения…"
        className={cn(
          "font-ui flex-1 resize-none bg-transparent outline-none placeholder:text-[var(--ink-disabled)]",
          compact
            ? "mt-4 min-h-40 text-[14px] leading-[1.55]"
            : "mt-6 min-h-56 text-[clamp(19px,2.2vw,28px)] leading-[1.65]",
        )}
        disabled={capturing || disabled}
      />
      {partial ? (
        <div
          className={cn(
            "mt-3 rounded-[var(--radius-md)] bg-[var(--surface-subtle)] px-4 py-3 text-[var(--ink-secondary)]",
            compact ? "text-[13px] leading-5" : "text-[16px]",
          )}
        >
          <span className="mr-2 text-[9px] tracking-wider uppercase">Распознаётся</span>
          {partial}
        </div>
      ) : null}
      <div
        className={cn(
          "flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border-subtle)]",
          compact ? "mt-4 pt-3" : "mt-6 pt-5",
        )}
      >
        <div
          className={cn(
            "flex items-center gap-2 text-[var(--ink-tertiary)]",
            compact ? "text-[10px]" : "text-[11px]",
          )}
        >
          <ShieldCheck className="size-4 text-[var(--accent)]" aria-hidden="true" />
          Поручение не отправлено, пока вы не нажмёте «Выполнить»
        </div>
        <div className="flex gap-2">
          {capturing ? (
            <Button type="button" variant="secondary" onClick={onPause}>
              {connecting ? (
                <Square className="size-4" aria-hidden="true" />
              ) : (
                <Pause className="size-4" aria-hidden="true" />
              )}
              {connecting ? "Отмена" : "Пауза"}
            </Button>
          ) : (
            <Button type="button" variant="secondary" onClick={onStart} disabled={disabled}>
              <Mic2 className="size-4" aria-hidden="true" />
              {transcript ? "Продолжить" : "Начать"}
            </Button>
          )}
          <Button
            type="button"
            onClick={onExecute}
            disabled={!transcript.trim() || capturing || disabled}
          >
            <Check className="size-4" aria-hidden="true" />
            Выполнить
          </Button>
        </div>
      </div>
    </div>
  );
}

function Conversation({
  turns,
  working,
  events,
  variant = "chat",
  showEmpty = true,
  compact = false,
}: {
  turns: Awaited<ReturnType<typeof listAssistantTurns>>["turns"];
  working: boolean;
  events: AssistantEvent[];
  variant?: "chat" | "voice";
  showEmpty?: boolean;
  compact?: boolean;
}) {
  const activity = buildAssistantActivity(events);
  const nativeActions = buildAssistantNativeActions(events);
  const latestTurnId = turns.at(-1)?.id;
  return (
    <div className="mx-auto max-w-[820px]">
      {turns.length === 0 && showEmpty ? (
        <div className="grid min-h-72 place-items-center text-center">
          <div>
            <MessageCircle className="mx-auto size-8 text-[var(--accent)]" aria-hidden="true" />
            <h2 className="font-display mt-4 text-2xl">Поручите работу, не покидая поток</h2>
            <p className="mt-2 max-w-md text-sm leading-6 text-[var(--ink-secondary)]">
              Ассистент видит структуру этого типа проекта, словарь, guide и память. Любая запись
              сначала станет проверяемым предложением.
            </p>
          </div>
        </div>
      ) : (
        <div className="space-y-4">
          {turns.map((turn) => {
            const focusedVoiceTurn =
              variant === "voice" && turn.id === latestTurnId && turn.inputMode === "voice";
            return (
              <div key={turn.id}>
                <article
                  className={cn(
                    focusedVoiceTurn
                      ? cn(
                          "w-full bg-[var(--surface-subtle)]",
                          compact
                            ? "rounded-[var(--radius-md)] px-4 py-3"
                            : "rounded-[var(--radius-lg)] px-5 py-6 sm:px-7",
                        )
                      : "ml-auto max-w-[80%] rounded-[var(--radius-md)] bg-[var(--accent-soft)] px-5 py-4 text-[14px] leading-6",
                  )}
                >
                  <p className="mb-2 text-[9px] font-semibold tracking-wider text-[var(--accent)] uppercase">
                    {focusedVoiceTurn ? "Стенограмма поручения" : "Вы"} ·{" "}
                    {turn.inputMode === "voice"
                      ? "голосом"
                      : turn.inputMode === "mcp"
                        ? "через MCP"
                        : "в чате"}
                  </p>
                  <p
                    className={cn(
                      "whitespace-pre-wrap",
                      focusedVoiceTurn &&
                        cn(
                          "font-ui",
                          compact
                            ? "text-[14px] leading-[1.55]"
                            : "text-[clamp(18px,2vw,24px)] leading-[1.65]",
                        ),
                    )}
                  >
                    {turn.instruction}
                  </p>
                </article>
                {turn.assistantText ? (
                  <article className="mt-3 max-w-[92%] rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-panel)] px-5 py-4 text-[14px]">
                    <p className="mb-2 text-[9px] font-semibold tracking-wider text-[var(--ink-tertiary)] uppercase">
                      Ассистент
                    </p>
                    <AssistantMarkdown
                      className={
                        compact
                          ? "[&_h1]:text-[15px] [&_h2]:text-[14px] [&_h3]:text-[14px] [&_h1]:my-2 [&_h2]:my-2 [&_h3]:my-2"
                          : undefined
                      }
                    >
                      {turn.assistantText}
                    </AssistantMarkdown>
                  </article>
                ) : null}
                {turn.state === "failed" ? (
                  <div
                    role="alert"
                    className="mt-3 flex max-w-[92%] items-start gap-3 rounded-[var(--radius-md)] border border-[var(--danger)]/30 bg-[var(--danger-soft)] px-4 py-3 text-[12px] leading-5"
                  >
                    <TriangleAlert
                      className="mt-0.5 size-4 shrink-0 text-[var(--danger)]"
                      aria-hidden="true"
                    />
                    <div>
                      <p className="font-semibold">Шаг не завершён</p>
                      <p className="text-[var(--ink-secondary)]">
                        {assistantFailureMessage(turn.errorCode)}
                      </p>
                    </div>
                  </div>
                ) : null}
              </div>
            );
          })}
        </div>
      )}
      {nativeActions.length > 0 ? (
        <section aria-label="Действия в приложении" className="mt-5 space-y-3">
          {nativeActions.map((action) => (
            <AssistantNativeActionCard key={action.id} action={action} />
          ))}
        </section>
      ) : null}
      {working ? <AssistantActivity events={events} compact={compact} /> : null}
      {!working && activity.length > 0 ? (
        <details className="mt-5 border-t border-[var(--border-subtle)] pt-3">
          <summary className="flex cursor-pointer items-center gap-2 text-[11px] text-[var(--ink-tertiary)]">
            <History className="size-3.5" aria-hidden="true" />
            Ход выполнения · {activity.length} шагов
          </summary>
          <ol className="mt-3 space-y-1 text-[12px] text-[var(--ink-secondary)]">
            {activity.map((item) => (
              <li key={item.id} className="flex min-h-7 items-center gap-2">
                {item.status === "done" ? (
                  <Check className="size-3.5 text-[var(--success)]" aria-hidden="true" />
                ) : item.status === "failed" ? (
                  <TriangleAlert className="size-3.5 text-[var(--danger)]" aria-hidden="true" />
                ) : (
                  <LoaderCircle
                    className="size-3.5 animate-spin text-[var(--accent)]"
                    aria-hidden="true"
                  />
                )}
                {item.label}
              </li>
            ))}
          </ol>
        </details>
      ) : null}
    </div>
  );
}

function AssistantNativeActionCard({ action }: { action: AssistantNativeAction }) {
  const copy: Record<
    AssistantNativeAction["kind"],
    { title: string; description: string; action: string }
  > = {
    archive_node: {
      title: "Архивация требует подтверждения",
      description:
        "Ассистент ничего не архивировал. Откройте документ и подтвердите действие в его меню.",
      action: "Открыть документ",
    },
    restore_version: {
      title: "Восстановление версии требует подтверждения",
      description:
        "Текущий текст не изменён. Сравните версию с текущей и подтвердите восстановление в истории.",
      action: "Открыть версии",
    },
    retranscribe: {
      title: "Повторная обработка требует подтверждения",
      description:
        "Исходная стенограмма сохранена. Запустите повторную обработку на штатном экране диктовки.",
      action: "Открыть стенограмму",
    },
    export: {
      title: "Экспорт готов к настройке",
      description:
        "Файл ещё не создан. Проверьте формат и состав проекта на штатном экране экспорта.",
      action: "Настроить экспорт",
    },
  };
  const content = copy[action.kind];
  return (
    <article className="rounded-[var(--radius-md)] border border-[var(--warning)]/35 bg-[var(--warning-soft)] p-4 sm:flex sm:items-center sm:gap-4">
      <span className="grid size-10 shrink-0 place-items-center rounded-full bg-[var(--surface-paper)] text-[var(--warning)] shadow-sm">
        <ShieldCheck className="size-5" aria-hidden="true" />
      </span>
      <div className="mt-3 min-w-0 flex-1 sm:mt-0">
        <p className="text-sm font-semibold">{content.title}</p>
        <p className="mt-1 text-[12px] leading-5 text-[var(--ink-secondary)]">
          {content.description}
        </p>
      </div>
      <Button asChild variant="secondary" className="mt-4 w-full sm:mt-0 sm:w-auto">
        <AssistantNativeActionLink action={action}>
          {content.action}
          <ArrowRight className="size-4" aria-hidden="true" />
        </AssistantNativeActionLink>
      </Button>
    </article>
  );
}

function AssistantNativeActionLink({
  action,
  children,
}: {
  action: AssistantNativeAction;
  children: ReactNode;
}) {
  const destination = action.destination;
  switch (destination.type) {
    case "project_export":
      return (
        <Link
          to="/projects/$projectId/export"
          params={{ projectId: destination.projectId }}
          search={{
            format: destination.format,
            nodeIds: destination.nodeIds,
            chapterTitles: destination.chapterTitles,
            assistantPrepared: true,
          }}
        >
          {children}
        </Link>
      );
    case "project_node":
      return (
        <Link
          to="/projects/$projectId/studio/$nodeId"
          params={{ projectId: destination.projectId, nodeId: destination.nodeId }}
        >
          {children}
        </Link>
      );
    case "node_versions":
      return (
        <Link
          to="/nodes/$nodeId/versions"
          params={{ nodeId: destination.nodeId }}
          search={{
            versionId: destination.versionId,
            expectedCurrentRevision: destination.expectedCurrentRevision,
          }}
        >
          {children}
        </Link>
      );
    case "dictation_review":
      return (
        <Link to="/dictations/$sessionId/review" params={{ sessionId: destination.sessionId }}>
          {children}
        </Link>
      );
  }
}

function AssistantActivity({
  events,
  compact = false,
}: {
  events: AssistantEvent[];
  compact?: boolean;
}) {
  const activity = buildAssistantActivity(events);
  const current =
    [...activity].reverse().find((item) => item.status === "working") ?? activity.at(-1);
  return (
    <section
      role="status"
      aria-live="polite"
      aria-label="Ход работы ассистента"
      className="mt-4 overflow-hidden rounded-[var(--radius-md)] border border-[var(--accent)]/25 bg-[var(--accent-soft)]/45"
    >
      <div className={cn("flex items-start gap-3", compact ? "px-4 py-3" : "px-5 py-4")}>
        <span className="mt-0.5 grid size-8 shrink-0 place-items-center rounded-full bg-[var(--surface-paper)] text-[var(--accent)] shadow-sm">
          <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
        </span>
        <div className="min-w-0">
          <p className="text-[10px] font-semibold tracking-[0.12em] text-[var(--accent)] uppercase">
            Ассистент работает
          </p>
          <p className={cn("mt-1 font-medium", compact ? "text-[13px]" : "text-sm")}>
            {current?.label ?? "Разбирает поручение и планирует безопасные шаги"}
          </p>
          <p className="mt-1 text-[11px] text-[var(--ink-secondary)]">
            Все изменения появятся в отдельном просмотре до применения.
          </p>
        </div>
      </div>
      {activity.length > 0 ? (
        <ol
          className={cn(
            "border-t border-[var(--accent)]/15 bg-[var(--surface-paper)]/55 py-3",
            compact ? "px-4" : "px-5",
          )}
        >
          {activity.map((item) => (
            <li key={item.id} className="flex min-h-7 items-center gap-3 text-[12px]">
              {item.status === "done" ? (
                <Check className="size-3.5 shrink-0 text-[var(--success)]" aria-hidden="true" />
              ) : item.status === "failed" ? (
                <TriangleAlert
                  className="size-3.5 shrink-0 text-[var(--danger)]"
                  aria-hidden="true"
                />
              ) : (
                <LoaderCircle
                  className="size-3.5 shrink-0 animate-spin text-[var(--accent)]"
                  aria-hidden="true"
                />
              )}
              <span
                className={cn(
                  item.status === "done"
                    ? "text-[var(--ink-secondary)]"
                    : "text-[var(--ink-primary)]",
                )}
              >
                {item.label}
              </span>
            </li>
          ))}
        </ol>
      ) : null}
    </section>
  );
}
