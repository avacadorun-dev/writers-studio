import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowLeft, Clipboard, KeyRound, Network, RefreshCw, Trash2 } from "lucide-react";
import { type FormEvent, useState } from "react";

import { listProjects, type Project } from "~entities/project";
import { apiRequest } from "~shared/api";
import { Banner, Button, Checkbox, Field, Input, Select, Spinner, useToast } from "~shared/ui";
import { RequireSession } from "~widgets/require-session";

type MCPScope =
  | "projects:list"
  | "projects:read"
  | "projects:stage"
  | "projects:apply"
  | "projects:actions";

interface MCPGrant {
  id: string;
  name: string;
  clientName: string;
  authKind: "manual" | "oauth";
  scopes: MCPScope[];
  projectIds: string[];
  expiresAt?: string;
  lastUsedAt?: string;
  revokedAt?: string;
  createdAt: string;
}

interface MCPGrantsResponse {
  enabled: boolean;
  endpoint: string;
  grants: MCPGrant[] | null;
}

interface MCPSecretResponse {
  grant: MCPGrant;
  token: string;
  endpoint: string;
  config?: { url: string; headers: { Authorization: string } };
}

const scopeOptions: Array<{ scope: MCPScope; label: string; body: string }> = [
  { scope: "projects:list", label: "Список проектов", body: "Видит только выбранные проекты." },
  {
    scope: "projects:read",
    label: "Чтение",
    body: "Читает структуру, документы, версии и контекст.",
  },
  {
    scope: "projects:stage",
    label: "Подготовка изменений",
    body: "Создаёт diff без изменения проекта.",
  },
  {
    scope: "projects:apply",
    label: "Apply, Reject и Undo",
    body: "Может завершить цикл прямо из агента.",
  },
  {
    scope: "projects:actions",
    label: "Действия",
    body: "Экспорт и повторная расшифровка после preview.",
  },
];

