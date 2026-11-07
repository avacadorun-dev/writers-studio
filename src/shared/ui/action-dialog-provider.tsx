import type { LucideIcon } from "lucide-react";
import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
} from "react";

import { Button } from "./button";
import { ConfirmationDialog } from "./dialog";
import { Field, Input, Select } from "./field";

interface BaseDialogOptions {
  title: string;
  description?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  icon?: LucideIcon;
  tone?: "default" | "danger";
}

interface InputDialogOptions extends BaseDialogOptions {
  label: string;
  defaultValue?: string;
  placeholder?: string;
  requiredValue?: string;
  multiline?: boolean;
  rows?: number;
}

export interface ActionDialogOption {
  value: string;
  label: string;
}

interface SelectDialogOptions extends BaseDialogOptions {
  label: string;
  defaultValue?: string;
  options: ActionDialogOption[];
}

interface ActionDialogContextValue {
  requestInput: (options: InputDialogOptions) => Promise<string | undefined>;
  requestConfirmation: (options: BaseDialogOptions) => Promise<boolean>;
  requestSelect: (options: SelectDialogOptions) => Promise<string | undefined>;
}

type PendingDialog =
  | {
      kind: "input";
      options: InputDialogOptions;
      resolve: (value: string | undefined) => void;
    }
  | {
      kind: "confirmation";
      options: BaseDialogOptions;
      resolve: (value: boolean) => void;
    }
  | {
      kind: "select";
      options: SelectDialogOptions;
      resolve: (value: string | undefined) => void;
    };

const ActionDialogContext = createContext<ActionDialogContextValue | null>(null);

export function ActionDialogProvider({ children }: { children: ReactNode }) {
  const [pending, setPending] = useState<PendingDialog>();
  const [value, setValue] = useState("");
  const returnFocusRef = useRef<HTMLElement | null>(null);
  const dialogOpenRef = useRef(false);
  const rememberInvoker = useCallback(() => {
    const active = document.activeElement;
    if (
      !(active instanceof HTMLElement) ||
      active === document.body ||
      active.closest('[role="alertdialog"]')
    )
      return;
    const menu = active.closest('[role="menu"]');
    const menuTrigger = menu?.id
      ? Array.from(
          document.querySelectorAll<HTMLElement>('[aria-haspopup="menu"][aria-controls]'),
        ).find((element) => element.getAttribute("aria-controls") === menu.id)
      : undefined;
    returnFocusRef.current = menuTrigger ?? active;
  }, []);

  const requestInput = useCallback(
    (options: InputDialogOptions) =>
      new Promise<string | undefined>((resolve) => {
        rememberInvoker();
        dialogOpenRef.current = true;
        setValue(options.defaultValue ?? "");
        setPending({ kind: "input", options, resolve });
      }),
    [rememberInvoker],
  );
  const requestConfirmation = useCallback(
    (options: BaseDialogOptions) =>
      new Promise<boolean>((resolve) => {
        rememberInvoker();
        dialogOpenRef.current = true;
        setPending({ kind: "confirmation", options, resolve });
      }),
    [rememberInvoker],
  );
  const requestSelect = useCallback(
    (options: SelectDialogOptions) =>
      new Promise<string | undefined>((resolve) => {
        rememberInvoker();
        dialogOpenRef.current = true;
        setValue(options.defaultValue ?? options.options[0]?.value ?? "");
        setPending({ kind: "select", options, resolve });
      }),
    [rememberInvoker],
  );
  const contextValue = useMemo(
    () => ({ requestInput, requestConfirmation, requestSelect }),
    [requestConfirmation, requestInput, requestSelect],
  );

  const close = useCallback(() => {
    dialogOpenRef.current = false;
    setPending((current) => {
      if (current?.kind === "confirmation") {
        current.resolve(false);
      } else {
        current?.resolve(undefined);
      }
      return undefined;
    });
  }, []);

  const confirm = useCallback(() => {
    dialogOpenRef.current = false;
    setPending((current) => {
      if (!current) {
        return undefined;
      }
      if (current.kind === "confirmation") {
        current.resolve(true);
      } else {
        current.resolve(value);
      }
      return undefined;
    });
  }, [value]);

  const isConfirmation = pending?.kind === "confirmation";
  const requiredValue = pending?.kind === "input" ? pending.options.requiredValue : undefined;
  const confirmDisabled =
    pending?.kind === "input" && requiredValue !== undefined && value !== requiredValue;

  return (
    <ActionDialogContext.Provider value={contextValue}>
      {children}
      {pending ? (
        <ConfirmationDialog
          open
          onOpenChange={(open) => {
            if (!open) close();
          }}
          title={pending.options.title}
          description={pending.options.description}
          icon={pending.options.icon}
          tone={pending.options.tone}
          confirmLabel={pending.options.confirmLabel ?? (isConfirmation ? "Подтвердить" : "Готово")}
          cancelLabel={pending.options.cancelLabel}
          confirmDisabled={confirmDisabled}
          onConfirm={confirm}
          onCloseAutoFocus={(event) => {
            if (dialogOpenRef.current) {
              event.preventDefault();
              return;
            }
            if (returnFocusRef.current?.isConnected) {
              event.preventDefault();
              returnFocusRef.current.focus();
            }
          }}
        >
          {pending.kind === "input" ? (
            <form
              onSubmit={(event) => {
                event.preventDefault();
                if (!confirmDisabled) confirm();
              }}
            >
              <Field
                label={pending.options.label}
                hint={
                  requiredValue !== undefined
                    ? `Для подтверждения введите: ${requiredValue}`
                    : undefined
                }
              >
                {pending.options.multiline ? (
                  <textarea
                    rows={pending.options.rows ?? 8}
                    value={value}
                    placeholder={pending.options.placeholder}
                    className="w-full resize-y rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-paper)] px-3 py-2 font-mono text-sm leading-6 text-[var(--ink-primary)] focus:border-[var(--focus)] focus:outline-none"
                    onChange={(event) => setValue(event.target.value)}
                  />
                ) : (
                  <Input
                    autoFocus
                    value={value}
                    placeholder={pending.options.placeholder}
                    onChange={(event) => setValue(event.target.value)}
                  />
                )}
              </Field>
              <Button type="submit" className="sr-only" tabIndex={-1} aria-hidden="true">
                {pending.options.confirmLabel ?? "Готово"}
              </Button>
            </form>
          ) : pending.kind === "select" ? (
            <Field label={pending.options.label}>
              <Select autoFocus value={value} onChange={(event) => setValue(event.target.value)}>
                {pending.options.options.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </Select>
            </Field>
          ) : null}
        </ConfirmationDialog>
      ) : null}
    </ActionDialogContext.Provider>
  );
}

export function useActionDialog(): ActionDialogContextValue {
  const context = useContext(ActionDialogContext);
  if (!context) {
    throw new Error("useActionDialog must be used inside ActionDialogProvider");
  }
  return context;
}
