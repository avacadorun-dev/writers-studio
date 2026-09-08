import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import {
  ArrowRight,
  Bot,
  FileText,
  Globe2,
  MessageCircle,
  Mic2,
  Settings2,
  ShieldCheck,
} from "lucide-react";
import { useEffect, useState } from "react";

import {
  type AssistantInputMode,
  type AssistantTargetKind,
  type AssistantWriterPreferences,
  createAssistantRun,
  getAssistantCapabilities,
  listAssistantProviders,
} from "~entities/assistant";
import type { DocumentNode, Project } from "~entities/project";
import { cn, loadWriterPreferences } from "~shared/lib";
import { Banner, Button, ModalDialog, Select } from "~shared/ui";

interface AssistantLauncherProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  project: Project;
  node?: DocumentNode;
  canvas?: { id: string; title: string };
  canvasContext?: {
    selectionIds: string[];
    viewport: { x: number; y: number; zoom: number };
  };
  onStarted?: (runId: string) => void;
}

export function AssistantLauncher({
  open,
  onOpenChange,
  project,
  node,
  canvas,
  canvasContext,
  onStarted,
}: AssistantLauncherProps) {
  const navigate = useNavigate();
  const [targetKind, setTargetKind] = useState<AssistantTargetKind>(canvas ? "canvas" : "node");
  const [inputMode, setInputMode] = useState<AssistantInputMode>("voice");
  const [providerChoice, setProviderChoice] = useState("");
  const preferences = loadWriterPreferences();
  const silenceCheckpointMs = preferences.assistantSilenceCheckpointMs;
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
    enabled: open,
  });
  const providers = useQuery({
    queryKey: ["assistant-providers"],
    queryFn: listAssistantProviders,
    enabled: open,
  });
  const compatibleConnections =
    providers.data?.connections.filter(
      (connection) => connection.status === "compatible" || connection.status === "limited",
    ) ?? [];
  useEffect(() => {
    if (!open || providerChoice || !providers.data || !capabilities.data) return;
    const preferred = compatibleConnections.find((connection) => connection.isDefault);
    if (preferred) {
      setProviderChoice(preferred.id);
    } else if (capabilities.data.systemProviderAvailable) {
      setProviderChoice("system");
    } else if (compatibleConnections[0]) {
      setProviderChoice(compatibleConnections[0].id);
    }
  }, [capabilities.data, compatibleConnections, open, providerChoice, providers.data]);
  useEffect(() => {
    if (!open) {
      setProviderChoice("");
      return;
    }
    setTargetKind(canvas ? "canvas" : node ? "node" : "project");
  }, [canvas, node, open]);
  const selectedConnection = compatibleConnections.find(
    (connection) => connection.id === providerChoice,
  );
  const providerAvailable =
    providerChoice === "system"
      ? capabilities.data?.systemProviderAvailable === true
      : Boolean(selectedConnection);
  const createRun = useMutation({
    mutationFn: () =>
      createAssistantRun(project.id, {
        targetKind,
        targetNodeId: targetKind === "node" ? node?.id : undefined,
        targetCanvasId: targetKind === "canvas" ? canvas?.id : undefined,
        canvasSelectionIds: targetKind === "canvas" ? canvasContext?.selectionIds : undefined,
        canvasViewport: targetKind === "canvas" ? canvasContext?.viewport : undefined,
        inputMode,
        silenceCheckpointMs,
        providerConnectionId: selectedConnection?.id,
        writerPreferences,
      }),
    onSuccess: ({ run }) => {
      onOpenChange(false);
      if (onStarted) {
        onStarted(run.id);
        return;
      }
      void navigate({
        to: "/projects/$projectId/assistant/$runId",
        params: { projectId: project.id, runId: run.id },
      });
    },
  });

  return (
    <ModalDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Как ассистенту работать?"
      description="Поручение можно надиктовать или написать. Изменения появятся только после вашей проверки."
      icon={Bot}
      className="sm:w-[min(760px,calc(100vw-40px))]"
      footer={
        <>
          <Button variant="ghost" type="button" onClick={() => onOpenChange(false)}>
            Отмена
          </Button>
          <Button
            type="button"
            disabled={
              createRun.isPending ||
              capabilities.isPending ||
              providers.isPending ||
              !capabilities.data?.chatAvailable ||
              !providerAvailable
            }
            onClick={() => createRun.mutate()}
          >
            {createRun.isPending ? "Открываем…" : "Начать сессию"}
            <ArrowRight className="size-4" aria-hidden="true" />
          </Button>
        </>
      }
    >
      {capabilities.data &&
      !capabilities.data.systemProviderAvailable &&
      compatibleConnections.length === 0 ? (
        <Banner
          variant="warning"
          title="Нет модели с рабочими инструментами"
          body={
            capabilities.data.systemProviderStatusDetail ||
            "Добавьте совместимый провайдер или повторите проверку системной модели."
          }
          action={
            <Button asChild variant="secondary" size="compact">
              <Link to="/settings/assistant-providers">
                <Settings2 className="size-4" aria-hidden="true" />
                Открыть AI-настройки
              </Link>
            </Button>
          }
        />
      ) : null}
      {createRun.isError ? (
        <Banner
          variant="danger"
          title="Не удалось открыть ассистента"
          body={createRun.error.message}
        />
      ) : null}
      <div className="mt-4 rounded-[var(--radius-md)] bg-[var(--surface-subtle)] px-4 py-3 text-sm">
        <p className="font-medium">{project.title}</p>
        <p className="mt-1 text-[12px] text-[var(--ink-secondary)]">
          {targetKind === "canvas"
            ? canvas?.title
            : targetKind === "node"
              ? node?.title
              : project.structure.rootLabel}
        </p>
      </div>
      <div className="mt-5 rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-4">
        <div className="flex items-center justify-between gap-3">
          <label
            htmlFor="assistant-provider"
            className="text-[11px] font-semibold tracking-[0.12em] text-[var(--ink-tertiary)] uppercase"
          >
            Модель этой сессии
          </label>
          <Link
            to="/settings/assistant-providers"
            className="text-[11px] font-medium text-[var(--accent)] underline-offset-2 hover:underline"
          >
            Настроить
          </Link>
        </div>
        <Select
          id="assistant-provider"
          className="mt-2"
          value={providerChoice}
          onChange={(event) => setProviderChoice(event.target.value)}
          disabled={providers.isPending || capabilities.isPending}
        >
          <option value="" disabled>
            {providers.isPending || capabilities.isPending
              ? "Проверяем модели…"
              : "Нет совместимой модели"}
          </option>
          {capabilities.data?.systemProviderConfigured ? (
            <option value="system" disabled={!capabilities.data.systemProviderAvailable}>
              Системная · {capabilities.data.systemProvider?.model ?? "модель"}
              {capabilities.data.systemProviderAvailable ? "" : " — инструменты недоступны"}
            </option>
          ) : null}
          {compatibleConnections.map((connection) => (
            <option key={connection.id} value={connection.id}>
              {connection.name} · {connection.model}
              {connection.isDefault ? " · по умолчанию" : ""}
            </option>
          ))}
        </Select>
        <p className="mt-2 text-[11px] leading-5 text-[var(--ink-tertiary)]">
          Совместимость проверяется реальной цепочкой из двух tool-calls и возврата результата.
        </p>
      </div>
      <fieldset className="mt-5">
        <legend className="text-[11px] font-semibold tracking-[0.12em] text-[var(--ink-tertiary)] uppercase">
          Область работы
        </legend>
        <div className="mt-2 grid gap-2 sm:grid-cols-2">
          {canvas ? (
            <ChoiceCard
              selected={targetKind === "canvas"}
              icon={FileText}
              title={`Доска «${canvas.title}»`}
              body={
                canvasContext?.selectionIds.length
                  ? `Выбрано элементов: ${canvasContext.selectionIds.length}. Ассистент увидит selection и текущую область.`
                  : "Ассистент увидит доску и текущую область viewport; изменения сначала появятся как preview."
              }
              onClick={() => setTargetKind("canvas")}
            />
          ) : node ? (
            <ChoiceCard
              selected={targetKind === "node"}
              icon={FileText}
              title={`Только «${node.title}»`}
              body="Точная работа в открытом документе; остальной проект доступен как контекст."
              onClick={() => setTargetKind("node")}
            />
          ) : null}
          <ChoiceCard
            selected={targetKind === "project"}
            icon={Globe2}
            title="Весь проект"
            body={`Структура «${project.structure.rootLabel}», словарь, заметки, память и документы.`}
            onClick={() => setTargetKind("project")}
          />
        </div>
      </fieldset>
      <fieldset className="mt-5">
        <legend className="text-[11px] font-semibold tracking-[0.12em] text-[var(--ink-tertiary)] uppercase">
          Начать общение
        </legend>
        <div className="mt-2 grid gap-2 sm:grid-cols-2">
          <ChoiceCard
            selected={inputMode === "voice"}
            icon={Mic2}
            title="Голосом · live"
            body={
              silenceCheckpointMs === 0
                ? "Видите стенограмму; поручение выполняется только по явной команде."
                : `Видите стенограмму; после ${silenceCheckpointMs / 1000} секунд тишины ассистент спросит, выполнять ли поручение.`
            }
            onClick={() => setInputMode("voice")}
          />
          <ChoiceCard
            selected={inputMode === "chat"}
            icon={MessageCircle}
            title="Чат"
            body="Тот же контекст, инструменты и review, когда говорить неудобно."
            onClick={() => setInputMode("chat")}
          />
        </div>
      </fieldset>
      <div className="mt-5 flex items-start gap-3 rounded-[var(--radius-md)] bg-[var(--info-soft)] px-4 py-3 text-[12px] leading-5 text-[var(--ink-secondary)]">
        <ShieldCheck className="mt-0.5 size-4 shrink-0 text-[var(--info)]" aria-hidden="true" />
        Ассистент покажет поручение → точную цель → вызванные инструменты → diff. Можно принять
        отдельные группы и мгновенно отменить применённый набор.
      </div>
    </ModalDialog>
  );
}

function ChoiceCard({
  selected,
  icon: Icon,
  title,
  body,
  onClick,
}: {
  selected: boolean;
  icon: typeof Bot;
  title: string;
  body: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      aria-pressed={selected}
      onClick={onClick}
      className={cn(
        "min-h-28 rounded-[var(--radius-md)] border bg-[var(--surface-paper)] p-4 text-left transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus)]",
        selected
          ? "border-[var(--accent)] shadow-[inset_0_0_0_1px_var(--accent)]"
          : "border-[var(--border-subtle)] hover:border-[var(--border-strong)]",
      )}
    >
      <Icon
        className={cn("size-5", selected ? "text-[var(--accent)]" : "text-[var(--ink-tertiary)]")}
        aria-hidden="true"
      />
      <span className="mt-3 block text-[15px] font-semibold">{title}</span>
      <span className="mt-1 block text-[12px] leading-5 text-[var(--ink-secondary)]">{body}</span>
    </button>
  );
}
