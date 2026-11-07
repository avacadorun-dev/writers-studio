import { Link } from "@tanstack/react-router";
import { ArrowLeft } from "lucide-react";

import { Button } from "~shared/ui";
import { RequireSession } from "~widgets/require-session";

export function PageShell({
  title,
  eyebrow,
  backTo = "/projects",
  children,
  actions,
}: {
  title: string;
  eyebrow?: string;
  backTo?: string;
  children: React.ReactNode;
  actions?: React.ReactNode;
}) {
  return (
    <RequireSession>
      <div className="min-h-full">
        <header className="workspace-chrome border-b border-[var(--border-subtle)]">
          <div className="mx-auto flex min-h-20 max-w-[1120px] items-center gap-3 px-4 lg:px-8">
            <Button asChild variant="ghost" size="icon" aria-label="Назад">
              <Link to={backTo}>
                <ArrowLeft className="size-4" aria-hidden="true" />
              </Link>
            </Button>
            <div className="min-w-0">
              {eyebrow ? (
                <p className="text-[10px] font-semibold tracking-[0.14em] text-[var(--ink-tertiary)] uppercase">
                  {eyebrow}
                </p>
              ) : null}
              <h1 className="font-display truncate text-2xl">{title}</h1>
            </div>
            <div className="ml-auto">{actions}</div>
          </div>
        </header>
        <main className="mx-auto max-w-[1120px] px-4 py-7 lg:px-8">{children}</main>
      </div>
    </RequireSession>
  );
}
