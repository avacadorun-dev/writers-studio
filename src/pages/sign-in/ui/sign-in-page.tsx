import { useNavigate } from "@tanstack/react-router";
import { ArrowRight, LockKeyhole, Mail, ShieldCheck } from "lucide-react";
import { type FormEvent, useState } from "react";

import { useLogin, useRegister } from "~features/auth";
import { APIError } from "~shared/api";
import { BrandMark, Button, Field, Input } from "~shared/ui";

export function SignInPage() {
  const navigate = useNavigate();
  const loginMutation = useLogin();
  const registerMutation = useRegister();
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("anna@example.com");
  const [password, setPassword] = useState("writers-studio");
  const mutation = mode === "login" ? loginMutation : registerMutation;

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    mutation.mutate(
      { email, password },
      {
        onSuccess: () => {
          void navigate({ to: "/projects", replace: true });
        },
      },
    );
  };

  const errorMessage =
    mutation.error instanceof APIError ? mutation.error.message : mutation.error?.message;

  return (
    <main className="auth-screen grid min-h-full bg-[var(--surface-paper)] text-[var(--ink-primary)] lg:grid-cols-[minmax(0,840px)_minmax(480px,600px)]">
      <section className="relative hidden min-h-full overflow-hidden bg-[var(--surface-inverse)] px-[88px] py-[72px] text-[var(--ink-on-inverse)] lg:flex lg:flex-col">
        <div className="flex items-center gap-3">
          <BrandMark className="size-10 shrink-0" aria-hidden="true" />
          <span className="font-display text-[19px] leading-6">Студия автора</span>
        </div>
        <div className="mt-auto max-w-[520px]">
          <blockquote className="font-display text-[42px] leading-[1.17]">
            Место, где мысль становится текстом — и остаётся вашей.
          </blockquote>
          <p className="mt-6 max-w-[440px] text-[13px] leading-[1.7] text-[var(--ink-on-solid-muted)]">
            Голос, рукопись и каждая версия сохраняются отдельно. Никаких скрытых изменений.
          </p>
        </div>
      </section>

      <section className="flex min-h-full items-center justify-center bg-[var(--surface-paper)] p-6 sm:p-10">
        <form onSubmit={handleSubmit} className="min-h-[520px] w-full max-w-[424px]">
          <div className="mb-8 flex items-center gap-3 lg:hidden">
            <BrandMark className="size-10 shrink-0" aria-hidden="true" />
            <span className="font-display text-[19px]">Студия автора</span>
          </div>
          <p className="text-[10px] font-semibold tracking-[0.18em] text-[var(--ink-tertiary)] uppercase">
            Добро пожаловать
          </p>
          <h1 className="font-display mt-5 text-[36px] leading-[1.2]">
            {mode === "login" ? "Вернуться к рукописи" : "Создать пространство"}
          </h1>
          <p className="mt-5 max-w-[390px] text-[13px] leading-[1.55] text-[var(--ink-secondary)]">
            {mode === "login"
              ? "Войдите, чтобы открыть проекты и синхронизировать сохранённую работу."
              : "Начните с почты и пароля. Ваши неопубликованные тексты остаются приватными."}
          </p>

          <div className="mt-6 grid gap-5">
            <Field label="Почта" htmlFor="auth-email">
              <div className="relative">
                <Mail
                  className="pointer-events-none absolute top-1/2 left-3 size-[18px] -translate-y-1/2 text-[var(--ink-tertiary)]"
                  aria-hidden="true"
                />
                <Input
                  id="auth-email"
                  className="pl-10"
                  type="email"
                  autoComplete="email"
                  required
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                />
              </div>
            </Field>
            <Field label="Пароль" error={errorMessage} htmlFor="auth-password">
              <div className="relative">
                <LockKeyhole
                  className="pointer-events-none absolute top-1/2 left-3 size-[18px] -translate-y-1/2 text-[var(--ink-tertiary)]"
                  aria-hidden="true"
                />
                <Input
                  id="auth-password"
                  aria-describedby={errorMessage ? "auth-password-description" : undefined}
                  className="pl-10"
                  type="password"
                  autoComplete={mode === "login" ? "current-password" : "new-password"}
                  required
                  minLength={8}
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                />
              </div>
            </Field>
          </div>

          <Button className="mt-5 w-full" disabled={mutation.isPending} type="submit">
            <ArrowRight className="size-[18px]" aria-hidden="true" />
            {mutation.isPending ? "Проверяем…" : mode === "login" ? "Войти" : "Создать аккаунт"}
          </Button>
          <Button
            className="mt-2 w-full"
            type="button"
            variant="ghost"
            onClick={() => {
              mutation.reset();
              setMode((current) => (current === "login" ? "register" : "login"));
            }}
          >
            {mode === "login" ? "Создать аккаунт" : "У меня уже есть аккаунт"}
          </Button>

          <p className="mt-7 flex items-start gap-2 text-[11px] leading-[1.55] text-[var(--ink-tertiary)]">
            <ShieldCheck className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
            Неопубликованные тексты считаются чувствительными данными и не используются для
            обучения.
          </p>
        </form>
      </section>
    </main>
  );
}
