import {
  cloneElement,
  forwardRef,
  type InputHTMLAttributes,
  type ReactElement,
  type SelectHTMLAttributes,
  useId,
} from "react";

import { cn } from "~shared/lib/cn";

interface FieldProps {
  label: string;
  hint?: string;
  error?: string;
  className?: string;
  htmlFor?: string;
  children: ReactElement<{ id?: string; "aria-describedby"?: string }>;
}

export function Field({ label, hint, error, className, htmlFor, children }: FieldProps) {
  const generatedId = useId();
  const id = htmlFor ?? children.props.id ?? generatedId;
  const descriptionId = `${id}-description`;
  return (
    <div className={cn("grid content-start gap-2 text-sm text-[var(--ink-secondary)]", className)}>
      <label htmlFor={id} className="font-medium text-[var(--ink-primary)]">
        {label}
      </label>
      {htmlFor
        ? children
        : cloneElement(children, {
            id,
            "aria-describedby": error || hint ? descriptionId : undefined,
          })}
      {error ? (
        <span id={descriptionId} className="text-[13px] text-[var(--danger)]">
          {error}
        </span>
      ) : hint ? (
        <span id={descriptionId} className="text-[13px] text-[var(--ink-tertiary)]">
          {hint}
        </span>
      ) : null}
    </div>
  );
}

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(
  ({ className, ...props }, ref) => (
    <input
      ref={ref}
      className={cn(
        "h-11 w-full rounded-[var(--radius-sm)] border border-[var(--border-strong)] bg-[var(--surface-raised)] px-3 text-[15px] text-[var(--ink-primary)] placeholder:text-[var(--ink-tertiary)] focus:border-[var(--focus)] focus:outline-2 focus:outline-offset-1 focus:outline-[var(--focus)]",
        className,
      )}
      {...props}
    />
  ),
);
Input.displayName = "Input";

export const Select = forwardRef<HTMLSelectElement, SelectHTMLAttributes<HTMLSelectElement>>(
  ({ className, ...props }, ref) => (
    <select
      ref={ref}
      className={cn(
        "h-11 w-full rounded-[var(--radius-sm)] border border-[var(--border-strong)] bg-[var(--surface-raised)] px-3 text-[15px] text-[var(--ink-primary)] focus:border-[var(--focus)] focus:outline-2 focus:outline-offset-1 focus:outline-[var(--focus)]",
        className,
      )}
      {...props}
    />
  ),
);
Select.displayName = "Select";
