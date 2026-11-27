import { Check, CircleAlert, CloudOff, LoaderCircle, RefreshCw } from "lucide-react";
import { t } from "~shared/i18n";

import { cn } from "~shared/lib/cn";

export type SaveState = "saved" | "saving" | "local" | "pending" | "conflict";

const stateConfig = {
  saved: { label: t("Сохранено"), Icon: Check, className: "text-[var(--success)]" },
  saving: { label: t("Сохранение…"), Icon: LoaderCircle, className: "text-[var(--info)]" },
  local: { label: t("На этом устройстве"), Icon: CloudOff, className: "text-[var(--warning)]" },
  pending: {
    label: t("Нужна синхронизация"),
    Icon: RefreshCw,
    className: "text-[var(--warning)]",
  },
  conflict: {
    label: t("Сохранены оба варианта"),
    Icon: CircleAlert,
    className: "text-[var(--danger)]",
  },
} satisfies Record<SaveState, { label: string; Icon: typeof Check; className: string }>;

export function SaveStatus({ state }: { state: SaveState }) {
  const { Icon, label, className } = stateConfig[state];
  return (
    <span
      className={cn("inline-flex h-8 items-center gap-1.5 px-2 text-xs font-medium", className)}
      role="status"
      data-save-state={state}
      aria-live={state === "saving" ? "polite" : "off"}
    >
      <Icon className={cn("size-3.5", state === "saving" && "animate-spin")} aria-hidden="true" />
      {label}
    </span>
  );
}
