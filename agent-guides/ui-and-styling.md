# UI, styling and accessibility

## Stack

Use Tailwind CSS 4, Radix UI primitives and the project's shadcn/ui source components.
Do not introduce another UI or styling framework without an architectural decision.
For substantially changed legacy UI, prefer the established stack.

Dependencies include `tailwindcss`, `@tailwindcss/vite`, the Radix packages actually
used, `class-variance-authority`, `clsx`, `tailwind-merge` and `lucide-react`.
Do not install entire component catalogs speculatively.

## Shared primitives

Keep `shared/ui` domain-independent. Components there must not know about assistant
runs, providers, authentication entities or domain permissions. Put entity rendering
in `entities`, user actions in `features`, substantial compositions in `widgets`,
and route composition in `pages`. Store adapted shadcn components in kebab-case
files and export them from the public API.

## Styling

Keep Tailwind's entry point, resets and theme tokens in `shared/styles`. Use CSS
custom properties for colors and radii. Prefer utility classes for layout, spacing,
typography, responsive behavior and states. Extract recurring patterns into reusable
components rather than global utility classes. Use `class-variance-authority` for
variants and `cn` for merging classes. CSS is appropriate for complex animations,
browser rules and third-party integrations. Nontrivial inline styles are reserved
for dynamic values that cannot use a finite class set or CSS variables.

## Accessibility

- Use semantic interactive elements and complete keyboard navigation.
- Provide visible focus indicators and accessible names for icon-only buttons.
- Associate input labels with controls. Add ARIA only where native semantics are insufficient.
- Communicate state with text or icons as well as color.
- Use `lucide-react` for icons; hide decorative icons from assistive technology.
- Localize visible labels, accessible names, hints, validation messages and empty states.

## Component boundaries

Use one main component per file. At approximately 300–350 lines, look for meaningful
subcomponents or model boundaries; do not split mechanically to satisfy a count.
Avoid unnecessary DOM wrappers. Keep data fetching and complicated transforms out
of presentation. Extract form normalization and defaults when the logic grows.

Shared assets live in `shared/ui/assets`; slice-specific assets stay in their slice.
Use kebab-case for new asset names without renaming unrelated existing files.
