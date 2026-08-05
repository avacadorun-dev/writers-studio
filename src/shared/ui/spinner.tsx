import { LoaderCircle } from "lucide-react";

export function Spinner({ label = "Загрузка" }: { label?: string }) {
  return (
    <div
      className="flex min-h-40 items-center justify-center gap-3 text-sm text-[var(--ink-secondary)]"
      role="status"
    >
      <LoaderCircle className="size-5 animate-spin" aria-hidden="true" />
      <span>{label}</span>
    </div>
  );
}
