import { CircleAlert, CircleCheck, WifiOff } from "lucide-react";

import { cn } from "~shared/lib/cn";

type BannerVariant = "warning" | "success" | "danger" | "info";

const icons = {
  warning: WifiOff,
  success: CircleCheck,
  danger: CircleAlert,
  info: CircleAlert,
};

const styles = {
  warning: "border-[var(--warning)]/30 bg-[var(--warning-soft)] text-[var(--warning)]",
  success: "border-[var(--success)]/30 bg-[var(--success-soft)] text-[var(--success)]",
  danger: "border-[var(--danger)]/30 bg-[var(--danger-soft)] text-[var(--danger)]",
  info: "border-[var(--info)]/30 bg-[var(--info-soft)] text-[var(--info)]",
};

interface BannerProps {
  title: string;
  body: string;
  variant?: BannerVariant;
  action?: React.ReactNode;
  className?: string;
}

export function Banner({ title, body, variant = "info", action, className }: BannerProps) {
  const Icon = icons[variant];
  return (
    <section
      className={cn(
        "flex items-start gap-3 rounded-[var(--radius-md)] border px-4 py-3",
        styles[variant],
        className,
      )}
      role={variant === "danger" ? "alert" : "status"}
    >
      <Icon className="mt-0.5 size-5 shrink-0" aria-hidden="true" />
      <div className="min-w-0 flex-1">
        <h2 className="text-sm font-semibold">{title}</h2>
        <p className="mt-0.5 text-[13px] text-[var(--ink-secondary)]">{body}</p>
      </div>
      {action}
    </section>
  );
}
