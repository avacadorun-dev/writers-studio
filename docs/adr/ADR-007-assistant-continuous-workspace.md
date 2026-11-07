# ADR-007: Continuous assistant workspace inside Studio

Status: accepted; applies to the frozen edition.

## Context

Authors need the manuscript, instruction, ordered assistant history and review
context together. Large display typography in a narrow assistant panel and
full-screen review transitions interrupt that work.

## Decision

1. Use a compact docked scale: main transcript/chat 14 px, activity 13 px,
   supporting text 10–11 px, service labels 9 px and header title 15 px.
   Markdown headings stay approximately 14–15 px.
2. A fresh, fully selected proposal offers both detailed review and direct Apply.
   Both paths use identical server staging/base-revision validation and provide
   grouped Undo after success.
3. Detailed review opens as a modal over the current Studio. Opening/closing does
   not change the route/URL or remount the editor. Preserve node, selection,
   cursor, scroll, autosave and manual undo state.
4. The modal displays exact diff, change groups, instruction and resolved target.
   Ordered history remains in the panel; do not duplicate it as a full-screen page.
5. Opening the assistant integrates the manuscript as a full workspace column.
   The ordinary floating manuscript treatment returns when the panel closes.
6. A request-for-revision form shows Cancel and Send. Apply/Reject return after
   cancellation or completion of the new turn.

## Consequences and regression checks

Legacy standalone routes may remain compatible deep links. They are not the main
docked-review action. Responsive dialogs have safe insets and independent scroll,
while preserving current run state. Test unchanged URLs, mounted editor state,
modal bounds, direct Apply/Undo and compact typography.

Changing these contracts requires a new architectural decision. In particular,
review must not hide/remount the manuscript or bypass server revision checks.
See [workflows](../specification/WORKFLOWS.md) and
[design reference](../design/DESIGN_HANDOFF.md).
