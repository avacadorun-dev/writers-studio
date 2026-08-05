import * as ToastPrimitive from "@radix-ui/react-toast";
import { createContext, useCallback, useContext, useMemo, useState } from "react";

interface ToastMessage {
  id: number;
  title: string;
  description?: string;
  actionLabel?: string;
  onAction?: () => void;
}

interface ToastInput {
  title: string;
  description?: string;
  actionLabel?: string;
  onAction?: () => void;
}

interface ToastContextValue {
  showToast: (toast: ToastInput) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<ToastMessage[]>([]);
  const showToast = useCallback((toast: ToastInput) => {
    setToasts((current) => [...current, { ...toast, id: Date.now() + current.length }]);
  }, []);
  const value = useMemo(() => ({ showToast }), [showToast]);

  return (
    <ToastContext.Provider value={value}>
      <ToastPrimitive.Provider swipeDirection="right">
        {children}
        {toasts.map((toast) => (
          <ToastPrimitive.Root
            key={toast.id}
            defaultOpen
            duration={5_000}
            onOpenChange={(open) => {
              if (!open) {
                setToasts((current) => current.filter((item) => item.id !== toast.id));
              }
            }}
            className="grid w-[min(420px,calc(100vw-32px))] grid-cols-[1fr_auto] gap-x-4 rounded-[var(--radius-md)] border border-[var(--border-strong)] bg-[var(--surface-inverse)] px-4 py-3 text-[var(--ink-on-inverse)] shadow-[0_10px_24px_var(--shadow-color)]"
          >
            <ToastPrimitive.Title className="text-sm font-medium">
              {toast.title}
            </ToastPrimitive.Title>
            {toast.description ? (
              <ToastPrimitive.Description className="mt-1 text-[13px] text-[var(--ink-on-inverse-muted)]">
                {toast.description}
              </ToastPrimitive.Description>
            ) : null}
            {toast.actionLabel ? (
              <ToastPrimitive.Action
                altText={toast.actionLabel}
                onClick={toast.onAction}
                className="row-span-2 min-h-9 rounded-[var(--radius-sm)] px-2 text-sm font-medium text-[var(--focus-on-inverse)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-on-inverse)]"
              >
                {toast.actionLabel}
              </ToastPrimitive.Action>
            ) : null}
          </ToastPrimitive.Root>
        ))}
        <ToastPrimitive.Viewport className="fixed right-4 bottom-4 z-[100] grid gap-2 outline-none" />
      </ToastPrimitive.Provider>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastContextValue {
  const value = useContext(ToastContext);
  if (!value) {
    throw new Error("useToast must be used inside ToastProvider");
  }
  return value;
}
