import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { Link, useNavigate } from "@tanstack/react-router";
import { BookOpen, LifeBuoy, LogOut, Settings } from "lucide-react";

import { useSession } from "~entities/session";
import { useLogout } from "~features/auth";
import { BrandMark, Button } from "~shared/ui";

export function WorkspaceHeader() {
  const session = useSession();
  const logout = useLogout();
  const navigate = useNavigate();
  const initials = (session.data?.user.displayName || "Автор")
    .split(/\s+/)
    .map((part) => part[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();

  return (
    <header className="flex h-[60px] items-center justify-between border-b border-[var(--border-subtle)] bg-[var(--surface-panel)] px-4 sm:px-7">
      <Link
        to="/projects"
        aria-label="Студия автора — проекты"
        className="flex min-h-11 items-center gap-3 rounded-[var(--radius-sm)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus)]"
      >
        <BrandMark className="size-10 shrink-0" aria-hidden="true" />
        <span className="font-display hidden text-[19px] leading-6 sm:inline">Студия автора</span>
      </Link>

      <div className="flex items-center gap-1">
        <Button asChild variant="ghost" size="icon" tooltip="Открыть документацию">
          <a href="/docs" aria-label="Открыть документацию">
            <BookOpen className="size-4" aria-hidden="true" />
          </a>
        </Button>
        <Button asChild variant="ghost" size="icon" tooltip="Открыть центр восстановления">
          <Link to="/recovery" aria-label="Открыть центр восстановления">
            <LifeBuoy className="size-4" aria-hidden="true" />
          </Link>
        </Button>
        <Button asChild variant="ghost" size="icon" tooltip="Открыть настройки">
          <Link to="/settings" aria-label="Открыть настройки">
            <Settings className="size-4" aria-hidden="true" />
          </Link>
        </Button>
        <DropdownMenu.Root>
          <DropdownMenu.Trigger asChild>
            <button
              type="button"
              className="ui-tooltip ml-1 grid size-11 place-items-center rounded-full bg-[var(--surface-selected)] text-xs font-semibold text-[var(--accent-ink)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus)]"
              aria-label="Меню профиля"
              data-ui-tooltip="Меню профиля"
              data-ui-tooltip-side="bottom"
            >
              {initials}
            </button>
          </DropdownMenu.Trigger>
          <DropdownMenu.Portal>
            <DropdownMenu.Content
              align="end"
              sideOffset={8}
              className="z-50 min-w-56 rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-raised)] p-1 shadow-[0_10px_24px_var(--shadow-color)]"
            >
              <DropdownMenu.Label className="px-3 py-2 text-xs text-[var(--ink-tertiary)]">
                {session.data?.user.email}
              </DropdownMenu.Label>
              <DropdownMenu.Separator className="my-1 h-px bg-[var(--border-subtle)]" />
              <DropdownMenu.Item
                className="flex min-h-10 cursor-default items-center gap-2 rounded-[var(--radius-sm)] px-3 text-sm outline-none data-[highlighted]:bg-[var(--surface-subtle)]"
                onSelect={() =>
                  logout.mutate(undefined, {
                    onSuccess: () => void navigate({ to: "/login", replace: true }),
                  })
                }
              >
                <LogOut className="size-4" aria-hidden="true" />
                Выйти
              </DropdownMenu.Item>
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>
      </div>
    </header>
  );
}
