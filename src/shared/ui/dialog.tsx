import * as AlertDialogPrimitive from "@radix-ui/react-alert-dialog";
import * as DialogPrimitive from "@radix-ui/react-dialog";
import { AlertTriangle, type LucideIcon, X } from "lucide-react";
import type { ComponentPropsWithoutRef, ReactNode } from "react";

import { cn } from "~shared/lib/cn";

import { Button } from "./button";

type DialogTone = "default" | "danger";

interface DialogFrameProps {
  title: string;
  description?: string;
  icon?: LucideIcon;
  tone?: DialogTone;
  children?: ReactNode;
  footer?: ReactNode;
  className?: string;
}

interface ModalDialogProps extends DialogFrameProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function ModalDialog({
  open,
  onOpenChange,
  title,
  description,
  icon,
  tone = "default",
  children,
  footer,
  className,
}: ModalDialogProps) {
  const Icon = icon;
  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="fixed inset-0 z-[80] bg-[var(--surface-overlay)] backdrop-blur-[1px] data-[state=closed]:animate-out data-[state=open]:animate-in" />
        <DialogPrimitive.Content
          className={cn(
            "fixed right-0 bottom-0 left-0 z-[81] max-h-[min(88dvh,760px)] overflow-y-auto rounded-t-[var(--radius-lg)] border border-[var(--border-strong)] bg-[var(--surface-raised)] p-6 shadow-[0_24px_64px_var(--shadow-color)] focus:outline-none sm:top-1/2 sm:right-auto sm:bottom-auto sm:left-1/2 sm:w-[min(520px,calc(100vw-40px))] sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-[var(--radius-lg)]",
            className,
          )}
        >
          <DialogPrimitive.Close asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="absolute top-3 right-3 size-9 min-h-9"
              aria-label="Закрыть диалог"
            >
              <X className="size-4" aria-hidden="true" />
            </Button>
          </DialogPrimitive.Close>
          {Icon ? (
            <span
              className={cn(
                "mb-4 grid size-11 place-items-center rounded-full",
                tone === "danger"
                  ? "bg-[var(--danger-soft)] text-[var(--danger)]"
                  : "bg-[var(--accent-soft)] text-[var(--accent)]",
              )}
            >
              <Icon className="size-5" aria-hidden="true" />
            </span>
          ) : null}
          <DialogPrimitive.Title className="font-display pr-10 text-2xl text-[var(--ink-primary)]">
            {title}
          </DialogPrimitive.Title>
          {description ? (
            <DialogPrimitive.Description className="mt-2 text-sm leading-6 text-[var(--ink-secondary)]">
              {description}
            </DialogPrimitive.Description>
          ) : null}
          {children ? <div className="mt-5">{children}</div> : null}
          {footer ? <div className="mt-6 flex justify-end gap-2">{footer}</div> : null}
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}

interface ConfirmationDialogProps extends DialogFrameProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  confirmLabel: string;
  cancelLabel?: string;
  onConfirm: () => void;
  confirmDisabled?: boolean;
  pending?: boolean;
  onCloseAutoFocus?: ComponentPropsWithoutRef<
    typeof AlertDialogPrimitive.Content
  >["onCloseAutoFocus"];
}

export function ConfirmationDialog({
  open,
  onOpenChange,
  title,
  description,
  icon = AlertTriangle,
  tone = "default",
  children,
  confirmLabel,
  cancelLabel = "Отмена",
  onConfirm,
  confirmDisabled,
  pending,
  onCloseAutoFocus,
}: ConfirmationDialogProps) {
  const Icon = icon;
  return (
    <AlertDialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <AlertDialogPrimitive.Portal>
        <AlertDialogPrimitive.Overlay className="fixed inset-0 z-[80] bg-[var(--surface-overlay)] backdrop-blur-[1px]" />
        <AlertDialogPrimitive.Content
          onCloseAutoFocus={onCloseAutoFocus}
          className="fixed right-0 bottom-0 left-0 z-[81] max-h-[min(88dvh,760px)] overflow-y-auto rounded-t-[var(--radius-lg)] border border-[var(--border-strong)] bg-[var(--surface-raised)] p-6 shadow-[0_24px_64px_var(--shadow-color)] focus:outline-none sm:top-1/2 sm:right-auto sm:bottom-auto sm:left-1/2 sm:w-[min(520px,calc(100vw-40px))] sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-[var(--radius-lg)]"
        >
          <span
            className={cn(
              "mb-4 grid size-11 place-items-center rounded-full",
              tone === "danger"
                ? "bg-[var(--danger-soft)] text-[var(--danger)]"
                : "bg-[var(--accent-soft)] text-[var(--accent)]",
            )}
          >
            <Icon className="size-5" aria-hidden="true" />
          </span>
          <AlertDialogPrimitive.Title className="font-display text-2xl text-[var(--ink-primary)]">
            {title}
          </AlertDialogPrimitive.Title>
          {description ? (
            <AlertDialogPrimitive.Description className="mt-2 text-sm leading-6 text-[var(--ink-secondary)]">
              {description}
            </AlertDialogPrimitive.Description>
          ) : null}
          {children ? <div className="mt-5">{children}</div> : null}
          <div className="mt-6 flex justify-end gap-2">
            <AlertDialogPrimitive.Cancel asChild>
              <Button type="button" variant="secondary">
                {cancelLabel}
              </Button>
            </AlertDialogPrimitive.Cancel>
            <AlertDialogPrimitive.Action asChild>
              <Button
                type="button"
                variant={tone === "danger" ? "destructive" : "primary"}
                disabled={confirmDisabled || pending}
                onClick={onConfirm}
              >
                {pending ? "Выполняем…" : confirmLabel}
              </Button>
            </AlertDialogPrimitive.Action>
          </div>
        </AlertDialogPrimitive.Content>
      </AlertDialogPrimitive.Portal>
    </AlertDialogPrimitive.Root>
  );
}
