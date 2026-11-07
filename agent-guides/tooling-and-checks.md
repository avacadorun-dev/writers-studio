# Tooling, Bun, Biome and verification

## Runtime and package manager

Use Bun for frontend package management and project commands:

```sh
bun install --frozen-lockfile
bun run dev
bun run check
bun run type-check
bun run test
bun run build
```

Use `bunx --bun` for package executables. Do not introduce npm, Yarn, pnpm or Node
project scripts. Adapt third-party snippets to Bun.

## Formatting and linting

Biome is the project's formatter and linter; configuration lives in `biome.json`.
Do not add ESLint or Prettier. Change formatting rules in the shared configuration
and verify the resulting diff.

## UI tooling

Use Tailwind CSS 4 through `@tailwindcss/vite`. Keep shadcn configuration in
`components.json`; components resolve to `shared/ui`, utilities to `shared/lib`.
Add only used primitives and adapt their exports to the project's slice boundaries.

## TypeScript

Preserve strict mode, explicit public types, `unknown` instead of `any`, boundary
normalization and type guards. Do not use `@ts-ignore`. Any unavoidable
`@ts-expect-error` requires a short explanation.

## Verification before an authorized commit

```sh
bun run release:check
bun run check
bun run type-check
bun run test
bun run build
```

Run relevant Go and browser checks when those areas change. Git hooks, when installed,
are an additional guard rather than a replacement for complete verification. Do not
claim a command passed without running it against the reviewed state.
