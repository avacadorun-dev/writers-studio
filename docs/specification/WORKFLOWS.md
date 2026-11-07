# Functional workflows and acceptance reference

This document describes the frozen Go/web edition. English editorial consolidation
preserves historical workflow and acceptance identifiers for existing tests and
cross-references. It is not an active roadmap. Specialized specifications give
additional data contracts; runtime validation resolves obsolete planning details.

## Cross-cutting rules

| Rule | Contract |
| --- | --- |
| R-01 | Preserve original sources. |
| R-02 | Replaceable partial recognition is not a canonical document. |
| R-03 | Manual edits take precedence over late automated results. |
| R-04 | Recording and speech recognition have independent lifecycles. |
| R-05 | Report success only after the relevant durable acknowledgement. |
| R-06 | Provide a complete manuscript editor. |
| R-07 | Use reversible operations where possible and explicit confirmation otherwise. |
| R-08 | Voice commands are explicit and allowlisted. |
| R-09 | Dictation and assistant instructions are distinct modes. |
| R-10 | MCP cannot bypass domain validation or ownership. |
| R-11 | Assistant writes remain staged until Apply. |
| R-12 | Action risk determines review or native confirmation. |
| R-13 | Retrieve the minimum relevant context. |
| R-14 | Probe actual provider capabilities. |
| R-15 | Project guidance is explicit and versioned. |
| R-16 | Project memory belongs to the project, not the provider. |
| R-17 | Voice and chat continue one ordered assistant run. |
| R-18 | Keep the manuscript present in the assistant workspace. |
| R-19 | Secondary screens have an explicit safe return path. |
| R-20 | Conversations are independent; manuscript revisions are shared. |

## Workflow map

| ID | Author journey and required result |
| --- | --- |
| WF-01 | Sign in or register, then open the project library and visible recovery entry. |
| WF-02 | Choose project type/name/language; create the project and initial structure atomically. |
| WF-03 | Create, reorder, move, rename, trash and restore schema-valid nodes with Undo. |
| WF-04 | Edit a content-bearing node, preserve selection, save revisions and resolve stale writes without loss. |
| WF-05 | Select destination, microphone, recognition language and command policy before starting capture. |
| WF-06 | Record locally while showing distinct partial/stable transcript, durations and source status. |
| WF-07 | Recognize explicit prefixed structural commands; keep raw source and offer Undo. |
| WF-08 | Pause/resume one session, flushing pending media and distinguishing elapsed/captured time. |
| WF-09 | Continue local recording during network/recognition failure and retry pending uploads. |
| WF-10 | Stop idempotently, finalize durably and retain source/draft/retry on failure. |
| WF-11 | Review editable transcript against bounded source audio and uncertainty markers. |
| WF-12 | Compare a late quality result without replacing manual work before acceptance. |
| WF-13 | Transfer a checked draft to an explicit manuscript destination; default to append and offer Undo. |
| WF-14 | Maintain canonical terms/variants/hints; historical text changes only through explicit actions. |
| WF-15 | Create named versions, compare without mutation and restore through a new revision. |
| WF-16 | Search, navigate to exact results, preview replacement and reject stale preview tokens. |
| WF-17 | Export a stable ordered snapshot; report failures without a misleading partial success. |
| WF-18 | Discover unfinished local work, download/recover it or explicitly delete it. |
| WF-19 | Archive, trash, restore and permanently delete through distinct, confirmed actions. |
| WF-20 | Choose dictation or assistant before microphone use. |
| WF-21 | Compose a voice/text instruction with visible target, read scope and provider. |
| WF-22 | Execute scoped reads and staged tools with readable activity, Stop and recoverable partial work. |
| WF-23 | Review exact groups, reject stale bases, Apply atomically and provide grouped Undo. |
| WF-24 | Hand sensitive actions to typed native flows; models cannot perform irreversible deletion. |
| WF-25 | Reopen durable activity/results and recover interrupted runs without replaying applied changes. |
| WF-26 | Configure/probe system or encrypted custom model connections with explicit capability status. |
| WF-27 | Create/edit an optional versioned guide, pin its revision and stage assistant-authored changes. |
| WF-28 | Capture dictionary and typed memory updates in one reviewable proposal with duplicate handling. |
| WF-29 | Switch voice/chat within one run, preserving drafts and requiring explicit submission. |
| WF-30 | Resolve an addressed edit to exact targets, respect protected text and retain instruction-to-diff trace. |
| WF-31 | Use project-defined structure types and validate structure revisions instead of hardcoded chapter kinds. |
| WF-32 | Begin each turn with one semantic mode; reject invalid declarations and lexical intent routing. |
| WF-33 | Keep the mounted manuscript, ordered assistant feed, unapplied preview and modal review in one workspace. |
| WF-34 | Create exact dictionary links/notes from context actions and map anchors with document revisions. |
| WF-35 | Search all project annotations, deep-link to exact quotes and transfer metadata portably. |
| WF-36 | Propose up to 50 annotation fixes using server-owned anchors; apply/undo text and note state atomically. |
| WF-37 | Switch independent conversations, preserve per-run drafts and detect shared-document conflicts. |

