import * as TooltipPrimitive from "@radix-ui/react-tooltip";
import type { ReactElement } from "react";

export function UiTooltip({
  children,
  content,
  side = "bottom",
}: {
  children: ReactElement;
  content: string;
  side?: "top" | "right" | "bottom" | "left";
}) {
  return (
    <TooltipPrimitive.Provider delayDuration={400} skipDelayDuration={150}>
      <TooltipPrimitive.Root>
        <TooltipPrimitive.Trigger asChild>{children}</TooltipPrimitive.Trigger>
        <TooltipPrimitive.Portal>
          <TooltipPrimitive.Content
            side={side}
            sideOffset={8}
            collisionPadding={12}
            className="pointer-events-none z-[110] max-w-[min(260px,calc(100vw-24px))] rounded-[7px] border border-[var(--border-strong)] bg-[var(--surface-inverse)] px-[9px] py-1.5 text-left text-[11px] leading-[1.35] font-medium text-[var(--ink-on-inverse)] shadow-[0_6px_20px_var(--shadow-color)]"
          >
            {content}
            <TooltipPrimitive.Arrow className="fill-[var(--surface-inverse)]" />
          </TooltipPrimitive.Content>
        </TooltipPrimitive.Portal>
      </TooltipPrimitive.Root>
    </TooltipPrimitive.Provider>
  );
}
