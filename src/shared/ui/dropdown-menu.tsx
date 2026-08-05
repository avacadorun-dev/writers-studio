import * as DropdownMenuPrimitive from "@radix-ui/react-dropdown-menu";
import type { ComponentPropsWithoutRef } from "react";

import { cn } from "~shared/lib/cn";

export const DropdownMenu = DropdownMenuPrimitive.Root;
export const DropdownMenuTrigger = DropdownMenuPrimitive.Trigger;

export function DropdownMenuContent({
  className,
  sideOffset = 4,
  ...props
}: ComponentPropsWithoutRef<typeof DropdownMenuPrimitive.Content>) {
  return (
    <DropdownMenuPrimitive.Portal>
      <DropdownMenuPrimitive.Content
        sideOffset={sideOffset}
        className={cn(
          "z-[75] grid min-w-52 rounded-[var(--radius-md)] border border-[var(--border-strong)] bg-[var(--surface-raised)] p-1 shadow-[0_20px_40px_var(--shadow-color)] outline-none data-[state=closed]:animate-out data-[state=open]:animate-in",
          className,
        )}
        {...props}
      />
    </DropdownMenuPrimitive.Portal>
  );
}

export function DropdownMenuItem({
  className,
  destructive = false,
  ...props
}: ComponentPropsWithoutRef<typeof DropdownMenuPrimitive.Item> & {
  destructive?: boolean;
}) {
  return (
    <DropdownMenuPrimitive.Item
      className={cn(
        "flex min-h-10 cursor-default select-none items-center gap-2 rounded-[var(--radius-sm)] px-3 text-left text-sm outline-none data-[disabled]:pointer-events-none data-[disabled]:opacity-50",
        destructive
          ? "text-[var(--danger)] data-[highlighted]:bg-[var(--danger-soft)]"
          : "text-[var(--ink-primary)] data-[highlighted]:bg-[var(--surface-subtle)]",
        className,
      )}
      {...props}
    />
  );
}
