import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import {
  ArrowLeft,
  ArrowRight,
  Bot,
  Brain,
  Database,
  Download,
  Keyboard,
  Mic,
  Network,
  Server,
  Shield,
  Trash2,
  WifiOff,
} from "lucide-react";
import { useEffect, useId, useState } from "react";

import { getProjectAssistantSettings, updateProjectAssistantSettings } from "~entities/project";
import { useSession } from "~entities/session";
import { apiRequest } from "~shared/api";
import {
  type AssistantSilenceCheckpointMs,
  applyWriterPreferences,
  clearLocalRecoveryData,
  type EditorFontFamily,
  type EditorFontSize,
  type EditorLineHeight,
  type EditorListItemSpacing,
  type EditorManuscriptWidth,
  type EditorParagraphSpacing,
  type EditorStyle,
  type EditorTheme,
  type EditorToolbarMode,
  loadWriterPreferences,
  saveWriterPreferences,
  type WriterPreferences,
} from "~shared/lib";
import {
  Banner,
  Button,
  Field,
  Input,
  Select,
  Switch,
  useActionDialog,
  useToast,
} from "~shared/ui";
import { RequireSession } from "~widgets/require-session";

import { EditorThemePicker } from "./editor-theme-picker";

export function SettingsPage({ projectId, nodeId }: { projectId?: string; nodeId?: string }) {
  const { showToast } = useToast();
  const { requestInput } = useActionDialog();
  const session = useSession();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [preferences, setPreferences] = useState<WriterPreferences>(loadWriterPreferences);
  const [storage, setStorage] = useState<{ usage?: number; quota?: number; persistent?: boolean }>(
    {},
  );
  const [microphones, setMicrophones] = useState<MediaDeviceInfo[]>([]);
  const [audioRetentionDays, setAudioRetentionDays] = useState<30 | 90 | 365>(365);
  const [assistantStepLimit, setAssistantStepLimit] = useState<number | undefined>();
  const serverPreferences = useQuery({
    queryKey: ["preferences"],
    queryFn: () =>
      apiRequest<{ preferences: { audioRetentionDays: 30 | 90 | 365 } }>("/api/preferences"),
  });
  const projectAssistantSettings = useQuery({
    queryKey: ["project", projectId, "assistant-settings"],
    queryFn: () => getProjectAssistantSettings(projectId ?? ""),
    enabled: Boolean(projectId),
  });

  useEffect(() => {
    void navigator.storage
      ?.estimate()
      .then((estimate) =>
        setStorage((current) => ({ ...current, usage: estimate.usage, quota: estimate.quota })),
      );
    void navigator.storage
      ?.persisted?.()
      .then((persistent) => setStorage((current) => ({ ...current, persistent })));
  }, []);

  useEffect(
    () => () => {
      applyWriterPreferences(loadWriterPreferences());
    },
    [],
  );

  useEffect(() => {
    const refreshMicrophones = async () => {
      if (!navigator.mediaDevices?.enumerateDevices) {
        return;
      }
      try {
        const devices = await navigator.mediaDevices.enumerateDevices();
        setMicrophones(devices.filter((device) => device.kind === "audioinput"));
      } catch {
        setMicrophones([]);
      }
    };
    void refreshMicrophones();
    navigator.mediaDevices?.addEventListener?.("devicechange", refreshMicrophones);
    return () => {
      navigator.mediaDevices?.removeEventListener?.("devicechange", refreshMicrophones);
    };
  }, []);

  useEffect(() => {
    if (serverPreferences.data?.preferences.audioRetentionDays) {
      setAudioRetentionDays(serverPreferences.data.preferences.audioRetentionDays);
    }
  }, [serverPreferences.data?.preferences.audioRetentionDays]);

  useEffect(() => {
    if (projectAssistantSettings.data) {
      setAssistantStepLimit(projectAssistantSettings.data.settings.stepLimit);
    }
  }, [projectAssistantSettings.data]);

  const assistantStepLimitError =
    assistantStepLimit !== undefined &&
    (!Number.isInteger(assistantStepLimit) ||
      assistantStepLimit < 1 ||
      assistantStepLimit > 1_000_000)
      ? "Введите целое число от 1 до 1 000 000 или оставьте поле пустым."
      : projectAssistantSettings.isError
        ? "Не удалось загрузить настройку проекта. Остальные настройки можно сохранить."
        : undefined;

  const save = useMutation({
    mutationFn: async () => {
      const [preferencesResponse, projectSettingsResponse] = await Promise.all([
        apiRequest<{
          preferences: { audioRetentionDays: number };
          deletedObjects: number;
        }>("/api/preferences", {
          method: "PUT",
          body: JSON.stringify({ audioRetentionDays }),
        }),
        projectId && projectAssistantSettings.isSuccess
          ? updateProjectAssistantSettings(projectId, { stepLimit: assistantStepLimit })
          : Promise.resolve(undefined),
      ]);
      return { ...preferencesResponse, projectSettingsResponse };
    },
    onSuccess: (response) => {
      const normalized = saveWriterPreferences(preferences);
      setPreferences(normalized);
      if (projectId && response.projectSettingsResponse) {
        queryClient.setQueryData(
          ["project", projectId, "assistant-settings"],
          response.projectSettingsResponse,
        );
        void queryClient.invalidateQueries({ queryKey: ["projects"] });
      }
      showToast({
        title: "Настройки сохранены",
        description:
          response.deletedObjects > 0
            ? `Удалено просроченных аудиообъектов: ${response.deletedObjects}.`
            : projectId
              ? "Настройки интерфейса действуют в этом браузере, лимит ассистента — для текущего проекта."
              : "Настройки редактора действуют в этом браузере, срок хранения — для аккаунта.",
      });
    },
    onError: (error) => showToast({ title: "Настройки не сохранены", description: error.message }),
  });
  const purgeAccount = useMutation({
    mutationFn: (confirmation: string) =>
      apiRequest<{ purged: boolean; notice: string }>("/api/account", {
        method: "DELETE",
        body: JSON.stringify({ confirmation }),
      }),
    onSuccess: async ({ notice }) => {
      await clearLocalRecoveryData();
      localStorage.clear();
      queryClient.clear();
      showToast({ title: "Аккаунт удалён", description: notice });
      await navigate({ to: "/login", replace: true });
    },
    onError: (error) => showToast({ title: "Удаление не завершено", description: error.message }),
  });

  return (
    <RequireSession>
      <main className="mx-auto min-h-full w-full max-w-[1120px] px-5 py-10 lg:px-0">
        <header className="mb-6 flex min-h-14 items-center gap-3 border-b border-[var(--border-subtle)] pb-5">
          <Button asChild variant="ghost" size="icon" aria-label="Назад">
            {projectId && nodeId ? (
              <Link to="/projects/$projectId/studio/$nodeId" params={{ projectId, nodeId }}>
                <ArrowLeft className="size-4" aria-hidden="true" />
              </Link>
            ) : projectId ? (
              <Link to="/projects/$projectId" params={{ projectId }}>
                <ArrowLeft className="size-4" aria-hidden="true" />
              </Link>
            ) : (
              <Link to="/projects">
                <ArrowLeft className="size-4" aria-hidden="true" />
              </Link>
            )}
          </Button>
          <div className="min-w-0">
            <h1 className="font-display truncate text-[30px] leading-tight sm:text-[34px]">
              Настройки
            </h1>
            <p className="mt-1 text-sm text-[var(--ink-tertiary)]">
              {projectId ? "Проект и рабочее пространство" : "Рабочее пространство"}
            </p>
          </div>
        </header>
        <div className="grid gap-6 lg:grid-cols-[250px_minmax(0,1fr)] lg:gap-9">
          <nav
            aria-label="Разделы настроек"
            className="flex gap-1 overflow-x-auto pb-1 lg:sticky lg:top-6 lg:h-fit lg:flex-col lg:overflow-visible"
          >
            {[
              ["#assistant-settings", "AI и ассистент"],
              ["#editor-settings", "Интерфейс редактора"],
              ["#dictation-settings", "Диктовка и команды"],
              ["#local-storage", "Хранение"],
              ["#privacy-settings", "Приватность"],
            ].map(([href, label]) => (
              <a
                key={href}
                href={href}
                className="inline-flex min-h-11 shrink-0 items-center rounded-[var(--radius-sm)] px-3 text-sm text-[var(--ink-secondary)] hover:bg-[var(--surface-subtle)] hover:text-[var(--ink-primary)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus)]"
              >
                {label}
              </a>
            ))}
          </nav>
          <div className="grid min-w-0 gap-6">
            <section className="grid min-w-0 gap-6">
              <SettingsSection id="assistant-settings" icon={Bot} title="AI и ассистент">
                <p className="text-sm leading-6 text-[var(--ink-secondary)]">
                  Выберите системную модель или добавьте свой OpenAI-compatible endpoint. Перед
                  использованием приложение проверит настоящую цепочку tool-calls и возврат
                  результата.
                </p>
                <Field
                  label="Пауза перед предложением выполнить"
                  hint="Срабатывает только после распознанной речи. Поручение никогда не отправляется автоматически."
                >
                  <Select
                    value={preferences.assistantSilenceCheckpointMs}
                    onChange={(event) =>
                      setPreferences((current) => ({
                        ...current,
                        assistantSilenceCheckpointMs: Number(
                          event.target.value,
                        ) as AssistantSilenceCheckpointMs,
                      }))
                    }
                  >
                    <option value={3000}>3 секунды</option>
                    <option value={5000}>5 секунд — по умолчанию</option>
                    <option value={8000}>8 секунд</option>
                    <option value={0}>Выключено</option>
                  </Select>
                </Field>
                {projectId ? (
                  <Field
                    label="Максимум шагов на поручение"
                    hint="Пустое поле снимает ограничение. Укажите лимит, если хотите заранее ограничить расход токенов и стоимость долгой задачи."
                    error={assistantStepLimitError}
                  >
                    <Input
                      type="number"
                      inputMode="numeric"
                      min={1}
                      max={1_000_000}
                      step={1}
                      value={assistantStepLimit ?? ""}
                      placeholder="Без ограничений"
                      disabled={
                        projectAssistantSettings.isLoading || projectAssistantSettings.isError
                      }
                      onChange={(event) =>
                        setAssistantStepLimit(
                          event.target.value === "" ? undefined : Number(event.target.value),
                        )
                      }
                    />
                  </Field>
                ) : null}
                <div className="grid gap-3 sm:grid-cols-2">
                  <Button asChild variant="secondary" className="min-h-14 justify-between px-4">
                    <Link to="/settings/assistant-providers">
                      <span className="flex items-center gap-3">
                        <Server className="size-4 text-[var(--accent)]" aria-hidden="true" />
                        Модели и провайдеры
                      </span>
                      <ArrowRight className="size-4" aria-hidden="true" />
                    </Link>
                  </Button>
                  {projectId ? (
                    <Button asChild variant="secondary" className="min-h-14 justify-between px-4">
                      <Link to="/projects/$projectId/assistant-context" params={{ projectId }}>
                        <span className="flex items-center gap-3">
                          <Brain className="size-4 text-[var(--accent)]" aria-hidden="true" />
                          Контекст текущего проекта
                        </span>
                        <ArrowRight className="size-4" aria-hidden="true" />
                      </Link>
                    </Button>
                  ) : null}
                  <Button asChild variant="secondary" className="min-h-14 justify-between px-4">
                    <Link to="/settings/mcp">
                      <span className="flex items-center gap-3">
                        <Network className="size-4 text-[var(--accent)]" aria-hidden="true" />
                        Агенты и MCP
                      </span>
                      <ArrowRight className="size-4" aria-hidden="true" />
                    </Link>
                  </Button>
                </div>
              </SettingsSection>
              <SettingsSection id="dictation-settings" icon={Mic} title="Диктовка">
                <div className="grid items-start gap-x-5 gap-y-4 sm:grid-cols-2">
                  <Field label="Язык по умолчанию">
                    <Select
                      value={preferences.language}
                      onChange={(event) =>
                        setPreferences((current) => ({
                          ...current,
                          language: event.target.value as "ru" | "en",
                        }))
                      }
                    >
                      <option value="ru">Русский</option>
                      <option value="en">English</option>
                    </Select>
                  </Field>
                  <Field
                    label="Микрофон по умолчанию"
                    hint="Названия устройств появляются после разрешения доступа в браузере."
                  >
                    <Select
                      value={preferences.microphoneId}
                      onChange={(event) =>
                        setPreferences((current) => ({
                          ...current,
                          microphoneId: event.target.value,
                        }))
                      }
                    >
                      <option value="">Системный по умолчанию</option>
                      {preferences.microphoneId &&
                      !microphones.some(
                        (device) => device.deviceId === preferences.microphoneId,
                      ) ? (
                        <option value={preferences.microphoneId}>
                          Сохранённый микрофон сейчас недоступен
                        </option>
                      ) : null}
                      {microphones.map((device, index) => (
                        <option key={device.deviceId} value={device.deviceId}>
                          {device.label || `Микрофон ${index + 1}`}
                        </option>
                      ))}
                    </Select>
                  </Field>
                  <Field
                    label="Префикс голосовых команд"
                    hint="Команда выполнится только после этого слова."
                    className="sm:col-span-2"
                  >
                    <Input
                      value={preferences.commandPrefix}
                      maxLength={40}
                      onChange={(event) =>
                        setPreferences((current) => ({
                          ...current,
                          commandPrefix: event.target.value,
                        }))
                      }
                    />
                  </Field>
                </div>
                <div className="divide-y divide-[var(--border-subtle)] overflow-hidden rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-raised)]">
                  <Toggle
                    checked={preferences.commandsEnabled}
                    onChange={(commandsEnabled) =>
                      setPreferences((current) => ({ ...current, commandsEnabled }))
                    }
                    label="Распознавать голосовые команды"
                    body="Поддерживаются: новый абзац, начало диалога, отбивка, метка, отмена и переход к абзацу, предложению или словам для правки."
                  />
                  <Toggle
                    checked={preferences.liveTranscript}
                    onChange={(liveTranscript) =>
                      setPreferences((current) => ({ ...current, liveTranscript }))
                    }
                    label="Показывать живую расшифровку"
                    body="Отключение текста не останавливает запись аудио."
                  />
                </div>
              </SettingsSection>
              <SettingsSection id="editor-settings" icon={Keyboard} title="Редактор">
                <div className="grid items-start gap-x-5 gap-y-4 sm:grid-cols-2">
                  <Field label="Шрифт рукописи">
                    <Select
                      value={preferences.fontFamily}
                      onChange={(event) =>
                        setPreferences((current) => ({
                          ...current,
                          fontFamily: event.target.value as EditorFontFamily,
                        }))
                      }
                    >
                      <option value="sofia">Sofia Sans — округлый (по умолчанию)</option>
                      <option value="inter">Inter — утилитарный без засечек</option>
                      <option value="source-serif">Source Serif 4 — утилитарный с засечками</option>
                    </Select>
                  </Field>
                  <Field label="Кегль">
                    <Select
                      value={preferences.fontSize}
                      onChange={(event) =>
                        setPreferences((current) => ({
                          ...current,
                          fontSize: Number(event.target.value) as EditorFontSize,
                        }))
                      }
                    >
                      <option value={17}>17 px — компактный</option>
                      <option value={19}>19 px — обычный</option>
                      <option value={21}>21 px — крупный</option>
                    </Select>
                  </Field>
                  <Field label="Межстрочный интервал">
                    <Select
                      value={preferences.lineHeight}
                      onChange={(event) =>
                        setPreferences((current) => ({
                          ...current,
                          lineHeight: Number(event.target.value) as EditorLineHeight,
                        }))
                      }
                    >
                      <option value={1.5}>1,5 — плотный</option>
                      <option value={1.75}>1,75 — обычный</option>
                      <option value={2}>2 — свободный</option>
                    </Select>
                  </Field>
                  <Field label="Интервал между абзацами">
                    <Select
                      value={preferences.paragraphSpacing}
                      onChange={(event) =>
                        setPreferences((current) => ({
                          ...current,
                          paragraphSpacing: Number(event.target.value) as EditorParagraphSpacing,
                        }))
                      }
                    >
                      <option value={0.65}>0,65 em — плотный</option>
                      <option value={0.9}>0,9 em — обычный</option>
                      <option value={1.1}>1,1 em — свободный</option>
                    </Select>
                  </Field>
                  <Field label="Интервал элементов списка">
                    <Select
                      value={preferences.listItemSpacing}
                      onChange={(event) =>
                        setPreferences((current) => ({
                          ...current,
                          listItemSpacing: Number(event.target.value) as EditorListItemSpacing,
                        }))
                      }
                    >
                      <option value={0}>Без дополнительного интервала</option>
                      <option value={0.25}>0,25 em — обычный</option>
                      <option value={0.5}>0,5 em — свободный</option>
                    </Select>
                  </Field>
                  <Field label="Ширина рукописи">
                    <Select
                      value={preferences.manuscriptWidth}
                      onChange={(event) =>
                        setPreferences((current) => ({
                          ...current,
                          manuscriptWidth: Number(event.target.value) as EditorManuscriptWidth,
                        }))
                      }
                    >
                      <option value={60}>60 знаков — узкая</option>
                      <option value={70}>70 знаков — обычная</option>
                      <option value={80}>80 знаков — широкая</option>
                    </Select>
                  </Field>
                  <Field
                    label="Панель форматирования"
                    hint="Авто раскрывает основные действия, когда редактору хватает ширины."
                  >
                    <Select
                      value={preferences.toolbarMode}
                      onChange={(event) =>
                        setPreferences((current) => ({
                          ...current,
                          toolbarMode: event.target.value as EditorToolbarMode,
                        }))
                      }
                    >
                      <option value="auto">Авто</option>
                      <option value="expanded">Расширенная</option>
                      <option value="compact">Компактная</option>
                    </Select>
                  </Field>
                  <Field label="Тема">
                    <Select
                      value={preferences.theme}
                      onChange={(event) => {
                        const theme = event.target.value as EditorTheme;
                        setPreferences((current) => {
                          const next = {
                            ...current,
                            theme,
                          };
                          applyWriterPreferences(next);
                          return next;
                        });
                      }}
                    >
                      <option value="system">Как в системе</option>
                      <option value="light">Светлая</option>
                      <option value="dark">Тёмная</option>
                    </Select>
                  </Field>
                  <Field
                    label="Цветовая тема"
                    hint="Палитра применяется независимо от светлого или тёмного режима. Наведите на цвет, чтобы увидеть его назначение."
                  >
                    <EditorThemePicker
                      value={preferences.style}
                      onChange={(style: EditorStyle) => {
                        setPreferences((current) => {
                          const next = {
                            ...current,
                            style,
                          };
                          applyWriterPreferences(next);
                          return next;
                        });
                      }}
                    />
                  </Field>
                  <Field
                    label="Цель по словам"
                    hint="0 отключает цель. Прогресс показывается внизу редактора."
                  >
                    <Input
                      type="number"
                      inputMode="numeric"
                      min={0}
                      max={1_000_000}
                      step={100}
                      value={preferences.wordGoal}
                      onChange={(event) =>
                        setPreferences((current) => ({
                          ...current,
                          wordGoal: Number(event.target.value),
                        }))
                      }
                    />
                  </Field>
                </div>
                <div className="divide-y divide-[var(--border-subtle)] overflow-hidden rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-raised)]">
                  <Toggle
                    checked={preferences.spellcheck}
                    onChange={(spellcheck) =>
                      setPreferences((current) => ({ ...current, spellcheck }))
                    }
                    label="Проверять орфографию средствами браузера"
                    body="Подчёркивание ошибок зависит от словарей, установленных в браузере."
                  />
                  <Toggle
                    checked={preferences.autoPlaySource}
                    onChange={(autoPlaySource) =>
                      setPreferences((current) => ({ ...current, autoPlaySource }))
                    }
                    label="Автоматически воспроизводить источник"
                    body="При переходе к абзацу с голосовым происхождением."
                  />
                </div>
              </SettingsSection>
              <SettingsSection id="privacy-settings" icon={Shield} title="Хранение и приватность">
                <Banner
                  variant="info"
                  title="Где обрабатывается голос"
                  body="Живое аудио отправляется в Deepgram для расшифровки. OpenRouter используется только при явном повторном распознавании или как резерв, если live-сервис недоступен. Ключи провайдеров остаются на сервере."
                />
                <Field
                  label="Срок хранения исходного аудио на сервере"
                  hint="Просроченные объекты удаляются при применении настройки. Тексты и история версий сохраняются."
                >
                  <Select
                    value={audioRetentionDays}
                    onChange={(event) =>
                      setAudioRetentionDays(Number(event.target.value) as 30 | 90 | 365)
                    }
                  >
                    <option value={30}>30 дней</option>
                    <option value={90}>90 дней</option>
                    <option value={365}>1 год</option>
                  </Select>
                </Field>
                <div id="data-retention" className="text-sm leading-6 text-[var(--ink-secondary)]">
                  <a
                    href="#data-retention-details"
                    className="font-medium text-[var(--accent)] underline underline-offset-2"
                  >
                    Политика хранения данных
                  </a>
                  <p id="data-retention-details" className="mt-2">
                    Просроченное рабочее аудио удаляется при применении настройки. Рукописи,
                    расшифровки и версии хранятся до удаления проекта или аккаунта. Срок физического
                    удаления инфраструктурных резервных копий определяется оператором развёртывания.
                  </p>
                </div>
                <div>
                  <p className="text-sm leading-6 text-[var(--ink-secondary)]">
                    ZIP-бэкап включает проекты, текущие документы, историю редакций, словарь,
                    расшифровки и доступное исходное аудио. Пароль и серверные сессии в архив не
                    попадают.
                  </p>
                  <Button asChild type="button" variant="secondary" className="mt-3">
                    <a href="/api/account/backup" download>
                      <Download className="size-4" aria-hidden="true" />
                      Скачать полный бэкап
                    </a>
                  </Button>
                </div>
                {projectId ? (
                  <p className="text-sm leading-6 text-[var(--ink-secondary)]">
                    Удаление текущего проекта доступно в разделе «Проекты»: сначала переместите его
                    в корзину, затем подтвердите окончательное удаление точным названием.
                  </p>
                ) : null}
                <div className="rounded-[var(--radius-md)] border border-[var(--danger)]/40 bg-[var(--danger-soft)] p-4">
                  <h3 className="text-sm font-medium text-[var(--danger)]">Удалить аккаунт</h3>
                  <p className="mt-1 text-xs leading-5 text-[var(--ink-secondary)]">
                    Будут безвозвратно удалены проекты, рукописи, версии, словари, облачное аудио и
                    локальные незавершённые записи этого браузера. Перед удалением скачайте бэкап.
                  </p>
                  <Button
                    type="button"
                    variant="destructive"
                    className="mt-3"
                    disabled={!session.data?.user.email || purgeAccount.isPending}
                    onClick={() => {
                      const email = session.data?.user.email;
                      if (!email) {
                        return;
                      }
                      void requestInput({
                        title: "Удалить аккаунт безвозвратно?",
                        description:
                          "Отменить удаление нельзя. Резервные копии инфраструктуры могут храниться по политике оператора.",
                        label: "Адрес электронной почты",
                        requiredValue: email,
                        icon: Trash2,
                        tone: "danger",
                        confirmLabel: "Удалить аккаунт",
                      }).then((confirmation) => {
                        if (confirmation !== undefined) {
                          purgeAccount.mutate(confirmation);
                        }
                      });
                    }}
                  >
                    <Trash2 className="size-4" aria-hidden="true" />
                    Удалить аккаунт
                  </Button>
                </div>
              </SettingsSection>
              <Button
                type="button"
                className="justify-self-start"
                disabled={
                  save.isPending ||
                  Boolean(assistantStepLimitError && !projectAssistantSettings.isError)
                }
                onClick={() => save.mutate()}
              >
                Сохранить настройки
              </Button>
            </section>
            <aside className="grid h-fit gap-4">
              <SettingsSection id="local-storage" icon={Database} title="Локальное хранилище">
                <p className="text-sm leading-6 text-[var(--ink-secondary)]">
                  Использовано {formatBytes(storage.usage)} из {formatBytes(storage.quota)}. Здесь
                  хранятся незавершённые аудиофрагменты и аварийные снимки редактора.
                </p>
                <Button
                  type="button"
                  variant="secondary"
                  disabled={storage.persistent}
                  onClick={async () => {
                    const persistent = await navigator.storage?.persist?.();
                    setStorage((current) => ({ ...current, persistent }));
                  }}
                >
                  <Shield className="size-4" aria-hidden="true" />
                  {storage.persistent ? "Защищённое хранение включено" : "Защитить от автоочистки"}
                </Button>
              </SettingsSection>
              <Banner
                variant="warning"
                title="Не очищайте данные сайта во время записи"
                body="При работе без сети локальная копия может быть единственным полным источником до синхронизации."
              />
              <div className="flex items-start gap-3 rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-4 text-sm text-[var(--ink-secondary)]">
                <WifiOff className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
                Проект и редактор требуют сервер для окончательного сохранения; запись аудио может
                продолжаться офлайн.
              </div>
            </aside>
          </div>
        </div>
      </main>
    </RequireSession>
  );
}

