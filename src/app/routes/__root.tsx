import { createRootRoute, Outlet } from "@tanstack/react-router";

export const Route = createRootRoute({
  component: RootLayout,
  notFoundComponent: NotFound,
});

function RootLayout() {
  return <Outlet />;
}

function NotFound() {
  return (
    <main className="grid min-h-full place-items-center p-6">
      <section className="max-w-md text-center">
        <p className="text-xs font-semibold tracking-[0.16em] text-[var(--ink-tertiary)] uppercase">
          Страница не найдена
        </p>
        <h1 className="font-display mt-3 text-3xl">Вернитесь к рукописи</h1>
        <a
          href="/projects"
          className="mt-6 inline-flex min-h-11 items-center rounded-[var(--radius-sm)] bg-[var(--accent-surface)] px-4 text-sm font-medium text-[var(--ink-on-solid)]"
        >
          Открыть проекты
        </a>
      </section>
    </main>
  );
}
