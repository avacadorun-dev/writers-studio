# API, authentication and environment configuration

## API boundaries

Shared HTTP infrastructure lives in `src/shared/api`. Put entity requests in
`entities/<entity>/api`, workflow-specific requests in `features/<feature>/api`,
and temporary widget integration in `widgets/<widget>/api` only when necessary.
Components consume hooks and models rather than calling `fetch` directly.

Keep transport DTOs beside their requests and domain/UI types in `model`. Introduce
pure adapters when transport data differs from the client contract: normalize nulls,
defaults, dates, enums, names, nested structures and discriminated responses explicitly.
Do not conceal optional transport fields inside supposedly complete UI state.

## Server state

Use TanStack Query with stable keys. Mutations invalidate or update related queries.
Do not duplicate query-owned data in local state. Pages, widgets and features render
loading, error and empty states; API helpers return data or typed errors.

## Authentication and secrets

The application uses its own server sessions; inspect `internal/httpapi/auth.go`,
`internal/store/auth.go`, `src/entities/session` and `src/features/auth` for the
current contract. Do not assume unrelated identity-provider documentation exists.
Never put server secrets in frontend code. `VITE_*` values and `window.__ENV__`
are public to the browser.

## Runtime configuration

Browser runtime configuration is read by `src/shared/config/env.ts`, from
`window.__ENV__` with an `import.meta.env` fallback. The served runtime file is
`public/env-config.js`; server environment examples live in `.env.example`.

When adding a variable, update its boundary type, shared configuration export,
relevant container configuration and the sanitized example. Use a safe default only
when appropriate for development. Do not modify a local `.env` without authorization;
never print its contents or include it in commits, images or release snapshots.
