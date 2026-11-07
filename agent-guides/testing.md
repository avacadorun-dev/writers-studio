# Frontend testing

Use the existing Bun test runner. Add behavior tests for new business rules, important
boundaries and nontrivial state transitions. Avoid tests that merely restate types,
pass-through implementations or third-party behavior.

## What to cover

- Pure helpers, normalization, domain rules and transport adapters.
- Hooks with branching, asynchronous state, cache invalidation or side effects.
- Authentication and permission boundaries when behavior changes.
- Empty inputs, absent optional fields, invalid data, fallbacks and access failures
  where those cases change the user-visible result.

Do not add tests for re-exports, generated route trees, thin presentation or obvious
library behavior without a concrete reason.

## Placement and approach

Place `*.test.ts` or `*.test.tsx` beside the implementation, using kebab-case names.
Test public behavior. Mock the network at the API boundary. Control time, storage
and environment inputs; avoid real-network, timer-dependent or order-dependent tests.
Snapshots must not be the principal proof of correctness. One meaningful behavior
can cover several implementation branches.

Use `bun run test` for the frontend suite. The project script limits discovery to
`src`, keeping Playwright specifications separate. Relevant integration and browser
commands are documented in the README and verification guide.

When commits are authorized, include tests with the behavior they verify. Run the
relevant suite and full frontend checks for changes spanning multiple areas. Never
commit intentionally failing, focused or skipped tests without explicit agreement.
