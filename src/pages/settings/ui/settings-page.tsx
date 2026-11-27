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
  Languages,
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
import { localizeMessage, t } from "~shared/i18n";
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
  InterfaceLanguagePicker,
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
      ? t("Введите целое число от 1 до 1 000 000 или оставьте поле пустым.")
      : projectAssistantSettings.isError
        ? t("Не удалось загрузить настройку проекта. Остальные настройки можно сохранить.")
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
        title: t("Настройки сохранены"),
        description:
          response.deletedObjects > 0
            ? t("Удалено просроченных аудиообъектов: {0}.", response.deletedObjects)
            : projectId
              ? t(
                  "Настройки интерфейса действуют в этом браузере, лимит ассистента — для текущего проекта.",
                )
              : t("Настройки редактора действуют в этом браузере, срок хранения — для аккаунта."),
      });
    },
    onError: (error) =>
      showToast({ title: t("Настройки не сохранены"), description: error.message }),
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
      showToast({ title: t("Аккаунт удалён"), description: localizeMessage(notice) });
      await navigate({ to: "/login", replace: true });
    },
    onError: (error) =>
      showToast({ title: t("Удаление не завершено"), description: error.message }),
  });

  return (
    <RequireSession>
      <main className="mx-auto min-h-full w-full max-w-[1120px] px-5 py-10 lg:px-0">
        <header className="mb-6 flex min-h-14 items-center gap-3 border-b border-[var(--border-subtle)] pb-5">
          <Button asChild variant="ghost" size="icon" aria-label={t("Назад")}>
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
              {t("Настройки")}
            </h1>
            <p className="mt-1 text-sm text-[var(--ink-tertiary)]">
              {projectId ? t("Проект и рабочее пространство") : t("Рабочее пространство")}
            </p>
          </div>
        </header>
        <div className="grid gap-6 lg:grid-cols-[250px_minmax(0,1fr)] lg:gap-9">
          <nav
            aria-label={t("Разделы настроек")}
            className="flex gap-1 overflow-x-auto pb-1 lg:sticky lg:top-6 lg:h-fit lg:flex-col lg:overflow-visible"
          >
            {[
              ["#interface-language", t("Язык интерфейса")],
              ["#assistant-settings", t("AI и ассистент")],
              ["#editor-settings", t("Интерфейс редактора")],
              ["#dictation-settings", t("Диктовка и команды")],
              ["#local-storage", t("Хранение")],
              ["#privacy-settings", t("Приватность")],
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
              <SettingsSection
                id="interface-language"
                icon={Languages}
                title={t("Язык интерфейса")}
              >
                <InterfaceLanguagePicker />
              </SettingsSection>
              <SettingsSection id="assistant-settings" icon={Bot} title={t("AI и ассистент")}>
                <p className="text-sm leading-6 text-[var(--ink-secondary)]">
                  {t(
                    "Выберите системную модель или добавьте свой OpenAI-compatible endpoint. Перед использованием приложение проверит настоящую цепочку tool-calls и возврат результата.",
                  )}
                </p>
                <Field
                  label={t("Пауза перед предложением выполнить")}
                  hint={t(
                    "Срабатывает только после распознанной речи. Поручение никогда не отправляется автоматически.",
                  )}
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
                    <option value={3000}>{t("3 секунды")}</option>
                    <option value={5000}>{t("5 секунд — по умолчанию")}</option>
                    <option value={8000}>{t("8 секунд")}</option>
                    <option value={0}>{t("Выключено")}</option>
                  </Select>
                </Field>
                {projectId ? (
                  <Field
                    label={t("Максимум шагов на поручение")}
                    hint={t(
                      "Пустое поле снимает ограничение. Укажите лимит, если хотите заранее ограничить расход токенов и стоимость долгой задачи.",
                    )}
                    error={assistantStepLimitError}
                  >
                    <Input
                      type="number"
                      inputMode="numeric"
                      min={1}
                      max={1_000_000}
                      step={1}
                      value={assistantStepLimit ?? ""}
                      placeholder={t("Без ограничений")}
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
                        {t("Модели и провайдеры")}
                      </span>
                      <ArrowRight className="size-4" aria-hidden="true" />
                    </Link>
                  </Button>
                  {projectId ? (
                    <Button asChild variant="secondary" className="min-h-14 justify-between px-4">
                      <Link to="/projects/$projectId/assistant-context" params={{ projectId }}>
                        <span className="flex items-center gap-3">
                          <Brain className="size-4 text-[var(--accent)]" aria-hidden="true" />
                          {t("Контекст текущего проекта")}
                        </span>
                        <ArrowRight className="size-4" aria-hidden="true" />
                      </Link>
                    </Button>
                  ) : null}
                  <Button asChild variant="secondary" className="min-h-14 justify-between px-4">
                    <Link to="/settings/mcp">
                      <span className="flex items-center gap-3">
                        <Network className="size-4 text-[var(--accent)]" aria-hidden="true" />
                        {t("Агенты и MCP")}
                      </span>
                      <ArrowRight className="size-4" aria-hidden="true" />
                    </Link>
                  </Button>
                </div>
              </SettingsSection>
              <SettingsSection id="dictation-settings" icon={Mic} title={t("Диктовка")}>
                <div className="grid items-start gap-x-5 gap-y-4 sm:grid-cols-2">
                  <Field label={t("Язык по умолчанию")}>
                    <Select
                      value={preferences.language}
                      onChange={(event) =>
                        setPreferences((current) => ({
                          ...current,
                          language: event.target.value as "ru" | "en",
                        }))
                      }
                    >
                      <option value="ru">{t("Русский")}</option>
                      <option value="en">English</option>
                    </Select>
                  </Field>
                  <Field
                    label={t("Микрофон по умолчанию")}
                    hint={t("Названия устройств появляются после разрешения доступа в браузере.")}
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
                      <option value="">{t("Системный по умолчанию")}</option>
                      {preferences.microphoneId &&
                      !microphones.some(
                        (device) => device.deviceId === preferences.microphoneId,
                      ) ? (
                        <option value={preferences.microphoneId}>
                          {t("Сохранённый микрофон сейчас недоступен")}
                        </option>
                      ) : null}
                      {microphones.map((device, index) => (
                        <option key={device.deviceId} value={device.deviceId}>
                          {device.label || t("Микрофон {0}", index + 1)}
                        </option>
                      ))}
                    </Select>
                  </Field>
                  <Field
                    label={t("Префикс голосовых команд")}
                    hint={t("Команда выполнится только после этого слова.")}
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
                    label={t("Распознавать голосовые команды")}
                    body={t(
                      "Поддерживаются: новый абзац, начало диалога, отбивка, метка, отмена и переход к абзацу, предложению или словам для правки.",
                    )}
                  />
                  <Toggle
                    checked={preferences.liveTranscript}
                    onChange={(liveTranscript) =>
                      setPreferences((current) => ({ ...current, liveTranscript }))
                    }
                    label={t("Показывать живую расшифровку")}
                    body={t("Отключение текста не останавливает запись аудио.")}
                  />
                </div>
              </SettingsSection>
              <SettingsSection id="editor-settings" icon={Keyboard} title={t("Редактор")}>
                <div className="grid items-start gap-x-5 gap-y-4 sm:grid-cols-2">
                  <Field label={t("Шрифт рукописи")}>
                    <Select
                      value={preferences.fontFamily}
                      onChange={(event) =>
                        setPreferences((current) => ({
                          ...current,
                          fontFamily: event.target.value as EditorFontFamily,
                        }))
                      }
                    >
                      <option value="sofia">{t("Sofia Sans — округлый (по умолчанию)")}</option>
                      <option value="inter">{t("Inter — утилитарный без засечек")}</option>
                      <option value="source-serif">
                        {t("Source Serif 4 — утилитарный с засечками")}
                      </option>
                    </Select>
                  </Field>
                  <Field label={t("Кегль")}>
                    <Select
                      value={preferences.fontSize}
                      onChange={(event) =>
                        setPreferences((current) => ({
                          ...current,
                          fontSize: Number(event.target.value) as EditorFontSize,
                        }))
                      }
                    >
                      <option value={17}>{t("17 px — компактный")}</option>
                      <option value={19}>{t("19 px — обычный")}</option>
                      <option value={21}>{t("21 px — крупный")}</option>
                    </Select>
                  </Field>
                  <Field label={t("Межстрочный интервал")}>
                    <Select
                      value={preferences.lineHeight}
                      onChange={(event) =>
                        setPreferences((current) => ({
                          ...current,
                          lineHeight: Number(event.target.value) as EditorLineHeight,
                        }))
                      }
                    >
                      <option value={1.5}>{t("1,5 — плотный")}</option>
                      <option value={1.75}>{t("1,75 — обычный")}</option>
                      <option value={2}>{t("2 — свободный")}</option>
                    </Select>
                  </Field>
                  <Field label={t("Интервал между абзацами")}>
                    <Select
                      value={preferences.paragraphSpacing}
                      onChange={(event) =>
                        setPreferences((current) => ({
                          ...current,
                          paragraphSpacing: Number(event.target.value) as EditorParagraphSpacing,
                        }))
                      }
                    >
                      <option value={0.65}>{t("0,65 em — плотный")}</option>
                      <option value={0.9}>{t("0,9 em — обычный")}</option>
                      <option value={1.1}>{t("1,1 em — свободный")}</option>
                    </Select>
                  </Field>
                  <Field label={t("Интервал элементов списка")}>
                    <Select
                      value={preferences.listItemSpacing}
                      onChange={(event) =>
                        setPreferences((current) => ({
                          ...current,
                          listItemSpacing: Number(event.target.value) as EditorListItemSpacing,
                        }))
                      }
                    >
                      <option value={0}>{t("Без дополнительного интервала")}</option>
                      <option value={0.25}>{t("0,25 em — обычный")}</option>
                      <option value={0.5}>{t("0,5 em — свободный")}</option>
                    </Select>
                  </Field>
                  <Field label={t("Ширина рукописи")}>
                    <Select
                      value={preferences.manuscriptWidth}
                      onChange={(event) =>
                        setPreferences((current) => ({
                          ...current,
                          manuscriptWidth: Number(event.target.value) as EditorManuscriptWidth,
                        }))
                      }
                    >
                      <option value={60}>{t("60 знаков — узкая")}</option>
                      <option value={70}>{t("70 знаков — обычная")}</option>
                      <option value={80}>{t("80 знаков — широкая")}</option>
                    </Select>
                  </Field>
                  <Field
                    label={t("Панель форматирования")}
                    hint={t("Авто раскрывает основные действия, когда редактору хватает ширины.")}
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
                      <option value="auto">{t("Авто")}</option>
                      <option value="expanded">{t("Расширенная")}</option>
                      <option value="compact">{t("Компактная")}</option>
                    </Select>
                  </Field>
                  <Field label={t("Тема")}>
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
                      <option value="system">{t("Как в системе")}</option>
                      <option value="light">{t("Светлая")}</option>
                      <option value="dark">{t("Тёмная")}</option>
                    </Select>
                  </Field>
                  <Field
                    label={t("Цветовая тема")}
                    className="sm:col-span-2"
                    hint={t(
                      "Палитра применяется независимо от светлого или тёмного режима. Наведите на цвет, чтобы увидеть его назначение.",
                    )}
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
                    label={t("Цель по словам")}
                    hint={t("0 отключает цель. Прогресс показывается внизу редактора.")}
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
                    label={t("Проверять орфографию средствами браузера")}
                    body={t("Подчёркивание ошибок зависит от словарей, установленных в браузере.")}
                  />
                  <Toggle
                    checked={preferences.autoPlaySource}
                    onChange={(autoPlaySource) =>
                      setPreferences((current) => ({ ...current, autoPlaySource }))
                    }
                    label={t("Автоматически воспроизводить источник")}
                    body={t("При переходе к абзацу с голосовым происхождением.")}
                  />
                </div>
              </SettingsSection>
              <SettingsSection
                id="privacy-settings"
                icon={Shield}
                title={t("Хранение и приватность")}
              >
                <Banner
                  variant="info"
                  title={t("Где обрабатывается голос")}
                  body={t(
                    "Живое аудио отправляется в Deepgram для расшифровки. OpenRouter используется только при явном повторном распознавании или как резерв, если live-сервис недоступен. Ключи провайдеров остаются на сервере.",
                  )}
                />
                <Field
                  label={t("Срок хранения исходного аудио на сервере")}
                  hint={t(
                    "Просроченные объекты удаляются при применении настройки. Тексты и история версий сохраняются.",
                  )}
                >
                  <Select
                    value={audioRetentionDays}
                    onChange={(event) =>
                      setAudioRetentionDays(Number(event.target.value) as 30 | 90 | 365)
                    }
                  >
                    <option value={30}>{t("30 дней")}</option>
                    <option value={90}>{t("90 дней")}</option>
                    <option value={365}>{t("1 год")}</option>
                  </Select>
                </Field>
                <div id="data-retention" className="text-sm leading-6 text-[var(--ink-secondary)]">
                  <a
                    href="#data-retention-details"
                    className="font-medium text-[var(--accent)] underline underline-offset-2"
                  >
                    {t("Политика хранения данных")}
                  </a>
                  <p id="data-retention-details" className="mt-2">
                    {t(
                      "Просроченное рабочее аудио удаляется при применении настройки. Рукописи, расшифровки и версии хранятся до удаления проекта или аккаунта. Срок физического удаления инфраструктурных резервных копий определяется оператором развёртывания.",
                    )}
                  </p>
                </div>
                <div>
                  <p className="text-sm leading-6 text-[var(--ink-secondary)]">
                    {t(
                      "ZIP-бэкап включает проекты, текущие документы, историю редакций, словарь, расшифровки и доступное исходное аудио. Пароль и серверные сессии в архив не попадают.",
                    )}
                  </p>
                  <Button asChild type="button" variant="secondary" className="mt-3">
                    <a href="/api/account/backup" download>
                      <Download className="size-4" aria-hidden="true" />
                      {t("Скачать полный бэкап")}
                    </a>
                  </Button>
                </div>
                {projectId ? (
                  <p className="text-sm leading-6 text-[var(--ink-secondary)]">
                    {t(
                      "Удаление текущего проекта доступно в разделе «Проекты»: сначала переместите его в корзину, затем подтвердите окончательное удаление точным названием.",
                    )}
                  </p>
                ) : null}
                <div className="rounded-[var(--radius-md)] border border-[var(--danger)]/40 bg-[var(--danger-soft)] p-4">
                  <h3 className="text-sm font-medium text-[var(--danger)]">
                    {t("Удалить аккаунт")}
                  </h3>
                  <p className="mt-1 text-xs leading-5 text-[var(--ink-secondary)]">
                    {t(
                      "Будут безвозвратно удалены проекты, рукописи, версии, словари, облачное аудио и локальные незавершённые записи этого браузера. Перед удалением скачайте бэкап.",
                    )}
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
                        title: t("Удалить аккаунт безвозвратно?"),
                        description: t(
                          "Отменить удаление нельзя. Резервные копии инфраструктуры могут храниться по политике оператора.",
                        ),
                        label: t("Адрес электронной почты"),
                        requiredValue: email,
                        icon: Trash2,
                        tone: "danger",
                        confirmLabel: t("Удалить аккаунт"),
                      }).then((confirmation) => {
                        if (confirmation !== undefined) {
                          purgeAccount.mutate(confirmation);
                        }
                      });
                    }}
                  >
                    <Trash2 className="size-4" aria-hidden="true" />
                    {t("Удалить аккаунт")}
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
                {t("Сохранить настройки")}
              </Button>
            </section>
            <aside className="grid h-fit gap-4">
              <SettingsSection id="local-storage" icon={Database} title={t("Локальное хранилище")}>
                <p className="text-sm leading-6 text-[var(--ink-secondary)]">
                  {t("Использовано ")}
                  {formatBytes(storage.usage)}
                  {t(" из ")}
                  {formatBytes(storage.quota)}
                  {t(". Здесь хранятся незавершённые аудиофрагменты и аварийные снимки редактора.")}
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
                  {storage.persistent
                    ? t("Защищённое хранение включено")
                    : t("Защитить от автоочистки")}
                </Button>
              </SettingsSection>
              <Banner
                variant="warning"
                title={t("Не очищайте данные сайта во время записи")}
                body={t(
                  "При работе без сети локальная копия может быть единственным полным источником до синхронизации.",
                )}
              />
              <div className="flex items-start gap-3 rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-4 text-sm text-[var(--ink-secondary)]">
                <WifiOff className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
                {t(
                  "Проект и редактор требуют сервер для окончательного сохранения; запись аудио может продолжаться офлайн.",
                )}
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
  if (value === undefined) return t("неизвестно");
  if (value > 1024 * 1024 * 1024) return t("{0} ГБ", (value / 1024 / 1024 / 1024).toFixed(1));
  return t("{0} МБ", Math.round(value / 1024 / 1024));
}
