import * as SwitchPrimitive from "@radix-ui/react-switch";
import { forwardRef } from "react";

import { cn } from "~shared/lib/cn";

export const Switch = forwardRef<
  React.ComponentRef<typeof SwitchPrimitive.Root>,
  React.ComponentPropsWithoutRef<typeof SwitchPrimitive.Root>
>(({ className, ...props }, ref) => (
  <SwitchPrimitive.Root
    ref={ref}
    className={cn(
      "relative h-[26px] w-11 shrink-0 rounded-full border border-[var(--border-strong)] bg-[var(--surface-subtle)] transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus)] data-[state=checked]:border-[var(--accent)] data-[state=checked]:bg-[var(--accent)] disabled:cursor-not-allowed disabled:opacity-50",
      className,
    )}
    {...props}
  >
    <SwitchPrimitive.Thumb className="block size-5 translate-x-0.5 rounded-full border border-[var(--border-subtle)] bg-[var(--control-thumb)] shadow-[0_1px_2px_var(--shadow-color)] transition-transform data-[state=checked]:translate-x-[21px]" />
  </SwitchPrimitive.Root>
));
Switch.displayName = "Switch";
