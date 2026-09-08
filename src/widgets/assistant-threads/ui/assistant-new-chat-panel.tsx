import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  ArrowLeft,
  Bot,
  FileText,
  Globe2,
  MessageCircle,
  Mic2,
  PanelsTopLeft,
  Paperclip,
  Send,
  Settings2,
  X,
} from "lucide-react";
import { type FormEvent, useEffect, useState } from "react";

import {
  type AssistantInputMode,
  type AssistantSelectionAttachment,
  type AssistantTargetKind,
  type AssistantWriterPreferences,
  createAssistantRun,
  getAssistantCapabilities,
  listAssistantProviders,
} from "~entities/assistant";
import type { DocumentNode, Project } from "~entities/project";
import { cn, loadWriterPreferences } from "~shared/lib";
import { Banner, Button, Select } from "~shared/ui";

interface AssistantNewChatPanelProps {
  project: Project;
  node?: DocumentNode;
  canvas?: { id: string; title: string };
  canvasContext?: {
    selectionIds: string[];
    viewport: { x: number; y: number; zoom: number };
  };
  selectionAttachment?: AssistantSelectionAttachment;
  hasThreads: boolean;
  backgroundWorkingCount: number;
  onStarted: (runId: string) => void;
  onSelectionConsumed?: () => void;
  onBack: () => void;
  onClose: () => void;
}