function SettingsSection({
  id,
  icon: Icon,
  title,
  children,
}: {
  id?: string;
  icon: typeof Mic;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <section
      id={id}
      className="scroll-mt-6 rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-5 sm:p-6"
    >
      <h2 className="flex items-center gap-2 font-medium">
        <Icon className="size-4 text-[var(--accent)]" aria-hidden="true" />
        {title}
      </h2>
      <div className="mt-6 grid gap-6">{children}</div>
    </section>
  );
}

function Toggle({
  checked,
  onChange,
  label,
  body,
}: {
  checked: boolean;
  onChange: (value: boolean) => void;
  label: string;
  body: string;
}) {
  const id = useId();
  return (
    <label
      htmlFor={id}
      className="flex min-h-[76px] cursor-pointer items-center justify-between gap-4 px-4 py-3"
    >
      <span>
        <span className="block text-sm font-medium">{label}</span>
        <span className="mt-0.5 block text-xs leading-5 text-[var(--ink-tertiary)]">{body}</span>
      </span>
      <Switch id={id} checked={checked} onCheckedChange={onChange} />
    </label>
  );
}

function formatBytes(value?: number): string {
  if (value === undefined) return "неизвестно";
  if (value > 1024 * 1024 * 1024) return `${(value / 1024 / 1024 / 1024).toFixed(1)} ГБ`;
  return `${Math.round(value / 1024 / 1024)} МБ`;
}
