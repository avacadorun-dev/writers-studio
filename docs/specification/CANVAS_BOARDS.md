# Canvas boards

The original card-first release was 0.4.0. Later Go/web releases add drawing,
media, nested boards, typed graphs and assistant review. This reference describes
the frozen edition and distinguishes those later extensions from the first release.

## Model and interaction

A project has independent boards backed by React Flow and JSON Canvas-compatible
snapshots. Markdown text, document/media references, links, spatial groups and edges
share a viewport. Namespaced `x-writers-studio` metadata preserves resource/type
information; unknown compatible extension fields survive codec round-trips.

Desktop supports pan/zoom, grid, lasso/multiselect, resize, z-order, reconnection,
labels/colors, fit, copy/cut/paste, duplication and 100-operation history. Copies
remap IDs and retain only edges whose endpoints are selected. Moving a spatial
group moves enclosed cards; deleting the group preserves them. Mobile permits
viewing, navigation and pan/zoom without changing board layout.

Later whiteboard capabilities include pen/highlighter, eraser, shapes, frames,
media/link cards, nested boards, portable `.canvas`, image output, templates,
typed card/relation schemas, timeline/swimlane/dependency projections, graph
analysis and manual play-through. Projections do not rewrite canonical coordinates.
Assistant Canvas mutations use staged review/ghost previews before Apply.

## Persistence, validation and transfer

`canvases` and immutable `canvas_revisions` store canonical snapshots. Stale
`baseRevision` writes preserve an alternate conflict board rather than overwriting.
Local recovery saves after approximately 300 ms; server autosave waits about 1.5 s
of idle time, with a 15 s maximum attempt interval. Reconnect retries unsynced work.

Routes include project board list/create, board get/patch/delete, duplicate,
restore and revision writes. Reads batch authorized resource previews. Validation
checks ownership, resource project membership, unique IDs, geometry and dangling
edges. Limits include 2,000 nodes, 4,000 edges and 100,000 Unicode code points per
text card. Check the current validator for additional media/schema bounds.

Native archives have included boards/revisions since v6. Current v8 also transfers
typed schemas/media. Import remaps board/document/media IDs and namespaced/local
resource references atomically; older archives can create projects without boards.

## Acceptance references

- CAN-01: create/open/rename/duplicate/trash/restore independent boards.
- CAN-02: cards, edges and extension metadata survive reload.
- CAN-03: deterministic copy/paste remap, spatial movement and Undo/Redo.
- CAN-04: offline recovery and stale conflict copies preserve both versions.
- CAN-05: mobile viewing leaves layout unchanged and opens linked documents.
- CAN-06: native round-trips preserve revisions and remap resources.
- CAN-07: a 500-card/1,000-edge board uses visible-only rendering.
- CAN-08: critical Canvas logic targets at least 90% coverage.

Run `bun run test:canvas:coverage`; historical coverage values are not current evidence.