export function AssistantNewChatPanel({
  project,
  node,
  canvas,
  canvasContext,
  selectionAttachment,
  hasThreads,
  backgroundWorkingCount,
  onStarted,
  onSelectionConsumed,
  onBack,
  onClose,
}: AssistantNewChatPanelProps) {
  const queryClient = useQueryClient();
  const [targetKind, setTargetKind] = useState<AssistantTargetKind>(() =>
    canvas ? "canvas" : node ? "node" : "project",
  );
  const [inputMode, setInputMode] = useState<AssistantInputMode>("chat");
  const [message, setMessage] = useState("");
  const [providerChoice, setProviderChoice] = useState("");
  const preferences = loadWriterPreferences();
  const writerPreferences: AssistantWriterPreferences = {
    language: preferences.language,
    commandPrefix: preferences.commandPrefix,
    commandsEnabled: preferences.commandsEnabled,
    liveTranscript: preferences.liveTranscript,
    autoPlaySource: preferences.autoPlaySource,
    assistantSilenceCheckpointMs: preferences.assistantSilenceCheckpointMs,
    fontFamily: preferences.fontFamily,
    fontSize: preferences.fontSize,
    lineHeight: preferences.lineHeight,
    manuscriptWidth: preferences.manuscriptWidth,
    theme: preferences.theme,
    spellcheck: preferences.spellcheck,
    wordGoal: preferences.wordGoal,
  };
  const capabilities = useQuery({
    queryKey: ["assistant-capabilities"],
    queryFn: getAssistantCapabilities,
  });
  const providers = useQuery({
    queryKey: ["assistant-providers"],
    queryFn: listAssistantProviders,
  });
  const compatibleConnections =
    providers.data?.connections.filter(
      (connection) => connection.status === "compatible" || connection.status === "limited",
    ) ?? [];

  useEffect(() => {
    if (providerChoice || !providers.data || !capabilities.data) return;
    const preferred = compatibleConnections.find((connection) => connection.isDefault);
    if (preferred) setProviderChoice(preferred.id);
    else if (capabilities.data.systemProviderAvailable) setProviderChoice("system");
    else if (compatibleConnections[0]) setProviderChoice(compatibleConnections[0].id);
  }, [capabilities.data, compatibleConnections, providerChoice, providers.data]);

  const selectedConnection = compatibleConnections.find(
    (connection) => connection.id === providerChoice,
  );
  const providerAvailable =
    providerChoice === "system"
      ? capabilities.data?.systemProviderAvailable === true
      : Boolean(selectedConnection);
  const createThread = useMutation({
    mutationFn: () =>
      createAssistantRun(project.id, {
        title: inputMode === "chat" ? message.trim() : "Голосовое поручение",
        targetKind,
        targetNodeId: targetKind === "node" ? node?.id : undefined,
        targetCanvasId: targetKind === "canvas" ? canvas?.id : undefined,
        canvasSelectionIds: targetKind === "canvas" ? canvasContext?.selectionIds : undefined,
        canvasViewport: targetKind === "canvas" ? canvasContext?.viewport : undefined,
        inputMode,
        silenceCheckpointMs: preferences.assistantSilenceCheckpointMs,
        providerConnectionId: selectedConnection?.id,
        writerPreferences,
        initialInstruction: inputMode === "chat" ? message.trim() : undefined,
        execute: inputMode === "chat",
        selectionAttachment,
      }),
    onSuccess: async ({ run }) => {
      await queryClient.invalidateQueries({ queryKey: ["assistant-runs", project.id] });
      if (selectionAttachment) onSelectionConsumed?.();
      onStarted(run.id);
    },
  });

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!providerAvailable || (inputMode === "chat" && !message.trim())) return;
    createThread.mutate();
  };

  return (
    <aside
      aria-label="Новый чат ассистента"
      data-testid="assistant-new-chat-panel"
      className="flex h-full min-h-0 w-full flex-col border-l border-[var(--border-subtle)] bg-[var(--surface-panel)]"
    >
      <header className="flex h-14 shrink-0 items-center gap-2 border-b border-[var(--border-subtle)] px-3">
        {hasThreads ? (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            onClick={onBack}
            aria-label="Вернуться к чатам"
          >
            <ArrowLeft className="size-4" aria-hidden="true" />
          </Button>
        ) : (
          <span className="grid size-9 shrink-0 place-items-center text-[var(--accent)]">
            <Bot className="size-4" aria-hidden="true" />
          </span>
        )}
        <div className="min-w-0 flex-1">
          <p className="text-[9px] font-semibold tracking-[0.12em] text-[var(--ink-tertiary)] uppercase">
            Ассистент · новый чат
          </p>
          <p className="truncate text-sm font-semibold">Новое поручение</p>
        </div>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          onClick={onClose}
          aria-label="Скрыть панель"
        >
          <X className="size-4" aria-hidden="true" />
        </Button>
      </header>

      <form onSubmit={submit} className="flex min-h-0 flex-1 flex-col">
        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
          <h2 className="font-[var(--font-display)] text-xl font-semibold">Что поручить?</h2>
          <p className="mt-1 text-[12px] leading-5 text-[var(--ink-secondary)]">
            Новый чат работает независимо. {canvas ? "Доска" : "Рукопись"}, проект и руководство
            остаются перед глазами.
          </p>

          {selectionAttachment ? (
            <div className="mt-4 flex items-start gap-2 rounded-[var(--radius-md)] bg-[var(--accent-soft)] px-3 py-2 text-[11px]">
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

          <fieldset className="mt-5">
            <legend className="text-[10px] font-semibold tracking-[0.1em] text-[var(--ink-tertiary)] uppercase">
              Область работы
            </legend>
            <div className="mt-2 grid grid-cols-2 gap-2">
              {canvas ? (
                <ScopeButton
                  active={targetKind === "canvas"}
                  icon={PanelsTopLeft}
                  label={canvas.title}
                  onClick={() => setTargetKind("canvas")}
                />
              ) : node ? (
                <ScopeButton
                  active={targetKind === "node"}
                  icon={FileText}
                  label={node.title}
                  onClick={() => setTargetKind("node")}
                />
              ) : null}
              <ScopeButton
                active={targetKind === "project"}
                icon={Globe2}
                label="Весь проект"
                onClick={() => setTargetKind("project")}
              />
            </div>
          </fieldset>

          <fieldset className="mt-5">
            <legend className="text-[10px] font-semibold tracking-[0.1em] text-[var(--ink-tertiary)] uppercase">
              Способ общения
            </legend>
            <div className="mt-2 grid grid-cols-2 rounded-full border border-[var(--border-subtle)] bg-[var(--surface-subtle)] p-1">
              <ModeButton
                active={inputMode === "chat"}
                icon={MessageCircle}
                label="Чат"
                onClick={() => setInputMode("chat")}
              />
              <ModeButton
                active={inputMode === "voice"}
                icon={Mic2}
                label="Голосом"
                disabled={!capabilities.data?.voiceAvailable}
                onClick={() => setInputMode("voice")}
              />
            </div>
          </fieldset>

          {inputMode === "chat" ? (
            <div className="mt-4 rounded-[var(--radius-md)] border border-[var(--border-strong)] bg-[var(--surface-paper)] p-3 focus-within:border-[var(--focus)]">
              <label htmlFor="assistant-new-thread-message" className="sr-only">
                Первое поручение
              </label>
              <textarea
                id="assistant-new-thread-message"
                value={message}
                onChange={(event) => setMessage(event.target.value)}
                rows={6}
                maxLength={32_000}
                placeholder={
                  canvas
                    ? "Например: сгруппируй выбранные идеи и предложи недостающие связи…"
                    : "Например: сделай первый абзац загадочнее, но не меняй последнюю реплику…"
                }
                className="w-full resize-none bg-transparent text-sm leading-6 outline-none placeholder:text-[var(--ink-tertiary)]"
                onKeyDown={(event) => {
                  if ((event.ctrlKey || event.metaKey) && event.key === "Enter") {
                    event.preventDefault();
                    event.currentTarget.form?.requestSubmit();
                  }
                }}
              />
              <p className="mt-2 text-[10px] text-[var(--ink-tertiary)]">Ctrl/⌘ + Enter</p>
            </div>
          ) : (
            <div className="mt-4 rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-4 text-[12px] leading-5 text-[var(--ink-secondary)]">
              Чат останется рядом с {canvas ? "доской" : "рукописью"}. Запись начнётся только после
              явного нажатия кнопки микрофона.
            </div>
          )}

          <details className="mt-4 rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] px-3 py-2">
            <summary className="cursor-pointer text-[11px] font-semibold text-[var(--ink-secondary)]">
              Модель и контекст
            </summary>
            <label
              htmlFor="assistant-new-thread-provider"
              className="mt-3 block text-[10px] font-semibold tracking-[0.1em] text-[var(--ink-tertiary)] uppercase"
            >
              Модель
            </label>
            <Select
              id="assistant-new-thread-provider"
              className="mt-2"
              value={providerChoice}
              onChange={(event) => setProviderChoice(event.target.value)}
              disabled={providers.isPending || capabilities.isPending}
            >
              <option value="" disabled>
                Нет совместимой модели
              </option>
              {capabilities.data?.systemProviderConfigured ? (
                <option value="system" disabled={!capabilities.data.systemProviderAvailable}>
                  Системная · {capabilities.data.systemProvider?.model ?? "модель"}
                </option>
              ) : null}
              {compatibleConnections.map((connection) => (
                <option key={connection.id} value={connection.id}>
                  {connection.name} · {connection.model}
                </option>
              ))}
            </Select>
            <Button asChild variant="ghost" size="compact" className="mt-2 w-full">
              <Link to="/settings/assistant-providers">
                <Settings2 className="size-3.5" aria-hidden="true" />
                Настроить модели
              </Link>
            </Button>
          </details>

          {createThread.isError ? (
            <div className="mt-4">
              <Banner variant="danger" title="Чат не создан" body={createThread.error.message} />
            </div>
          ) : null}
          {backgroundWorkingCount > 0 ? (
            <p className="mt-4 rounded-[var(--radius-md)] bg-[var(--surface-subtle)] px-3 py-2 text-[10px] leading-4 text-[var(--ink-secondary)]">
              {backgroundWorkingCount}{" "}
              {backgroundWorkingCount === 1 ? "другой чат продолжает" : "других чата продолжают"}{" "}
              работу в фоне.
            </p>
          ) : null}
        </div>

        <footer className="shrink-0 border-t border-[var(--border-subtle)] bg-[var(--surface-panel)] p-3">
          {!providerAvailable && !capabilities.isPending && !providers.isPending ? (
            <p className="mb-2 text-[11px] text-[var(--warning)]">Выберите совместимую модель.</p>
          ) : null}
          <Button
            type="submit"
            className="w-full"
            disabled={
              createThread.isPending ||
              capabilities.isPending ||
              providers.isPending ||
              !providerAvailable ||
              (inputMode === "chat" && !message.trim())
            }
          >
            {inputMode === "chat" ? (
              <Send className="size-4" aria-hidden="true" />
            ) : (
              <Mic2 className="size-4" aria-hidden="true" />
            )}
            {createThread.isPending
              ? "Запускаем…"
              : inputMode === "chat"
                ? "Начать чат"
                : "Открыть голосовой чат"}
          </Button>
        </footer>
      </form>
    </aside>
  );
}

function ScopeButton({
  active,
  icon: Icon,
  label,
  onClick,
}: {
  active: boolean;
  icon: typeof FileText;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={cn(
        "flex min-h-11 items-center gap-2 rounded-[var(--radius-md)] border px-3 text-left text-[11px] font-medium",
        active
          ? "border-[var(--accent)] bg-[var(--accent-soft)] text-[var(--accent-ink)]"
          : "border-[var(--border-subtle)] bg-[var(--surface-paper)] text-[var(--ink-secondary)]",
      )}
    >
      <Icon className="size-3.5 shrink-0" aria-hidden="true" />
      <span className="truncate">{label}</span>
    </button>
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
  icon: typeof MessageCircle;
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
        "flex min-h-9 items-center justify-center gap-2 rounded-full px-3 text-[11px] disabled:opacity-45",
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