## Responsive and keyboard behavior

Desktop at 1024 px and above supports the full writing workspace. Tablet uses
adapted panels/overlays. Mobile prioritizes recording, review and navigation;
Canvas layout is read-only there. Opening responsive assistant states preserves
the mounted manuscript. Use visible focus, semantic names, textual status cues,
at least 44 px touch targets and standard keyboard editing. Selection actions are
available through right-click, Shift+F10/context-menu key and touch long press.

## Acceptance criteria

The following are requirements and test references, not a claim that every check
was rerun during publication preparation.

- AC-01. One primary new-project action is visible after sign-in.
- AC-02. Recovery remains discoverable from active projects and the shared shell.
- AC-03. Created projects and nodes survive a fresh read from PostgreSQL.
- AC-04. Create project and initial structure in one database transaction.
- AC-05. Refreshing the same node/revision preserves the caret and mounted editor.
- AC-06. Move and trash actions provide Undo without losing entity identity.
- AC-07. Acknowledged revisions and formatting survive reload.
- AC-08. Transcript transfer creates one identifiable revision and supports Undo.
- AC-09. Autosave acknowledgement preserves content, selection and scroll.
- AC-10. Late quality output creates an artifact/suggestion rather than a hidden editor transaction.
- AC-11. Starting recording gives visible feedback within the specified 300 ms target.
- AC-12. Destination and language are selectable before capture.
- AC-13. Microphone denial, missing device and recognizer failure have distinct states.
- AC-14. Partial text is visibly and accessibly identified as provisional.
- AC-15. Manual scrolling disables follow-live until explicitly resumed.
- AC-16. Recorder status and both durations have textual accessible semantics.
- AC-17. A second start is rejected while recording is active.
- AC-17a. The recorder footer exposes the command prefix, examples and accessible quick buttons.
- AC-18. Executed commands have an explicit status card and Undo.
- AC-19. Raw command speech remains in the stored source artifact.
- AC-20. Disabled voice commands preserve phrases as ordinary text.
- AC-20a. Quick buttons remain allowlisted and retain persistence/Undo semantics when voice commands are disabled.
- AC-21. Pause flushes pending recorder data before pausing.
- AC-22. Resume keeps the existing local/server session identity.
- AC-23. Elapsed session time and captured audio time are separate and durable.
- AC-24. Offline or WebSocket failure does not stop local MediaRecorder capture.
- AC-25. Degraded state distinguishes local recording from missing live text.
- AC-26. Reconnect uploads every unacknowledged local chunk.
- AC-27. Stop/finalization is idempotent; successful Deepgram completion avoids automatic batch STT.
- AC-28. Finalization runs durably outside the page lifecycle.
- AC-29. Failed finalization retains source audio, draft and retry.
- AC-30. Source references play only the bounded segment interval.
- AC-31. Manual transcript edits append draft revisions without altering source artifacts.
- AC-32. Dictionary hints reach live and explicit file-recognition paths.
- AC-33. Audio playback does not make the editor read-only.
- AC-34. A late suggestion cannot alter the draft before explicit acceptance.
- AC-35. Accepting a comparison creates a new source-labeled draft revision.
- AC-36. Each comparison group links to a bounded audio range.
- AC-37. Block-level source references validate and survive reload.
- AC-38. Undo transfer restores previous content through a new revision.
- AC-39. Transfer defaults to append; replacement requires explicit selection.
- AC-40. Dictionary changes disclose that old text is not rewritten automatically.
- AC-41. Re-transcription adds a suggestion and preserves the original source/draft.
- AC-42. Restore appends a revision and retains newer history.
- AC-43. Named versions are immutable snapshots separate from autosave revisions.
- AC-44. Comparison does not mutate content or revision state.
- AC-45. Search navigation selects/highlights the intended manuscript range.
- AC-46. Replacement requires a fresh preview token; stale tokens fail.
- AC-47. Exports follow recursive ordered structure and chosen revisions.
- AC-48. Export snapshot creation uses a stable transaction and workers read that snapshot.
- AC-49. TXT/Markdown export is not blocked by a billing gate.
- AC-50. A new tab can recover unfinished IndexedDB recording data.
- AC-51. Recovery can download assembled local audio before deletion.
- AC-52. Failed uploads retain unacknowledged chunks until explicit deletion/recovery.
- AC-53. Archive and trash have distinct states, filters and actions.
- AC-54. Restore preserves the project's tree and revision history.
- AC-55. Purge requires exact confirmation and states backup-retention limitations.
- AC-56. The purpose of dictation and assistant is clear before enabling the microphone.
- AC-57. Choosing assistant does not create a dictation session/source artifact.
- AC-58. Unavailable LLM service does not block dictation or manual editing.
- AC-59. Stable/partial instruction text differs; authors can edit before a model request.
- AC-60. Project, target, read context and provider/model are separately visible.
- AC-61. The model receives instruction text; audio input support is optional.
- AC-62. Silence checkpoints never auto-submit and refresh restores instruction/context/state.
- AC-63. Read tools cannot mutate canonical state.
- AC-64. Every tool revalidates ownership and project scope.
- AC-65. Ordered transcript/activity/results remain visible without secrets or raw technical payloads.
- AC-66. Stop prevents subsequent calls for that run.
- AC-67. Provider/tool failures retain the instruction and staged draft; continuation stays in one run.
- AC-67a. Create 1–50 sibling nodes with content in one call; each remains independently reviewable without using uncommitted IDs.
- AC-67b. Use SSE keepalive and the shared provider timeout without hidden per-connection overrides or automatic paid retries.
- AC-67c. Timeout after staging retains a partial proposal; timeout before mutation creates no empty change set.
- AC-67d. Stop cannot leave a working turn inside a terminal/cancelled run.
- AC-67e. Long-form output uses 1–16 chunks in one preview/revision; incomplete series remain explicitly partial.
- AC-67f. Long-form continuation uses accumulated counts and bounded tails rather than replaying all earlier chunks.
- AC-67g. Chat Completions streams; SSE comments and fragmented arguments never execute an incomplete tool call.
- AC-67h. Mid-stream failures retain safe diagnostics; HTTP 408/504/524 classify as timeouts.
- AC-68. Canonical project state remains unchanged until Apply.
- AC-69. Multi-document review lists affected objects and operation types.
- AC-70. A server failure cannot leave a partly applied change set.
- AC-71. A stale base blocks Apply.
- AC-72. Applied revisions include run/operation assistant provenance.
- AC-73. Grouped Undo restores all affected objects through compensating revisions.
- AC-74. Assistant change sets cannot alter source transcripts or original audio.
- AC-74a. Revision-request composition hides Apply/Reject until cancel or completion of the next turn.
- AC-75. The model catalog exposes no commit, purge or credential-read tool.
- AC-76. Sensitive A3 actions require native confirmation.
- AC-77. Prompt/project text cannot expand allowed tools or read scope.
- AC-78. Reload or a new tab reopens durable runs and ready proposals.
- AC-79. Deleting a provider preserves history and applied revisions.
- AC-80. Deleting run history does not undo committed project changes.
- AC-80a. Startup converts orphaned working turns once to recoverable execution_interrupted without canonical changes.
- AC-81. Submitted API keys are masked and never returned in full.
- AC-82. System environment credentials cannot be read/changed through UI or MCP.
- AC-83. A connection without reliable sequential tools is unavailable for assistant selection.
- AC-84. A failed probe does not activate invalid credentials.
- AC-85. Provider/model stay pinned during a run.
- AC-86. Deleting a connection prevents new runs without breaking history.
- AC-87. Projects work without a guide and acquire no hidden generated style rules.
- AC-88. At most one project assistant guide is active.
- AC-89. Each confirmed turn pins an exact guide revision or explicit absence.
- AC-90. Targeted and project-wide assistant use the same active project guide.
- AC-91. Ordinary notes cannot impersonate a guide or override policy.
- AC-92. Assistant guide creation/update always has staged review.
- AC-93. Restoring guidance appends a revision and does not change an executing turn.
- AC-94. Guides are excluded from reader manuscript/search/export by default.
- AC-95. An explicit instruction can propose dictionary and memory updates together.
- AC-96. Dictionary/memory remain unchanged until Apply.
- AC-97. Dictionary records contain canonical spelling, variants, type and optional pronunciation.
- AC-98. Memory records contain kind, statement, scope, sources, tags and revision.
- AC-99. Duplicates/conflicts require an explicit merge/update decision.
- AC-100. Later turns retrieve committed facts through owner-scoped typed tools.
- AC-101. Provider-side memory is not canonical project truth.
- AC-102. Grouped Undo compensates dictionary/memory revisions together.
- AC-103. Voice and chat share a run identifier and ordered history.
- AC-104. Switching modes preserves target, context, guide, provider and proposal.
- AC-105. Listening pauses during handoff; drafts are retained and never auto-sent.
- AC-106. Chat sends only through explicit submission.
- AC-107. Chat turns do not create speech/audio artifacts.
- AC-108. Voice/text turns have distinct activity labels within one conversation.
- AC-109. Tool policy, preview and Undo are identical across input modes.
- AC-110. Chat remains usable when microphone access is unavailable.
- AC-111. Addressed edits preserve earlier dictation source without a new capture session.
- AC-112. Positional/entity references resolve to stable nodes, blocks/ranges and exact revisions.
- AC-113. Ambiguous references require clarification before mutation.
- AC-114. Pre-execution instruction correction avoids unnecessary provider requests.
- AC-115. Server validation preserves protected ranges byte-for-byte or subtree-equivalently.
- AC-116. Review connects instructions, targets, activity and diff groups.
- AC-117. Independent groups can be excluded; prerequisite dependencies are enforced.
- AC-118. Selected groups apply atomically and share grouped Undo.
- AC-119. Updating canon from a scene creates a separate sourced proposal and Apply decision.
- AC-120. Long-session reconnect resumes ordered events without duplicate applied operations.
- AC-121. Project schema defines allowed structure kinds, not a hardcoded chapter allowlist.
- AC-122. Invalid nesting is rejected before a canonical tree mutation.
- AC-123. Different templates retain their own kinds/labels with compatible tools.
- AC-124. Structure revisions participate in stale validation.
- AC-125. New type keys have UI fallback without losing content, revisions or actions.
- AC-126. Exactly one valid semantic declaration precedes project tool calls.
- AC-127. Intent routing contains no verb, synonym or keyword allowlist.
- AC-128. Mode declaration exposes the policy-permitted catalog; answer/clarify cannot mutate.
- AC-129. Revising a proposal reads its native change set rather than old assistant prose.
- AC-130. Exact replacement checks verified anchors, expected text, base revision and protected ranges.
- AC-131. Hidden reasoning stays hidden; Markdown/activity/review use readable semantic presentation.
- AC-132. Desktop shows manuscript and ordered assistant transcript without route changes.
- AC-133. Panel and mode changes preserve node, selection, cursor, scroll and manual Undo.
- AC-134. Selected text can be attached while the author continues editing before Execute.
- AC-135. Unapplied previews have non-color labels/diff semantics and no autosave/Undo effect.
- AC-136. Overlapping manual edits stale the preview and never revert the author.
- AC-137. Review preserves ordered session context and returns to the same workspace.
- AC-138. Responsive presentation preserves current run and mounted editor state.
- AC-139. Docked typography uses the compact transcript/activity/meta scale.
- AC-140. Detailed review is a modal with unchanged URL and restored focus.
- AC-141. Fresh fully selected proposals support direct atomic Apply and grouped Undo.
- AC-142. Review shows diff, instruction, groups and exact target before canonical changes.
- AC-143. Context actions require a nonempty single-block selection and a verified Unicode anchor.
- AC-144. Dictionary links show current targets and retain readable snapshots after target deletion.
- AC-145. Notes support editing, resolving, reopening and deletion from the annotation panel.
- AC-146. Decorations are excluded from canonical JSON and manual editor Undo/Redo.
- AC-147. Manual edits save document revisions and mapped anchors in one transaction.
- AC-148. Unprovable ranges retain exact quotes as detached, independently of open/resolved lifecycle.
- AC-149. Legacy saves without anchor data neither delete notes nor guess new positions.
- AC-150. Conflict copies/duplicates retain annotations and enforce ownership/anchor checks.
- AC-151. Desktop panels and mobile sheets expose equivalent annotation actions.
- AC-152. Project-wide annotation search supports lifecycle/type/document filters.
- AC-153. Deep links open the right document, annotation and exact selection.
- AC-154. Resolved notes remain available and can reopen.
- AC-155. Another owner cannot access text/links/notes; logs omit their content.
- AC-156. Legacy projects and v1 archives preserve text/revisions without required new fields.
- AC-157. Native archive v2+ transfers annotations with atomic node/term remapping.
- AC-158. Broken references, anchors and future formats fail without partial imports.
- AC-159. Annotation migrations are additive and reader exports exclude author metadata.
- AC-160. Critical annotation logic targets at least 90% line coverage plus integration/browser checks.
- AC-161. Annotation reads stay inside the current owner/project without direct provider DB access.
- AC-162. At most 50 versioned fixes use server-owned offsets and stay staged before Apply.
- AC-163. Fix review shows note, exact before/after and independent resolution choices.
- AC-164. Apply atomically updates document, anchors, lifecycle and immutable history.
- AC-165. Partial overlaps/deletions detach deterministically without fuzzy reattachment.
- AC-166. Stale annotation/document versions or expected-text mismatch leave no partial writes.
- AC-167. Grouped Undo restores text/note snapshots; audit survives run-history deletion.
- AC-168. Multiple active/executing runs can coexist within a project.
- AC-169. Visible conversation selection is not a singleton execution lock.
- AC-170. Hiding/switching panels neither cancels work nor sends another run's draft.
- AC-171. New-chat composition shows inherited context/provider/guide before creating a run.
- AC-172. Conversation sections have textual accessible attention/working/recent status.
- AC-173. Drafts, attachments, targets and review state remain isolated per run.
- AC-174. Shared-document revision conflicts block Apply atomically and offer refresh.
- AC-175. Tablet/mobile switching returns to the same mounted manuscript state.
- AC-176. Conversation creation/switching is keyboard/screen-reader accessible with 44 px touch targets.
