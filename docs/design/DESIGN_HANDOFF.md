# Product design reference

This is an English reference for the frozen Go/web edition. It replaces private
handoff logistics and obsolete prototype assignments with the public interaction
contracts. Original design-service files/tokens are not required to run the app.

## Principles and layout

The manuscript is primary. Capture must communicate reliability honestly, with
separate local/server/recognition statuses. Editing remains a complete activity.
Use progressive disclosure, legible prose typography and explicit actions rather
than decorative claims about AI. Avoid success messages before durable completion.

Target viewports are 1440×1024, 1024×768, 834×1194 and 390×844. Desktop supports
simultaneous manuscript and assistant; tablet uses overlays where necessary;
mobile prioritizes capture, review and safe navigation. Secondary screens always
have an explicit return path, including when opened through a direct URL.

## Screen and state inventory

| Screens | Responsibility |
| --- | --- |
| S-01–S-03 | Account entry, project library and project creation |
| S-04 | Studio tree, manuscript, formatting, source references and save state |
| S-05 | Dictation destination/language/microphone/storage/provider preflight |
| S-06–S-08 | Recording, degraded recognition/network, and paused recording |
| S-09 | Durable finalization progress, failure and retry |
| S-10–S-11 | Transcript/audio review and late-result comparison |
| S-12–S-17 | Versions, search, export, dictionary, recovery and settings |
| S-18–S-20 | Voice-mode choice, assistant composition and execution |
| S-21–S-22 | Modal change review and durable activity |
| S-23–S-25 | Provider list, connection form and probe results |
| S-26–S-30 | Guide, typed memory, chat, voice draft handoff and context |
| S-31–S-37 | Dictionary links, margin notes, project annotation list and fixes |
| S-38–S-41 | Conversation switcher, new chat, conflicts and responsive presentation |

Screen identifiers are historical cross-references, not a requirement for a
separate route per state.

## Editor and assistant

Formatting is accessible through a responsive toolbar. Compact layouts use an
overflow menu without horizontal scrolling. Keep manuscript column width, spacing,
caret alignment and footer status stable. Source playback does not disable editing.
Selection actions open through right-click, Shift+F10/context-menu key or long
press; merely selecting text does not add a permanent floating toolbar.

An open assistant stays beside the mounted editor. Use the compact scale from
[ADR-007](../adr/ADR-007-assistant-continuous-workspace.md). Preview overlays are
labeled as unapplied and excluded from autosave/undo until Apply. Conflicting
manual edits make proposals stale without reverting the author's work.

Review emphasizes readable exact differences. Groups, instruction and target
context are secondary. Apply, Reject, Stop and Undo have distinct meanings. Long
JSON/tool output is safely collapsed/scrollable rather than stretching the layout.
Voice and chat continue the same conversation without auto-submitting drafts.

## Recording and recovery

Separate session elapsed time from captured audio duration. Distinguish partial
text from stable segments without relying on color. Manual scrolling disables
follow-live; an explicit action resumes it. Pause/resume preserves the session.
A recognition outage states that local recording continues when it actually does.

Finalization progress survives navigation. Failed finalization retains source
access and retry. Recovery lists unfinished work, download and explicit deletion;
local-only deletion requires an unambiguous confirmation.

## Providers and privacy

Show provider/model and selected context before execution. Explain the difference
between base URL, model identifier and API key. A reachable endpoint without the
required tool chain is not presented as assistant-ready. Guide, dictionary and
memory remain distinguishable. Do not imply another provider's retention policy.

## Accessibility and design QA

Use semantic controls, visible focus, keyboard navigation, accessible icon labels,
textual statuses and touch targets at least 44 px. Do not announce every partial
word through a live region. Dialogs restore focus; background pages are not
interactive while a modal is active. Notes and diff states need non-color cues.

Check recording/degraded/offline/retry, paste and rich blocks, selection retention,
modal review, conflict copies, annotation mapping, multiple conversations,
responsive bounds, independent scroll, keyboard and screen-reader operation.
Historical prototype tasks are not claims that usability research was completed.
