import { useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";

import { useSession } from "~entities/session";
import { Spinner } from "~shared/ui";

export function RequireSession({ children }: { children: React.ReactNode }) {
  const session = useSession();
  const navigate = useNavigate();

  useEffect(() => {
    if (session.isError) {
      void navigate({ to: "/login", replace: true });
    }
  }, [navigate, session.isError]);

  if (session.isPending || session.isError) {
    return (
      <main className="min-h-full">
        <Spinner label={session.isError ? "Переходим ко входу…" : "Открываем студию…"} />
      </main>
    );
  }
  return children;
}
