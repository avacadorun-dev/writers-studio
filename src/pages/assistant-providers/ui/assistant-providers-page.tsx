import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  ArrowLeft,
  CheckCircle2,
  KeyRound,
  Plus,
  RefreshCw,
  Server,
  ShieldCheck,
  Trash2,
  TriangleAlert,
} from "lucide-react";
import { type FormEvent, useId, useState } from "react";

import {
  createAssistantProvider,
  disableAssistantProvider,
  listAssistantProviders,
  probeAssistantProvider,
  probeSystemAssistantProvider,
  setDefaultAssistantProvider,
} from "~entities/assistant";
import { Banner, Button, Field, Input, Select, Spinner, Switch, useToast } from "~shared/ui";
import { RequireSession } from "~widgets/require-session";

export function AssistantProvidersPage() {
  const queryClient = useQueryClient();
  const { showToast } = useToast();
  const providers = useQuery({
    queryKey: ["assistant-providers"],
    queryFn: listAssistantProviders,
  });
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState({
    name: "",
    baseUrl: "",
    dialect: "auto" as "auto" | "responses" | "chat_completions",
    model: "",
    apiKey: "",
    allowPrivateEndpoint: false,
    isDefault: false,
  });
  const invalidate = () => queryClient.invalidateQueries({ queryKey: ["assistant-providers"] });
  const create = useMutation({
    mutationFn: () =>
      createAssistantProvider({
        ...form,
        headers: {},
      }),
    onSuccess: async ({ connection }) => {
      setShowForm(false);
      setForm({
        name: "",
        baseUrl: "",
        dialect: "auto",
        model: "",
        apiKey: "",
        allowPrivateEndpoint: false,
        isDefault: false,
      });
      await invalidate();
      showToast({
        title:
          connection.status === "compatible"
            ? "Провайдер совместим"
            : "Провайдер подключён с ограничениями",
        description: connection.statusDetail,
      });
    },
  });
  const probe = useMutation({
    mutationFn: probeAssistantProvider,
    onSuccess: invalidate,
  });
  const probeSystem = useMutation({
    mutationFn: probeSystemAssistantProvider,
    onSuccess: invalidate,
  });
  const makeDefault = useMutation({
    mutationFn: setDefaultAssistantProvider,
    onSuccess: invalidate,
  });
  const disable = useMutation({
    mutationFn: disableAssistantProvider,
    onSuccess: invalidate,
  });
  const submit = (event: FormEvent) => {
    event.preventDefault();
    create.mutate();
  };

  if (providers.isPending) return <Spinner label="Проверяем LLM-провайдеры…" />;
  if (providers.isError || !providers.data) {
    return <Banner variant="danger" title="Провайдеры недоступны" body="Обновите страницу." />;
  }
  const connections = providers.data.connections ?? [];
  const systemCompatible =
    providers.data.systemStatus === "compatible" || providers.data.systemStatus === "limited";

  return (
    <RequireSession>
      <div className="min-h-full bg-[var(--surface-app)]">
        <header className="flex h-14 items-center gap-3 border-b border-[var(--border-subtle)] bg-[var(--surface-panel)] px-4">
          <Button asChild variant="ghost" size="icon" aria-label="Назад в настройки">
            <Link to="/settings">
              <ArrowLeft className="size-4" aria-hidden="true" />
            </Link>
          </Button>
          <Server className="size-5 text-[var(--accent)]" aria-hidden="true" />
          <p className="text-sm font-semibold">Модели и провайдеры ассистента</p>
          <Button
            className="ml-auto"
            size="compact"
            onClick={() => setShowForm((current) => !current)}
            disabled={!providers.data.connectionsEnabled}
          >
            <Plus className="size-4" aria-hidden="true" />
            Добавить
          </Button>
        </header>
        <main className="mx-auto w-full max-w-[980px] px-4 py-8 sm:px-8">
          <div className="flex items-start gap-4">
            <div className="grid size-12 shrink-0 place-items-center rounded-full bg-[var(--accent-soft)] text-[var(--accent)]">
              <KeyRound className="size-5" aria-hidden="true" />
            </div>
            <div>
              <h1 className="font-display text-4xl">OpenAI-compatible подключения</h1>
              <p className="mt-2 max-w-2xl text-sm leading-6 text-[var(--ink-secondary)]">
                Перед сохранением «Студия автора» проверяет текст, строгие tools, возврат результата
                и два последовательных вызова. Ключ шифруется на сервере и никогда не возвращается в
                браузер или модели.
              </p>
            </div>
          </div>
          {!providers.data.connectionsEnabled ? (
            <Banner
              className="mt-6"
              variant="warning"
              title="Свои провайдеры отключены оператором"
              body="Для безопасного хранения API-ключей нужно задать LLM_CREDENTIAL_ENCRYPTION_KEY. Системная модель продолжит работать."
            />
          ) : null}
          <section className="mt-6 rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-5">
            <p className="text-[10px] font-semibold tracking-[0.14em] text-[var(--ink-tertiary)] uppercase">
              Системный провайдер
            </p>
            {providers.data.system ? (
              <div className="mt-3 flex flex-wrap items-center gap-4">
                <span
                  className={`grid size-10 place-items-center rounded ${
                    systemCompatible
                      ? "bg-[var(--success-soft)] text-[var(--success)]"
                      : "bg-[var(--warning-soft)] text-[var(--warning)]"
                  }`}
                >
                  {systemCompatible ? (
                    <CheckCircle2 className="size-5" aria-hidden="true" />
                  ) : (
                    <TriangleAlert className="size-5" aria-hidden="true" />
                  )}
                </span>
                <div className="min-w-0 flex-1">
                  <p className="font-semibold">{providers.data.system.name}</p>
                  <p className="font-mono mt-1 truncate text-[11px] text-[var(--ink-tertiary)]">
                    {providers.data.system.model} · {providers.data.system.dialect}
                  </p>
                  <p className="mt-2 text-[12px] text-[var(--ink-secondary)]">
                    {providers.data.systemStatusDetail || "Совместимость ещё не проверена"}
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  <span className="rounded-full bg-[var(--surface-subtle)] px-3 py-1 text-[10px]">
                    {systemCompatible ? "инструменты работают" : "нужна проверка"}
                  </span>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="Повторить проверку системной модели"
                    disabled={probeSystem.isPending}
                    onClick={() => probeSystem.mutate()}
                  >
                    <RefreshCw
                      className={`size-4 ${probeSystem.isPending ? "animate-spin" : ""}`}
                      aria-hidden="true"
                    />
                  </Button>
                </div>
              </div>
            ) : (
              <p className="mt-3 text-sm text-[var(--ink-secondary)]">Не настроен</p>
            )}
          </section>

          {showForm ? (
            <form
              autoComplete="off"
              onSubmit={submit}
              className="mt-6 rounded-[var(--radius-lg)] border border-[var(--accent)] bg-[var(--surface-paper)] p-5 sm:p-6"
            >
              <h2 className="font-display text-2xl">Новое подключение</h2>
              <div className="mt-5 grid gap-4 sm:grid-cols-2">
                <Field label="Название">
                  <Input
                    required
                    name="provider-display-name"
                    autoComplete="off"
                    value={form.name}
                    placeholder="Локальная LLM"
                    onChange={(event) =>
                      setForm((current) => ({ ...current, name: event.target.value }))
                    }
                  />
                </Field>
                <Field label="Диалект API">
                  <Select
                    value={form.dialect}
                    onChange={(event) =>
                      setForm((current) => ({
                        ...current,
                        dialect: event.target.value as typeof form.dialect,
                      }))
                    }
                  >
                    <option value="auto">Определить автоматически</option>
                    <option value="responses">Responses API</option>
                    <option value="chat_completions">Chat Completions</option>
                  </Select>
                </Field>
                <Field label="Base URL" className="sm:col-span-2">
                  <Input
                    required
                    type="url"
                    name="provider-base-url"
                    autoComplete="url"
                    value={form.baseUrl}
                    placeholder="https://api.example.com/v1"
                    onChange={(event) =>
                      setForm((current) => ({ ...current, baseUrl: event.target.value }))
                    }
                  />
                </Field>
                <Field label="Model ID">
                  <Input
                    required
                    name="provider-model-id"
                    autoComplete="off"
                    data-1p-ignore="true"
                    data-bwignore="true"
                    data-lpignore="true"
                    value={form.model}
                    placeholder="provider/model-name"
                    onChange={(event) =>
                      setForm((current) => ({ ...current, model: event.target.value }))
                    }
                  />
                </Field>
                <Field label="API-ключ">
                  <Input
                    required
                    type="password"
                    name="provider-api-key"
                    autoComplete="new-password"
                    data-1p-ignore="true"
                    data-bwignore="true"
                    data-lpignore="true"
                    spellCheck={false}
                    value={form.apiKey}
                    placeholder="sk-…"
                    onChange={(event) =>
                      setForm((current) => ({ ...current, apiKey: event.target.value }))
                    }
                  />
                </Field>
              </div>
              <div className="mt-5 divide-y divide-[var(--border-subtle)] rounded-[var(--radius-md)] border border-[var(--border-subtle)]">
                <ToggleRow
                  label="Сделать провайдером по умолчанию"
                  body="Новые сессии будут выбирать это подключение, пока автор не укажет другое."
                  checked={form.isDefault}
                  onCheckedChange={(isDefault) => setForm((current) => ({ ...current, isDefault }))}
                />
                <ToggleRow
                  label="Разрешить приватный endpoint"
                  body="Сработает только если оператор сервера явно разрешил локальные сети. DNS и redirects всё равно проверяются."
                  checked={form.allowPrivateEndpoint}
                  onCheckedChange={(allowPrivateEndpoint) =>
                    setForm((current) => ({ ...current, allowPrivateEndpoint }))
                  }
                />
              </div>
              {create.isError ? (
                <Banner
                  className="mt-4"
                  variant="danger"
                  title="Проверка не пройдена"
                  body={create.error.message}
                />
              ) : null}
              <div className="mt-5 flex justify-end gap-2">
                <Button type="button" variant="ghost" onClick={() => setShowForm(false)}>
                  Отмена
                </Button>
                <Button type="submit" disabled={create.isPending}>
                  <ShieldCheck className="size-4" aria-hidden="true" />
                  {create.isPending ? "Проверяем 3 шага…" : "Проверить и сохранить"}
                </Button>
              </div>
            </form>
          ) : null}

          <section className="mt-6 space-y-3">
            <h2 className="text-[10px] font-semibold tracking-[0.14em] text-[var(--ink-tertiary)] uppercase">
              Подключения автора
            </h2>
            {connections.length === 0 ? (
              <Banner
                variant="info"
                title="Нет собственных провайдеров"
                body="Системная модель доступна по умолчанию. Добавьте своё OpenAI-compatible подключение при необходимости."
              />
            ) : (
              connections.map((connection) => {
                const compatible =
                  connection.status === "compatible" || connection.status === "limited";
                return (
                  <article
                    key={connection.id}
                    className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-5"
                  >
                    <div className="flex flex-wrap items-center gap-3">
                      <span
                        className={`grid size-10 place-items-center rounded ${
                          compatible
                            ? "bg-[var(--success-soft)] text-[var(--success)]"
                            : "bg-[var(--warning-soft)] text-[var(--warning)]"
                        }`}
                      >
                        {compatible ? (
                          <CheckCircle2 className="size-5" aria-hidden="true" />
                        ) : (
                          <TriangleAlert className="size-5" aria-hidden="true" />
                        )}
                      </span>
                      <div className="min-w-0 flex-1">
                        <p className="font-semibold">
                          {connection.name}
                          {connection.isDefault ? (
                            <span className="ml-2 rounded-full bg-[var(--accent-soft)] px-2 py-1 text-[9px] text-[var(--accent-ink)]">
                              по умолчанию
                            </span>
                          ) : null}
                        </p>
                        <p className="font-mono mt-1 truncate text-[10px] text-[var(--ink-tertiary)]">
                          {connection.baseUrl} · {connection.model} · key ••••
                          {connection.apiKeyLast4}
                        </p>
                        <p className="mt-2 text-[12px] text-[var(--ink-secondary)]">
                          {connection.statusDetail}
                        </p>
                      </div>
                      <div className="flex gap-1">
                        {!connection.isDefault && compatible ? (
                          <Button
                            variant="ghost"
                            size="compact"
                            onClick={() => makeDefault.mutate(connection.id)}
                          >
                            По умолчанию
                          </Button>
                        ) : null}
                        <Button
                          variant="ghost"
                          size="icon"
                          aria-label="Повторить проверку"
                          onClick={() => probe.mutate(connection.id)}
                        >
                          <RefreshCw className="size-4" aria-hidden="true" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          aria-label="Отключить провайдера"
                          onClick={() => disable.mutate(connection.id)}
                        >
                          <Trash2 className="size-4" aria-hidden="true" />
                        </Button>
                      </div>
                    </div>
                  </article>
                );
              })
            )}
          </section>
        </main>
      </div>
    </RequireSession>
  );
}

function ToggleRow({
  label,
  body,
  checked,
  onCheckedChange,
}: {
  label: string;
  body: string;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  const controlId = useId();

  return (
    <label className="flex items-center gap-4 p-4" htmlFor={controlId}>
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-medium">{label}</span>
        <span className="mt-1 block text-[11px] leading-5 text-[var(--ink-secondary)]">{body}</span>
      </span>
      <Switch id={controlId} checked={checked} onCheckedChange={onCheckedChange} />
    </label>
  );
}
