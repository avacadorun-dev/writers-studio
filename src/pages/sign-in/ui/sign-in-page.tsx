import { useNavigate } from "@tanstack/react-router";
import { ArrowRight, LockKeyhole, Mail, ShieldCheck } from "lucide-react";
import { type FormEvent, useState } from "react";

import { useLogin, useRegister } from "~features/auth";
import { APIError } from "~shared/api";
import { BrandMark, Button, Field, Input } from "~shared/ui";

import "./sign-in.css";

export function SignInPage() {
  const navigate = useNavigate();
  const loginMutation = useLogin();
  const registerMutation = useRegister();
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
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
    <main className="auth-screen grid min-h-full bg-[var(--surface-paper)] text-[var(--ink-primary)] lg:grid-cols-[minmax(0,1.1fr)_minmax(440px,1fr)]">
      <section className="auth-hero relative hidden min-h-full overflow-hidden px-12 py-10 text-[var(--ink-on-inverse)] lg:flex lg:flex-col xl:px-16 xl:py-12">
        <div className="flex items-center gap-3">
          <BrandMark className="size-10 shrink-0" aria-hidden="true" />
          <span className="font-display text-[19px] leading-6">Студия автора</span>
        </div>
        <div className="auth-manuscript-scene" aria-hidden="true">
          <div className="auth-manuscript-sheet auth-manuscript-sheet-back" />
          <div className="auth-manuscript-sheet auth-manuscript-sheet-middle" />
          <div className="auth-manuscript-sheet auth-manuscript-sheet-front">
            <span className="auth-manuscript-chapter">Глава первая</span>
            <span className="auth-manuscript-title">Начало истории</span>
            <span className="auth-manuscript-rule" />
            <div className="auth-manuscript-lines">
              <span />
              <span />
              <span />
              <span />
            </div>
            <div className="auth-manuscript-lines auth-manuscript-lines-short">
              <span />
              <span />
            </div>
            <span className="auth-manuscript-page">01</span>
          </div>
        </div>
        <div className="relative mt-auto max-w-[520px]">
          <blockquote className="font-display text-[34px] leading-[1.3] xl:text-[40px]">
            Место, где мысль становится текстом — и остаётся вашей.
          </blockquote>
          <p className="mt-5 max-w-[400px] text-[13px] leading-[1.8] text-[var(--ink-on-inverse-muted)]">
            От первого наброска до последней правки.
            <br />
            Ваши истории, заметки и идеи в одном месте.
          </p>
        </div>
      </section>

      <section className="auth-form-panel flex min-h-full items-center justify-center px-6 py-10 sm:p-12">
        <form onSubmit={handleSubmit} className="relative w-full max-w-[400px]">
          <div className="mb-12 flex items-center gap-3 lg:hidden">
            <BrandMark className="size-10 shrink-0" aria-hidden="true" />
            <span className="font-display text-[19px]">Студия автора</span>
          </div>
          <p className="text-[10px] font-semibold tracking-[0.18em] text-[var(--accent)] uppercase">
            Добро пожаловать
          </p>
          <h1 className="font-display mt-4 text-[32px] leading-[1.25] sm:text-[36px]">
            {mode === "login" ? "Вернуться к рукописи" : "Создать пространство"}
          </h1>
          <p className="mt-4 max-w-[390px] text-[13px] leading-[1.8] text-[var(--ink-secondary)]">
            {mode === "login"
              ? "Войдите, чтобы открыть проекты и синхронизировать сохранённую работу."
              : "Начните с почты и пароля. Ваши неопубликованные тексты остаются приватными."}
          </p>

          <div className="mt-8 grid gap-5">
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

          <Button className="mt-6 w-full" disabled={mutation.isPending} type="submit">
            {mutation.isPending ? "Проверяем…" : mode === "login" ? "Войти" : "Создать аккаунт"}
            <ArrowRight className="size-4" aria-hidden="true" />
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

          <p className="mt-8 flex items-start gap-2 border-t border-[var(--border-subtle)] pt-5 text-[11px] leading-[1.7] text-[var(--ink-tertiary)]">
            <ShieldCheck className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
            Неопубликованные тексты считаются чувствительными данными и не используются для
            обучения.
          </p>
        </form>
      </section>
    </main>
  );
}
