import { Mic, ShieldCheck } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { getSTTCapabilities, type STTCapabilities } from "~entities/dictation";
import type { DocumentNode, Project } from "~entities/project";
import {
  calculateMicrophoneLevel,
  recorderController,
  smoothMicrophoneLevel,
} from "~features/dictation";
import { loadWriterPreferences, saveWriterPreferences } from "~shared/lib";
import { Banner, Button, Field, ModalDialog, Select, Switch } from "~shared/ui";

interface DictationPreflightProps {
  project: Project;
  nodes: DocumentNode[];
  destinationNodeId?: string;
  onClose: () => void;
  onStarted: (localId: string) => void;
}

type PermissionState = "checking" | "prompt" | "granted" | "denied" | "insecure" | "unsupported";
type STTCheckState =
  | { status: "checking" }
  | { status: "ready"; capabilities: STTCapabilities }
  | { status: "unavailable"; capabilities: STTCapabilities }
  | { status: "error" };

export function DictationPreflight({
  project,
  nodes,
  destinationNodeId,
  onClose,
  onStarted,
}: DictationPreflightProps) {
  const [destination, setDestination] = useState(destinationNodeId ?? "");
  const [preferences] = useState(loadWriterPreferences);
  const [language, setLanguage] = useState<"ru" | "en">(project.language || preferences.language);
  const [commandsEnabled, setCommandsEnabled] = useState(preferences.commandsEnabled);
  const [microphoneId, setMicrophoneId] = useState(preferences.microphoneId);
  const [microphones, setMicrophones] = useState<MediaDeviceInfo[]>([]);
  const [permission, setPermission] = useState<PermissionState>("checking");
  const [online, setOnline] = useState(navigator.onLine);
  const [sttCheck, setSTTCheck] = useState<STTCheckState>({ status: "checking" });
  const [availableBytes, setAvailableBytes] = useState<number>();
  const [level, setLevel] = useState(0);
  const [signalDetected, setSignalDetected] = useState(false);
  const [trackMuted, setTrackMuted] = useState(false);
  const [stream, setStream] = useState<MediaStream>();
  const [requestingMicrophone, setRequestingMicrophone] = useState(false);
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState<string>();
  const animation = useRef<number | undefined>(undefined);
  const streamRef = useRef<MediaStream | undefined>(undefined);
  const audioContextRef = useRef<AudioContext | undefined>(undefined);
  const audioSourceRef = useRef<MediaStreamAudioSourceNode | undefined>(undefined);
  const analyserRef = useRef<AnalyserNode | undefined>(undefined);
  const meterSinkRef = useRef<MediaStreamAudioDestinationNode | undefined>(undefined);
  const smoothedLevelRef = useRef(0);
  const signalDetectedRef = useRef(false);
  const streamHandedOff = useRef(false);

  const secureOrigin = window.isSecureContext;
  const mediaSupported =
    typeof MediaRecorder !== "undefined" && Boolean(navigator.mediaDevices?.getUserMedia);
  const supported = secureOrigin && mediaSupported;
  const lowStorage = availableBytes !== undefined && availableBytes < 100 * 1024 * 1024;
  const sttCapabilities = "capabilities" in sttCheck ? sttCheck.capabilities : undefined;
  const languageSupported =
    sttCheck.status !== "ready" || Boolean(sttCapabilities?.languages.includes(language));
  const liveTranscriptionAvailable = Boolean(
    sttCapabilities?.modes.some((mode) => mode === "realtime" || mode === "chunked_live"),
  );
  const sttBlocked = online && (sttCheck.status !== "ready" || !languageSupported);

  useEffect(() => {
    if (!online) {
      return;
    }
    let active = true;
    setSTTCheck({ status: "checking" });
    void getSTTCapabilities()
      .then((capabilities) => {
        if (!active) return;
        const ready = capabilities.available && capabilities.modes.length > 0;
        setSTTCheck({ status: ready ? "ready" : "unavailable", capabilities });
      })
      .catch(() => {
        if (active) setSTTCheck({ status: "error" });
      });
    return () => {
      active = false;
    };
  }, [online]);

  useEffect(() => {
    const handleOnline = () => setOnline(navigator.onLine);
    window.addEventListener("online", handleOnline);
    window.addEventListener("offline", handleOnline);
    if (navigator.storage?.estimate) {
      void navigator.storage.estimate().then((estimate) => {
        if (estimate.quota !== undefined && estimate.usage !== undefined) {
          setAvailableBytes(estimate.quota - estimate.usage);
        }
      });
    }
    if (!secureOrigin) {
      setPermission("insecure");
    } else if (!mediaSupported) {
      setPermission("unsupported");
    } else if (navigator.permissions?.query) {
      void navigator.permissions
        .query({ name: "microphone" as PermissionName })
        .then((status) => {
          setPermission(status.state as Exclude<PermissionState, "checking" | "unsupported">);
          status.addEventListener("change", () => {
            setPermission(status.state as Exclude<PermissionState, "checking" | "unsupported">);
          });
        })
        .catch(() => setPermission("prompt"));
    } else {
      setPermission("prompt");
    }
    return () => {
      window.removeEventListener("online", handleOnline);
      window.removeEventListener("offline", handleOnline);
      window.cancelAnimationFrame(animation.current ?? 0);
      audioSourceRef.current?.disconnect();
      analyserRef.current?.disconnect();
      meterSinkRef.current?.disconnect();
      if (!streamHandedOff.current) {
        for (const track of streamRef.current?.getTracks() ?? []) {
          track.stop();
        }
      }
      void audioContextRef.current?.close();
    };
  }, [mediaSupported]);

  const requestMicrophone = async () => {
    await openMicrophone(microphoneId);
  };

  const openMicrophone = async (requestedDeviceId: string) => {
    setError(undefined);
    setRequestingMicrophone(true);
    try {
      window.cancelAnimationFrame(animation.current ?? 0);
      audioSourceRef.current?.disconnect();
      analyserRef.current?.disconnect();
      meterSinkRef.current?.disconnect();
      audioSourceRef.current = undefined;
      analyserRef.current = undefined;
      meterSinkRef.current = undefined;
      smoothedLevelRef.current = 0;
      signalDetectedRef.current = false;
      setLevel(0);
      setSignalDetected(false);
      setTrackMuted(false);
      for (const track of streamRef.current?.getTracks() ?? []) {
        track.stop();
      }
      streamRef.current = undefined;
      setStream(undefined);
      await audioContextRef.current?.close();
      const media = await navigator.mediaDevices.getUserMedia({
        audio: {
          ...(requestedDeviceId ? { deviceId: { exact: requestedDeviceId } } : {}),
          echoCancellation: true,
          noiseSuppression: true,
          autoGainControl: true,
        },
      });
      streamRef.current = media;
      setStream(media);
      setPermission("granted");
      const track = media.getAudioTracks()[0];
      if (!track) {
        throw new DOMException("Audio track is missing", "NotFoundError");
      }
      setTrackMuted(track.muted || !track.enabled || track.readyState !== "live");
      track.addEventListener("mute", () => setTrackMuted(true));
      track.addEventListener("unmute", () => setTrackMuted(false));
      track.addEventListener("ended", () => setTrackMuted(true));
      try {
        const devices = await navigator.mediaDevices.enumerateDevices();
        setMicrophones(devices.filter((device) => device.kind === "audioinput"));
      } catch {
        setMicrophones([]);
      }
      const context = new AudioContext({ latencyHint: "interactive" });
      audioContextRef.current = context;
      const analyser = context.createAnalyser();
      analyser.fftSize = 1024;
      analyser.smoothingTimeConstant = 0;
      const source = context.createMediaStreamSource(media);
      const meterSink = context.createMediaStreamDestination();
      source.connect(analyser);
      analyser.connect(meterSink);
      audioSourceRef.current = source;
      analyserRef.current = analyser;
      meterSinkRef.current = meterSink;
      if (context.state === "suspended") {
        await context.resume();
      }

      const samples = new Float32Array(analyser.fftSize);
      let lastPaint = 0;
      const update = (timestamp: number) => {
        analyser.getFloatTimeDomainData(samples);
        const measuredLevel = calculateMicrophoneLevel(samples);
        smoothedLevelRef.current = smoothMicrophoneLevel(smoothedLevelRef.current, measuredLevel);
        if (measuredLevel > 0.01 && !signalDetectedRef.current) {
          signalDetectedRef.current = true;
          setSignalDetected(true);
          setTrackMuted(false);
        }
        if (timestamp - lastPaint >= 50) {
          setLevel(smoothedLevelRef.current);
          lastPaint = timestamp;
        }
        animation.current = requestAnimationFrame(update);
      };
      animation.current = requestAnimationFrame(update);
    } catch (cause) {
      const name = cause instanceof DOMException ? cause.name : "";
      if (requestedDeviceId && (name === "NotFoundError" || name === "OverconstrainedError")) {
        setMicrophoneId("");
        saveWriterPreferences({ ...preferences, microphoneId: "" });
        await openMicrophone("");
        return;
      }
      setPermission(name === "NotAllowedError" ? "denied" : "unsupported");
      setError(
        name === "NotAllowedError"
          ? "Разрешите доступ к микрофону в настройках браузера и попробуйте снова."
          : "Микрофон не найден. Подключите устройство и повторите проверку.",
      );
    } finally {
      setRequestingMicrophone(false);
    }
  };

  const start = async () => {
    setStarting(true);
    setError(undefined);
    try {
      if (sttBlocked) {
        throw new Error(
          languageSupported
            ? "Распознавание речи сейчас недоступно. Проверьте настройки провайдера."
            : "Выбранный язык не поддерживается настроенным распознаванием речи.",
        );
      }
      let activeStream = stream;
      if (!activeStream) {
        activeStream = await navigator.mediaDevices.getUserMedia({
          audio: {
            ...(microphoneId ? { deviceId: { exact: microphoneId } } : {}),
            echoCancellation: true,
            noiseSuppression: true,
            autoGainControl: true,
          },
        });
        streamRef.current = activeStream;
        setStream(activeStream);
        setPermission("granted");
      }
      const node = nodes.find((item) => item.id === destination);
      const localId = await recorderController.start({
        projectId: project.id,
        projectTitle: project.title,
        destinationNodeId: node?.id,
        destinationTitle: node?.title,
        language,
        commandsEnabled,
        commandPrefix: preferences.commandPrefix,
        liveTranscript: preferences.liveTranscript,
        stream: activeStream,
      });
      streamHandedOff.current = true;
      onStarted(localId);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Не удалось начать запись");
      setStarting(false);
    }
  };

  return (
    <ModalDialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title="Подготовка диктовки"
      description="Проверьте место, язык, микрофон и распознавание речи."
      className="sm:h-[650px] sm:w-[min(520px,calc(100vw-40px))] sm:p-7"
      footer={
        <Button
          type="button"
          variant="record"
          disabled={
            !supported ||
            lowStorage ||
            sttBlocked ||
            permission === "denied" ||
            requestingMicrophone ||
            starting
          }
          onClick={start}
        >
          <Mic className="size-4" aria-hidden="true" />
          {starting ? "Начинаем…" : "Начать запись"}
        </Button>
      }
    >
      <div className="grid gap-4">
        <Field label="Куда вставить после проверки">
          <Select value={destination} onChange={(event) => setDestination(event.target.value)}>
            <option value="">Только в голосовые сессии</option>
            {nodes
              .filter((node) => !node.trashedAt)
              .map((node) => (
                <option key={node.id} value={node.id}>
                  {node.title}
                </option>
              ))}
          </Select>
        </Field>

        <div className="grid gap-4 sm:grid-cols-[1fr_1.15fr]">
          <Field label="Язык">
            <Select
              value={language}
              onChange={(event) => setLanguage(event.target.value as "ru" | "en")}
            >
              <option value="ru">Русский</option>
              <option value="en">English</option>
            </Select>
          </Field>
          <label
            htmlFor="preflight-commands"
            className="flex min-h-[70px] cursor-pointer items-end justify-between gap-3 rounded-[var(--radius-sm)] border border-[var(--border-subtle)] px-3 py-2.5"
          >
            <span className="min-w-0">
              <span className="block text-sm font-medium">Голосовые команды</span>
              <span className="mt-0.5 block truncate text-xs text-[var(--ink-tertiary)]">
                Префикс «{preferences.commandPrefix}»
              </span>
            </span>
            <Switch
              id="preflight-commands"
              checked={commandsEnabled}
              aria-label="Голосовые команды"
              onCheckedChange={(value) => {
                setCommandsEnabled(value);
                saveWriterPreferences({ ...preferences, commandsEnabled: value });
              }}
            />
          </label>
        </div>

        <Field label="Микрофон">
          <Select
            value={microphoneId}
            onChange={(event) => {
              const nextId = event.target.value;
              setMicrophoneId(nextId);
              saveWriterPreferences({ ...preferences, microphoneId: nextId });
              if (permission === "granted") {
                void openMicrophone(nextId);
              }
            }}
          >
            <option value="">Системный по умолчанию</option>
            {microphoneId && !microphones.some((device) => device.deviceId === microphoneId) ? (
              <option value={microphoneId}>Сохранённый микрофон сейчас недоступен</option>
            ) : null}
            {microphones.map((device, index) => (
              <option key={device.deviceId} value={device.deviceId}>
                {device.label || `Микрофон ${index + 1}`}
              </option>
            ))}
          </Select>
        </Field>

        <div className="rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] px-4 py-3">
          <div className="flex items-center gap-3">
            <span className="grid size-9 shrink-0 place-items-center rounded-full bg-[var(--surface-subtle)]">
              <Mic className="size-4" aria-hidden="true" />
            </span>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium">Проверка сигнала</p>
              <p className="text-xs text-[var(--ink-tertiary)]">
                {requestingMicrophone
                  ? "Ждём разрешения браузера…"
                  : permission === "granted"
                    ? signalDetected
                      ? "Доступ разрешён, сигнал поступает"
                      : trackMuted
                        ? "Доступ есть, но микрофон не передаёт звук"
                        : "Доступ разрешён — скажите несколько слов"
                    : permission === "denied"
                      ? "Доступ запрещён"
                      : permission === "insecure"
                        ? "Нужен HTTPS или localhost"
                        : supported
                          ? "Нужно проверить доступ"
                          : "Запись не поддерживается"}
              </p>
            </div>
            {supported && (permission !== "granted" || !signalDetected) ? (
              <Button
                type="button"
                variant="secondary"
                size="compact"
                disabled={requestingMicrophone}
                onClick={requestMicrophone}
              >
                {requestingMicrophone
                  ? "Ждём…"
                  : permission === "granted"
                    ? "Повторить"
                    : "Проверить"}
              </Button>
            ) : null}
          </div>

          <meter
            className="sr-only"
            aria-label="Уровень микрофона"
            min={0}
            max={100}
            value={Math.round(level * 100)}
          />
          <div className="mt-3 grid grid-cols-[repeat(24,minmax(0,1fr))] gap-1" aria-hidden="true">
            {meterSteps.map((step, index) => (
              <span
                key={step}
                className="h-1.5 rounded-full bg-[var(--border-subtle)] transition-colors duration-75 data-[active=true]:bg-[var(--accent)]"
                data-active={index / meterSteps.length < level}
              />
            ))}
          </div>
          <p className="mt-2 flex justify-between text-[11px] text-[var(--ink-tertiary)]">
            <span>
              {requestingMicrophone
                ? "Разрешите доступ во всплывающем окне браузера"
                : permission === "granted"
                  ? "Скажите несколько слов"
                  : "Проверка ещё не запущена"}
            </span>
            <span>{Math.round(level * 100)}%</span>
          </p>
        </div>

        <div className="flex items-start gap-3 text-xs leading-5 text-[var(--ink-secondary)]">
          <ShieldCheck
            className="mt-0.5 size-4 shrink-0 text-[var(--success)]"
            aria-hidden="true"
          />
          <p>
            Аудио сначала сохраняется на устройстве
            {availableBytes === undefined ? "" : ` · свободно ${formatBytes(availableBytes)}`}
            {online ? " · отправка доступна" : " · отправим после восстановления сети"}
            {online && sttCheck.status === "ready"
              ? liveTranscriptionAvailable
                ? " · live-текст готов"
                : " · текст после завершения"
              : ""}
            .
          </p>
        </div>
      </div>

      {online && sttCheck.status === "checking" ? (
        <Banner
          className="mt-4"
          variant="info"
          title="Проверяем распознавание речи"
          body="Начать запись можно после подтверждения рабочего STT-провайдера."
        />
      ) : null}
      {online && sttCheck.status === "unavailable" ? (
        <Banner
          className="mt-4"
          variant="danger"
          title="Распознавание речи не настроено"
          body="Запись не запустится с шаблонным текстом. Настройте Deepgram для live-режима или OpenRouter для расшифровки аудио."
        />
      ) : null}
      {online && sttCheck.status === "error" ? (
        <Banner
          className="mt-4"
          variant="danger"
          title="Не удалось проверить распознавание речи"
          body="Обновите страницу или проверьте соединение с сервером — до успешной проверки запись не начнётся."
        />
      ) : null}
      {online && sttCheck.status === "ready" && !languageSupported ? (
        <Banner
          className="mt-4"
          variant="danger"
          title="Выбранный язык не поддерживается"
          body="Выберите язык, который заявлен настроенным STT-провайдером."
        />
      ) : null}
      {online && sttCheck.status === "ready" && !liveTranscriptionAvailable ? (
        <Banner
          className="mt-4"
          variant="warning"
          title="Без live-текста"
          body="Аудио будет записано надёжно, а расшифровка появится после завершения сессии."
        />
      ) : null}
      {!online ? (
        <Banner
          className="mt-4"
          variant="warning"
          title="Можно записывать без сети"
          body="Живая расшифровка появится после восстановления соединения. Не очищайте данные сайта."
        />
      ) : null}
      {lowStorage ? (
        <Banner
          className="mt-4"
          variant="danger"
          title="Недостаточно свободного места"
          body="Освободите не менее 100 МБ, чтобы запись не оборвалась."
        />
      ) : null}
      {permission === "insecure" ? (
        <Banner
          className="mt-4"
          variant="warning"
          title="Браузер блокирует микрофон на HTTP-адресе"
          body="Откройте приложение по HTTPS. Если сервер запущен на этом же компьютере, используйте localhost — браузер считает его безопасным origin."
          action={
            <Button asChild variant="secondary" size="compact">
              <a href={localhostURL()}>Открыть localhost</a>
            </Button>
          }
        />
      ) : null}
      {error ? (
        <Banner className="mt-4" variant="danger" title="Проверка не пройдена" body={error} />
      ) : null}
    </ModalDialog>
  );
}

const meterSteps = Array.from({ length: 24 }, (_, index) => String(index + 1));

function formatBytes(bytes: number): string {
  if (bytes > 1024 * 1024 * 1024) {
    return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} ГБ`;
  }
  return `${Math.max(0, Math.round(bytes / 1024 / 1024))} МБ`;
}

function localhostURL(): string {
  const port = window.location.port ? `:${window.location.port}` : "";
  return `${window.location.protocol}//localhost${port}${window.location.pathname}${window.location.search}`;
}
