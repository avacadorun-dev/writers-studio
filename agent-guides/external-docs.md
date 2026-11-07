# Primary external documentation

Consult the relevant primary documentation when working in these areas.

## Runtime and tooling

- [Bun](https://bun.sh/docs)
- [Bun tests](https://bun.sh/docs/cli/test)
- [Biome](https://biomejs.dev/guides/getting-started/)
- [Biome configuration](https://biomejs.dev/reference/configuration/)
- [Vite](https://vite.dev/guide/)

## Architecture

- [Feature-Sliced Design](https://feature-sliced.design/docs)
- [Layers](https://feature-sliced.design/docs/reference/layers)
- [Public APIs](https://feature-sliced.design/docs/reference/public-api)

## React and routing

- [React](https://react.dev/learn)
- [React and TypeScript](https://react.dev/learn/typescript)
- [TanStack Router](https://tanstack.com/router/latest/docs/framework/react/overview)
- [TanStack Query](https://tanstack.com/query/latest/docs/framework/react/overview)

## UI

- [Tailwind CSS](https://tailwindcss.com/docs)
- [Tailwind with Vite](https://tailwindcss.com/docs/installation/using-vite)
- [Radix Primitives](https://www.radix-ui.com/primitives/docs/overview/introduction)
- [shadcn/ui](https://ui.shadcn.com/docs)
- [class-variance-authority](https://cva.style/docs)
- [MDN accessibility](https://developer.mozilla.org/en-US/docs/Web/Accessibility)

Prefer the established Tailwind and accessible primitive composition. Add Radix
packages only for needed behavior; keep adapted components as project source.
For project-specific contracts, use `docs/architecture`, `docs/adr` and
`docs/specification`. Provider and authentication setup must match the actual server
implementation and `.env.example` rather than documentation from another project.
