import { type RefObject, useCallback, useEffect, useRef, useState } from "react";

interface DismissiblePanel<T> {
  value: T | undefined;
  isOpen: boolean;
  panelRef: RefObject<HTMLDivElement | null>;
  triggerRef: RefObject<HTMLSpanElement | null>;
  open: (value: T) => void;
  close: () => void;
}

export function useDismissiblePanel<T>(): DismissiblePanel<T> {
  const [value, setValue] = useState<T>();
  const panelRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLSpanElement>(null);
  const open = useCallback((next: T) => setValue(next), []);
  const close = useCallback(() => setValue(undefined), []);

  useEffect(() => {
    if (value === undefined) return;

    const onPointerDown = (event: globalThis.PointerEvent) => {
      if (
        !(event.target instanceof Node) ||
        panelRef.current?.contains(event.target) ||
        triggerRef.current?.contains(event.target)
      ) {
        return;
      }
      close();
    };
    const onKeyDown = (event: globalThis.KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      close();
    };

    document.addEventListener("pointerdown", onPointerDown, true);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown, true);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [close, value]);

  return { value, isOpen: value !== undefined, panelRef, triggerRef, open, close };
}
