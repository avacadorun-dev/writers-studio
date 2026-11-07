# Historical release 0.4.0: Canvas boards

The first card-first Canvas release added independent project boards, Markdown,
document and image cards, spatial groups, colored connections, pan/zoom,
multiselect, resizing, copy/paste, Undo/Redo, local recovery and server revisions.
Mobile supported read-only layout with navigation to linked documents.

Migration 023 adds Canvas tables/indexes without rewriting old document history.
Native archive v6 added complete board snapshots and resource-ID remapping while
retaining import of v1–v5. Later releases added drawing and expanded media/types.

Update through the backup-first deployment script. An older binary may ignore
new tables but cannot open/export the new board features. Preserve a consistent
database/object-storage recovery point before rollback. Historical coverage and
deployment notes are not evidence for the current publication candidate.
