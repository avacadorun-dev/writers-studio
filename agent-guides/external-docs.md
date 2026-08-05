# Основные внешние документации

Используй эти ссылки как первичные источники перед изменениями в соответствующей области.

## Runtime и tooling

- [Bun Documentation](https://bun.sh/docs)
- [Bun test runner](https://bun.sh/docs/cli/test)
- [Biome Getting Started](https://biomejs.dev/guides/getting-started/)
- [Biome configuration](https://biomejs.dev/reference/configuration/)
- [Vite Guide](https://vite.dev/guide/)

## Архитектура

- [Feature-Sliced Design docs](https://feature-sliced.design/docs)
- [FSD layers](https://feature-sliced.design/docs/reference/layers)
- [FSD public API](https://feature-sliced.design/docs/reference/public-api)

## React и роутинг

- [React docs](https://react.dev/learn)
- [React TypeScript guide](https://react.dev/learn/typescript)
- [TanStack Router React docs](https://tanstack.com/router/latest/docs/framework/react/overview)
- [TanStack Query React docs](https://tanstack.com/query/latest/docs/framework/react/overview)

## UI

- [Tailwind CSS documentation](https://tailwindcss.com/docs)
- [Tailwind CSS with Vite](https://tailwindcss.com/docs/installation/using-vite)
- [Radix Primitives documentation](https://www.radix-ui.com/primitives/docs/overview/introduction)
- [shadcn/ui documentation](https://ui.shadcn.com/docs)
- [class-variance-authority documentation](https://cva.style/docs)
- [MDN Accessibility](https://developer.mozilla.org/en-US/docs/Web/Accessibility)

Для UI ориентируйся на Tailwind CSS и композицию доступных primitives из shadcn/ui. Используй Radix напрямую только когда нужного primitive нет в shadcn/ui; добавляй компонент из shadcn/ui как исходный код проекта, а не как непрозрачную зависимость.

## Проектные docs

- [OpenClaw API](../docs/api/openclaw.md)
- [Auth architecture](../docs/auth/01-architecture.md)
- [Auth flows](../docs/auth/02-auth-flows.md)
- [Keycloak setup](../docs/auth/03-keycloak-setup.md)
- [Auth troubleshooting](../docs/auth/04-troubleshooting.md)
