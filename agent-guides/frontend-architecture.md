# Frontend architecture and Feature-Sliced Design

## Layers and dependency direction

Use Feature-Sliced Design. Place code by responsibility:

- `app`: application initialization, providers, global configuration and TanStack Router routes.
- `pages`: complete route screens assembled from widgets and features.
- `widgets`: substantial, self-contained screen sections.
- `features`: user actions and workflows.
- `entities`: domain types, models, requests and entity-specific UI.
- `shared`: infrastructure without domain knowledge: API client, configuration, libraries, hooks, styles and primitives.

Dependencies point downwards: `app -> pages -> widgets -> features -> entities -> shared`.
Lower layers must not import higher ones. Sibling slices must not reach into each other's internals.

## Public APIs and file layout

Expose each slice through `index.ts`. External imports use its public API:

```ts
import { ModelForm } from "~entities/models";
```

Do not deep-import another slice's `model`, `api`, `ui` or hooks. Within a slice,
prefer relative imports instead of importing its own public API, which can create cycles.

Use `api` for requests and transport adapters; `model` for state and business rules;
`lib` or `utils` for pure helpers; `hooks` when separate hooks are useful; `ui` for
components; and `assets` for slice-specific resources. Keep simple slices flat.
Use kebab-case for new files and directories. Components may use PascalCase.
Do not rename existing files as an unrelated cleanup.

## Components and state

Keep components focused. Move business rules and complicated transformations into
models, helpers or hooks. Do not perform HTTP requests directly in JSX components.
Represent loading, success, error and empty states explicitly. Compute derived data
instead of duplicating it in state. Query caches own server state.

## Routing

Route files live in `src/app/routes`; the generated tree is
`src/app/routeTree.gen.ts`. Never edit the generated tree manually. Route modules
import screen components from the `pages` layer.

## Refactoring and commits

When commits are authorized, keep preparatory refactors separate from behavior
changes. Each commit has one reviewable purpose; inspect its diff to avoid adding
unrelated user changes or generated artifacts. Use imperative scoped messages such
as `refactor(models): split model form sections`. Include relevant tests with the
behavior they verify. Do not mix unrelated renames, moves, style migrations and
functional changes.
