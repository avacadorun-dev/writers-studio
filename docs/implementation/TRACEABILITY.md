# Requirements and verification traceability

This public English index replaces private deployment-evidence inventories.
Historical PASS labels are intentionally not presented as current execution
results. Requirements remain in [WORKFLOWS](../specification/WORKFLOWS.md), with
all original numbered AC identifiers retained.

| Requirements | Primary implementation and checks |
| --- | --- |
| WF-01–WF-04 / AC-01–AC-10 | Project/store APIs, editor autosave, lifecycle/browser tests |
| WF-05–WF-10 / AC-11–AC-29 | Dictation controller, speech gateway, chunk recovery, finalization tests |
| WF-11–WF-14 / AC-30–AC-41 | Transcript review/diff/transfer and dictionary tests |
| WF-15–WF-19 / AC-42–AC-55 | Versions, search/replace, exports, recovery and purge tests |
| WF-20–WF-26 / AC-56–AC-86 | Assistant orchestration, scoped MCP, provider tests, staged apply/undo |
| WF-27–WF-29 / AC-87–AC-110 | Guide/memory revisions and voice/chat continuity |
| WF-30–WF-32 / AC-111–AC-131 | Exact anchors, protected ranges, dynamic structures and semantic protocol |
| WF-33 / AC-132–AC-142 | Mounted workspace, inline preview, modal review and direct Apply/Undo |
| WF-34–WF-36 / AC-143–AC-167 | Annotation mapping/history, project search and staged exact fixes |
| WF-37 / AC-168–AC-176 | Independent conversations, isolated drafts and shared-revision conflicts |
| AC-MA-01–AC-MA-23 | Annotation domain/unit, PostgreSQL integration and browser scenarios |
| CAN-01–CAN-08 | Canvas model/history/codec/recovery, validator and coverage gates |
| AC-NAV-01/02, AC-PT-01–05 | Secondary navigation and native project transfer tests |
| WS-0701–WS-0713 | Editor rich blocks, responsive toolbar, typography and assistant review |

Tests live next to frontend source and under `tests/e2e`; Go tests live beside
packages under `internal/`. PostgreSQL integration uses the `integration` build
tag and a disposable `TEST_DATABASE_URL`. Real-provider/browser checks are opt-in
and distinct from deterministic fixtures. See [verification](VERIFICATION_REPORT.md).
