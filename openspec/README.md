# Writer's Studio specifications

These OpenSpec documents describe the externally observable behavior of the Go/web
edition. The architecture and implementation decisions are documented in the
[arc42 reference](../docs/architecture/ARC42.md).

| Capability | Scope |
| --- | --- |
| [Interface language](specs/interface-language/spec.md) | Language selection, browser preferences, and fallback |
| [Projects](specs/projects/spec.md) | Library, document tree, project lifecycle, and navigation |
| [Manuscript](specs/manuscript/spec.md) | Rich editing, revisions, search, and replacement |
| [Dictation](specs/dictation/spec.md) | Capture, recognition, recovery, review, and transfer |
| [Annotations](specs/annotations/spec.md) | Project dictionary, exact anchors, and editorial notes |
| [Assistant](specs/assistant/spec.md) | Scoped tools, conversation state, staged review, and Undo |
| [Canvas](specs/canvas/spec.md) | Visual boards, recovery, and transfer |
| [Project transfer](specs/project-transfer/spec.md) | Reader formats, native archives, and Markdown trees |

Validate the capability specifications with the [OpenSpec CLI](https://github.com/Fission-AI/OpenSpec).

```sh
openspec validate --specs --strict
```
