import * as Tooltip from "@radix-ui/react-tooltip";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import {
  applyWriterPreferences,
  loadWriterPreferences,
  writerPreferencesChangedEvent,
  writerPreferencesStorageKey,
} from "~shared/lib";
import { ActionDialogProvider, ToastProvider } from "~shared/ui";

export function AppProviders({ children }: { children: React.ReactNode }) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            staleTime: 15_000,
            retry: (count, error) =>
              count < 2 && !(error instanceof Error && error.message.includes("Сессия")),
          },
          mutations: {
            retry: false,
          },
        },
      }),
  );
  useEffect(() => {
    const media = window.matchMedia?.("(prefers-color-scheme: dark)");
    const apply = () => applyWriterPreferences(loadWriterPreferences());
    const handleStorage = (event: StorageEvent) => {
      if (event.key === writerPreferencesStorageKey) {
        apply();
      }
    };
    apply();
    window.addEventListener(writerPreferencesChangedEvent, apply);
    window.addEventListener("storage", handleStorage);
    media?.addEventListener("change", apply);
    return () => {
      window.removeEventListener(writerPreferencesChangedEvent, apply);
      window.removeEventListener("storage", handleStorage);
      media?.removeEventListener("change", apply);
    };
  }, []);
  return (
    <QueryClientProvider client={queryClient}>
      <Tooltip.Provider delayDuration={400}>
        <ToastProvider>
          <ActionDialogProvider>{children}</ActionDialogProvider>
        </ToastProvider>
      </Tooltip.Provider>
    </QueryClientProvider>
  );
}
