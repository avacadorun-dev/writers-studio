# Product brief: Writer's Studio

## Status and intent

This is the public product reference for the frozen Go/web edition. Work is moving
to a separate desktop application; the ideas below do not promise future releases.
Earlier interview plans, monetization options and unvalidated roadmap proposals
are historical research rather than an active development plan.

Writer's Studio helps authors turn spoken or written ideas into structured,
reviewable long-form work. It combines capture, editing, source verification,
visual planning and controlled assistant actions in one self-hosted workspace.

## Problem and audience

Novelists, screenwriters, nonfiction authors and narrative/game designers often
split their work across recordings, transcription tools, notes and manuscript
editors. Moving text loses context and provenance. Automated rewrites can replace
intentional choices, and a network failure can lose a capture session.

The primary user needs to speak freely, check uncertain words against the audio,
organize the resulting material and retain ownership of every editorial decision.
Researchers, journalists and users who find voice input easier can use the same
capture and review model. Real-time collaborative editing is outside this edition.

## Product principles

1. The manuscript is the central work product. Voice capture and the assistant
   support it without replacing the editor.
2. Preserve original sources. Audio, source transcripts, editable drafts and
   document revisions are independent.
3. Separate capture from recognition. Local recording can continue when live
   transcription or the network fails.
4. Acknowledge durable work honestly. Local recovery and server persistence have
   different statuses; an unacknowledged write is not presented as saved.
5. Keep the author in control. Assistant mutations are staged and reviewable;
   irreversible or sensitive actions use explicit native confirmation.
6. Make projects portable. Reader exports, Markdown trees and `.wstudio` serve
   different purposes, with no account credentials in project transfers.
7. Disclose external processing and provider identity. Self-hosting does not imply
   that third-party speech/model providers process data locally.

## Main journey

Create a project, choose a structural template, add or open a document, then type
or start dictation. Recording stores local chunks while live text gives feedback.
At completion, review the transcript, listen to uncertain ranges, correct the
editable draft and transfer it at an explicit manuscript destination.

Continue editing with normal formatting, versions, dictionary links, annotations,
search and replacement. Use Canvas for spatial research, story planning or typed
game-design graphs. Ask the assistant to examine selected context and prepare a
change set; compare, apply, reject or undo it. Export a reader format for sharing
or an entire project archive for transfer.

## Functional boundaries

| Area | Responsibilities |
| --- | --- |
| Project library | Create, import, archive, trash, restore and explicitly purge |
| Structure | Schema-defined hierarchy, content-bearing nodes and ordered navigation |
| Manuscript | Rich text, autosave, recovery, provenance, annotations and versions |
| Dictation | Preflight, recording, pause/resume, live text, commands and finalization |
| Review | Separate source/draft, audio ranges, explicit transfer and late-result diff |
| Dictionary | Canonical terms, variants and provider hints; no silent historical rewrite |
| Canvas | Spatial documents/media, drawing, typed cards, views, analysis and revisions |
| Assistant | Voice/chat runs, scoped tools, guide/memory, staged exact operations |
| Export | Snapshot-based reader output, Markdown tree and native archive portability |
| Operations | Authentication, owner scope, backups, retention and recovery |

The assistant guide, dictionary and memory are different stores. A guide defines
writing preferences, dictionary entries define canonical spellings, and typed
memory records describe project facts with provenance. The assistant cannot use
project content to expand its own permissions or read provider credentials.

## Success and quality criteria

A complete capture-to-manuscript session must survive refresh, interrupted uploads,
provider failures and late recognition without losing confirmed edits. Users must
understand what is local, what is saved remotely, what is only a proposal and what
will be sent to an external provider.

Useful historical research measures include time from capture to a usable draft,
correction effort, recovery success, successful exports and user understanding of
Apply/Undo. These measures are product hypotheses, not evidence that telemetry is
collected. Operational events must omit manuscript/prompt/audio content.

## Constraints and risks

Browser microphone APIs require a secure context. Storage quotas, permission
changes and explicit site-data deletion limit browser recovery. Speech quality
varies with language, microphone and provider. Long model tasks need bounded
calls, durable staging and explicit partial-result states. Self-hosting requires
ongoing dependency maintenance and tested backups, which this frozen repository
does not guarantee on behalf of operators.

See the [architecture](docs/architecture/ARC42.md) and
[workflow reference](docs/specification/WORKFLOWS.md) for detailed contracts.
