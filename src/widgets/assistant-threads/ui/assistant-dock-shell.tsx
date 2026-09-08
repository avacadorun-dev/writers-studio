import type { ReactNode, RefObject } from "react";
import { useEffect } from "react";

import { cn } from "~shared/lib";

interface AssistantDockShellProps {
  children: ReactNode;
  className?: string;
  testId?: string;
  returnFocusRef?: RefObject<HTMLElement | null>;
}

export function AssistantDockShell({
  children,
  className,
  testId = "assistant-dock",
  returnFocusRef,
}: AssistantDockShellProps) {
  useEffect(
    () => () => {
      window.requestAnimationFrame(() => returnFocusRef?.current?.focus());
    },
    [returnFocusRef],
  );

  return (
    <aside
      data-testid={testId}
      className={cn(
        "fixed inset-x-0 top-14 bottom-0 z-40 min-h-0 min-w-0 overscroll-contain overflow-hidden border-l border-[var(--border-subtle)] bg-[var(--surface-panel)] md:left-auto md:right-0 md:w-[380px] md:shadow-[-18px_0_40px_var(--shadow-color)] xl:static xl:z-auto xl:w-[380px] xl:shrink-0 xl:shadow-none",
        className,
      )}
      onWheel={(event) => event.stopPropagation()}
      onTouchMove={(event) => event.stopPropagation()}
    >
      {children}
    </aside>
  );
}
