# Historical release 0.7.0: Editor and assistant usability

This is the final recorded release line of the frozen Go/web edition. The notes
preserve feature identifiers while omitting private rollout details.

| ID | Resulting behavior |
| --- | --- |
| WS-0701 | Responsive editor toolbar with automatic, expanded and compact modes |
| WS-0702 | Image drop at a precise manuscript position after durable upload |
| WS-0703 | Independent line/paragraph/list spacing controls |
| WS-0704 | Editable inline and block LaTeX with safe KaTeX rendering |
| WS-0705 | Predictable inline-code formatting regardless of backtick entry order |
| WS-0706 | Visible, configurable manuscript working-width boundaries |
| WS-0707 | Aligned empty states, headings, caret and quick actions |
| WS-0708 | Stable editor status bar without unnecessary content movement |
| WS-0709 | Quick project/material renaming with ordinary validation |
| WS-0710 | Consistent docked assistant shell in Studio and Canvas |
| WS-0711 | Independent assistant transcript and review scrolling |
| WS-0712 | Safe, bounded rendering of long messages and JSON payloads |
| WS-0713 | Clear diff-first hierarchy in change review |

Document schema v4 adds mathematics while preserving old revision readability.
Native archives remain a separate versioned format. Check the current source for
accepted versions; older release notes do not override validation.

Quality gates include Bun lint/type/unit/build, Go tests/vet/race, database
integration, browser review and rich-block coverage. See the
[verification guide](VERIFICATION_REPORT.md) for commands and limitations.
