import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { type ButtonHTMLAttributes, forwardRef } from "react";

import { cn } from "~shared/lib/cn";

import { UiTooltip } from "./tooltip";

const buttonVariants = cva(
  "studio-button inline-flex min-h-11 items-center justify-center gap-2 rounded-[var(--radius-sm)] px-4 text-sm font-medium focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus)] disabled:cursor-not-allowed disabled:opacity-50",
  {
    variants: {
      variant: {
        primary:
          "studio-button-primary bg-[var(--accent-surface)] text-[var(--ink-on-solid)] hover:bg-[var(--accent-surface-hover)] active:bg-[var(--accent-surface-hover)]",
        creative:
          "bg-[var(--creative-surface)] text-[var(--ink-on-creative)] hover:bg-[var(--creative-surface-hover)] active:bg-[var(--creative-surface-hover)]",
        secondary:
          "border border-[var(--border-subtle)] bg-[var(--surface-raised)] text-[var(--ink-primary)] hover:border-[var(--border-strong)] hover:bg-[var(--surface-subtle)]",
        ghost:
          "bg-transparent text-[var(--ink-secondary)] hover:bg-[var(--surface-subtle)] hover:text-[var(--ink-primary)]",
        destructive:
          "bg-[var(--danger-surface)] text-[var(--ink-on-solid)] hover:bg-[var(--record-surface)]",
        record:
          "bg-[var(--record-surface)] text-[var(--ink-on-solid)] hover:bg-[var(--danger-surface)]",
      },
      size: {
        default: "h-11 px-4",
        compact: "h-9 min-h-9 px-3 text-[13px]",
        icon: "size-11 p-0",
      },
    },
    defaultVariants: {
      variant: "primary",
      size: "default",
    },
  },
);

export interface ButtonProps
  extends ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
  tooltip?: string | false;
  tooltipSide?: "top" | "right" | "bottom" | "left";
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  (
    { asChild = false, className, variant, size, title, tooltip, tooltipSide = "bottom", ...props },
    ref,
  ) => {
    const Component = asChild ? Slot : "button";
    const accessibleLabel = props["aria-label"];
    const tooltipLabel =
      tooltip === false
        ? undefined
        : (tooltip ??
          (size === "icon"
            ? typeof title === "string"
              ? title
              : typeof accessibleLabel === "string"
                ? accessibleLabel
                : undefined
            : undefined));
    const control = (
      <Component
        ref={ref}
        className={cn(buttonVariants({ variant, size }), className)}
        data-ui-tooltip={tooltipLabel}
        data-ui-tooltip-side={tooltipLabel ? tooltipSide : undefined}
        title={tooltipLabel ? undefined : title}
        {...props}
      />
    );
    return tooltipLabel ? (
      <UiTooltip content={tooltipLabel} side={tooltipSide}>
        {control}
      </UiTooltip>
    ) : (
      control
    );
  },
);
Button.displayName = "Button";

export { buttonVariants };
