import { useCallback, useEffect, useRef, useState } from "react";

export function useToolRailMenu<TMenu extends string>() {
  const [openMenu, setOpenMenu] = useState<TMenu>();
  const railRef = useRef<HTMLDivElement>(null);

  const closeMenu = useCallback(() => setOpenMenu(undefined), []);
  const toggleMenu = useCallback(
    (menu: TMenu) => setOpenMenu((current) => (current === menu ? undefined : menu)),
    [],
  );

  useEffect(() => {
    if (!openMenu) return;
    const dismissOutside = (event: globalThis.PointerEvent) => {
      if (!railRef.current?.contains(event.target as Node)) closeMenu();
    };
    const dismissWithEscape = (event: globalThis.KeyboardEvent) => {
      if (event.key === "Escape") closeMenu();
    };
    document.addEventListener("pointerdown", dismissOutside);
    document.addEventListener("keydown", dismissWithEscape);
    return () => {
      document.removeEventListener("pointerdown", dismissOutside);
      document.removeEventListener("keydown", dismissWithEscape);
    };
  }, [closeMenu, openMenu]);

  return { closeMenu, openMenu, railRef, toggleMenu };
}