export function MCPSettingsPage() {
  const { showToast } = useToast();
  const queryClient = useQueryClient();
  const grants = useQuery({
    queryKey: ["mcp-grants"],
    queryFn: () => apiRequest<MCPGrantsResponse>("/api/mcp/grants"),
  });
  const projects = useQuery({
    queryKey: ["mcp-project-options"],
    queryFn: async () => {
      const [active, archived] = await Promise.all([
        listProjects("active"),
        listProjects("archived"),
      ]);
      return [...active.projects, ...archived.projects];
    },
  });
  const [name, setName] = useState("Мой локальный агент");
  const [clientName, setClientName] = useState("Local MCP agent");
  const [expiresDays, setExpiresDays] = useState(90);
  const [selectedProjects, setSelectedProjects] = useState<string[]>([]);
  const [scopes, setScopes] = useState<MCPScope[]>(scopeOptions.map((item) => item.scope));
  const [secret, setSecret] = useState<MCPSecretResponse>();

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ["mcp-grants"] });
  const create = useMutation({
    mutationFn: () =>
      apiRequest<MCPSecretResponse>("/api/mcp/grants", {
        method: "POST",
        body: JSON.stringify({
          name,
          clientName,
          expiresDays,
          projectIds: selectedProjects,
          scopes,
        }),
      }),
    onSuccess: async (result) => {
      setSecret(result);
      await invalidate();
    },
    onError: (error) => showToast({ title: "Доступ не создан", description: error.message }),
  });
  const revoke = useMutation({
    mutationFn: (grantId: string) =>
      apiRequest<void>(`/api/mcp/grants/${grantId}`, { method: "DELETE" }),
    onSuccess: invalidate,
  });
  const rotate = useMutation({
    mutationFn: (grantId: string) =>
      apiRequest<MCPSecretResponse>(`/api/mcp/grants/${grantId}/rotate`, {
        method: "POST",
        body: JSON.stringify({ expiresDays }),
      }),
    onSuccess: async (result) => {
      setSecret(result);
      await invalidate();
    },
  });

  if (grants.isPending || projects.isPending) return <Spinner label="Загружаем MCP-доступы…" />;
  if (!grants.data || projects.isError) {
    return (
      <Banner
        variant="danger"
        title="MCP недоступен"
        body="Обновите страницу или проверьте конфигурацию сервера."
      />
    );
  }

  const copy = async (value: string, title: string) => {
    await navigator.clipboard.writeText(value);
    showToast({ title, description: "Скопировано в буфер обмена." });
  };
  const submit = (event: FormEvent) => {
    event.preventDefault();
    create.mutate();
  };
  const issuedGrants = grants.data.grants ?? [];

  return (
    <RequireSession>
      <div className="min-h-full bg-[var(--surface-app)]">
        <header className="flex h-14 items-center gap-3 border-b border-[var(--border-subtle)] bg-[var(--surface-panel)] px-4">
          <Button asChild variant="ghost" size="icon" aria-label="Назад в настройки">
            <Link to="/settings">
              <ArrowLeft className="size-4" aria-hidden="true" />
            </Link>
          </Button>
          <Network className="size-5 text-[var(--accent)]" aria-hidden="true" />
          <p className="text-sm font-semibold">Агенты и MCP</p>
        </header>
        <main className="mx-auto grid w-full max-w-[980px] gap-6 px-4 py-8 sm:px-8">
          <div>
            <h1 className="font-display text-4xl">Удалённый доступ для локальных агентов</h1>
            <p className="mt-2 max-w-3xl text-sm leading-6 text-[var(--ink-secondary)]">
              Агент в локальной сети или VPN может открыть выбранный проект, подготовить diff,
              применить либо отклонить его и выполнить Undo без открытия интерфейса студии.
            </p>
          </div>
          {!grants.data.enabled ? (
            <Banner
              variant="warning"
              title="Remote MCP выключен"
              body="Оператор должен установить MCP_ENABLED=true и перезапустить сервер."
            />
          ) : null}
          <section className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-5 sm:p-6">
            <div className="flex flex-wrap items-center gap-3">
              <div className="min-w-0 flex-1">
                <p className="text-xs font-semibold tracking-wide text-[var(--ink-tertiary)] uppercase">
                  Endpoint
                </p>
                <code className="mt-1 block truncate text-sm">{grants.data.endpoint}</code>
              </div>
              <Button
                type="button"
                variant="secondary"
                onClick={() => copy(grants.data.endpoint, "Endpoint скопирован")}
              >
                <Clipboard className="size-4" aria-hidden="true" />
                Копировать
              </Button>
            </div>
          </section>

          {secret ? (
            <section className="rounded-[var(--radius-lg)] border border-[var(--warning)]/40 bg-[var(--warning-soft)] p-5">
              <h2 className="font-medium">Сохраните токен сейчас</h2>
              <p className="mt-1 text-sm text-[var(--ink-secondary)]">
                После закрытия этого блока сервер больше не покажет секрет.
              </p>
              <code className="mt-4 block overflow-x-auto rounded bg-[var(--surface-raised)] p-3 text-xs">
                {secret.token}
              </code>
              <div className="mt-3 flex flex-wrap gap-2">
                <Button type="button" onClick={() => copy(secret.token, "Токен скопирован")}>
                  <KeyRound className="size-4" aria-hidden="true" />
                  Копировать токен
                </Button>
                <Button
                  type="button"
                  variant="secondary"
                  onClick={() =>
                    copy(
                      JSON.stringify(
                        secret.config ?? {
                          url: secret.endpoint,
                          headers: { Authorization: `Bearer ${secret.token}` },
                        },
                        null,
                        2,
                      ),
                      "Конфигурация скопирована",
                    )
                  }
                >
                  <Clipboard className="size-4" aria-hidden="true" />
                  Копировать config
                </Button>
                <Button type="button" variant="ghost" onClick={() => setSecret(undefined)}>
                  Я сохранил
                </Button>
              </div>
            </section>
          ) : null}

          <form
            onSubmit={submit}
            className="grid gap-5 rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-5 sm:p-6"
          >
            <h2 className="font-medium">Новый ручной токен</h2>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Название доступа">
                <Input
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  required
                  maxLength={120}
                />
              </Field>
              <Field label="Имя клиента">
                <Input
                  value={clientName}
                  onChange={(event) => setClientName(event.target.value)}
                  required
                  maxLength={160}
                />
              </Field>
              <Field label="Срок действия">
                <Select
                  value={expiresDays}
                  onChange={(event) => setExpiresDays(Number(event.target.value))}
                >
                  <option value={7}>7 дней</option>
                  <option value={30}>30 дней</option>
                  <option value={90}>90 дней — по умолчанию</option>
                  <option value={365}>1 год</option>
                </Select>
              </Field>
            </div>
            <ChoiceList title="Проекты">
              {(projects.data ?? []).map((project: Project) => (
                <Choice
                  key={project.id}
                  checked={selectedProjects.includes(project.id)}
                  onChange={(checked) =>
                    setSelectedProjects((current) =>
                      checked
                        ? [...current, project.id]
                        : current.filter((id) => id !== project.id),
                    )
                  }
                  label={project.title}
                  body={project.status === "archived" ? "Архив · только чтение" : "Активный"}
                />
              ))}
            </ChoiceList>
            <ChoiceList title="Права">
              {scopeOptions.map((item) => (
                <Choice
                  key={item.scope}
                  checked={scopes.includes(item.scope)}
                  onChange={(checked) =>
                    setScopes((current) =>
                      checked
                        ? [...current, item.scope]
                        : current.filter((scope) => scope !== item.scope),
                    )
                  }
                  label={item.label}
                  body={item.body}
                />
              ))}
            </ChoiceList>
            <Button
              className="justify-self-start"
              disabled={
                !grants.data.enabled ||
                selectedProjects.length === 0 ||
                scopes.length === 0 ||
                create.isPending
              }
            >
              Создать и показать токен
            </Button>
          </form>

          <section className="grid gap-3">
            <h2 className="font-medium">Выданные доступы</h2>
            {issuedGrants.length === 0 ? (
              <p className="text-sm text-[var(--ink-tertiary)]">Доступов пока нет.</p>
            ) : (
              issuedGrants.map((grant) => (
                <article
                  key={grant.id}
                  className="rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] p-4"
                >
                  <div className="flex flex-wrap items-start gap-3">
                    <div className="min-w-0 flex-1">
                      <h3 className="font-medium">{grant.name}</h3>
                      <p className="mt-1 text-xs text-[var(--ink-tertiary)]">
                        {grant.authKind === "oauth" ? "OAuth" : "Bearer"} · проектов{" "}
                        {grant.projectIds.length} ·{" "}
                        {grant.revokedAt
                          ? "отозван"
                          : `до ${grant.expiresAt ? new Date(grant.expiresAt).toLocaleDateString("ru-RU") : "отзыва"}`}{" "}
                        · использован{" "}
                        {grant.lastUsedAt
                          ? new Date(grant.lastUsedAt).toLocaleString("ru-RU")
                          : "никогда"}
                      </p>
                    </div>
                    {!grant.revokedAt && grant.authKind === "manual" ? (
                      <Button
                        type="button"
                        size="compact"
                        variant="secondary"
                        onClick={() => rotate.mutate(grant.id)}
                        disabled={rotate.isPending}
                      >
                        <RefreshCw className="size-4" aria-hidden="true" />
                        Сменить токен
                      </Button>
                    ) : null}
                    {!grant.revokedAt ? (
                      <Button
                        type="button"
                        size="compact"
                        variant="destructive"
                        onClick={() => revoke.mutate(grant.id)}
                        disabled={revoke.isPending}
                      >
                        <Trash2 className="size-4" aria-hidden="true" />
                        Отозвать
                      </Button>
                    ) : null}
                  </div>
                </article>
              ))
            )}
          </section>
        </main>
      </div>
    </RequireSession>
  );
}

function ChoiceList({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <fieldset className="grid gap-2">
      <legend className="mb-2 text-sm font-medium">{title}</legend>
      <div className="grid gap-2 sm:grid-cols-2">{children}</div>
    </fieldset>
  );
}

function Choice({
  checked,
  onChange,
  label,
  body,
}: {
  checked: boolean;
  onChange: (checked: boolean) => void;
  label: string;
  body: string;
}) {
  return (
    <div className="flex items-start gap-3 rounded-[var(--radius-sm)] border border-[var(--border-subtle)] p-3">
      <Checkbox
        aria-label={label}
        checked={checked}
        onCheckedChange={(value) => onChange(value === true)}
      />
      <span>
        <span className="block text-sm font-medium">{label}</span>
        <span className="block text-xs text-[var(--ink-tertiary)]">{body}</span>
      </span>
    </div>
  );
}
