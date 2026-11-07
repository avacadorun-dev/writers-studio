# Documentation

These English documents describe the frozen Go/web edition. They consolidate
older planning and implementation notes into a public reference. They are not a
roadmap for the independently developed desktop application. The application UI
and synthetic fixture text may still use Russian.

## Start here

- [Root README](../README.md): features, status and quick start.
- [Product brief](../PRODUCT_BRIEF.md): goals and constraints.
- [Configuration](CONFIGURATION.md) and [deployment](DEPLOYMENT.md).
- [Contributor guide](../CONTRIBUTING.md) and [security policy](../SECURITY.md).
- [Built-in user help](../content/docs/index.mdx), served at `/docs`.

## Architecture decisions

- [Architecture overview](architecture/ARC42.md).
- [ADR-001: speech recognition](adr/ADR-001-speech-to-text.md).
- [ADR-002: editor engine](adr/ADR-002-editor-engine.md).
- [ADR-003: assistant and project MCP](adr/ADR-003-assistant-mcp.md).
- [ADR-007: continuous assistant workspace](adr/ADR-007-assistant-continuous-workspace.md).
- [ADR-008: remote MCP gateway](adr/ADR-008-remote-mcp-gateway.md).

## Specifications

- [Workflows and acceptance](specification/WORKFLOWS.md).
- [Assistant mode](specification/ASSISTANT_MODE.md).
- [Rich document editor](specification/RICH_DOCUMENT_EDITOR.md).
- [Manuscript annotations](specification/MANUSCRIPT_ANNOTATIONS.md).
- [Project navigation and transfer](specification/PROJECT_NAVIGATION_AND_TRANSFER.md).
- [Markdown tree migration](specification/MARKDOWN_TREE_MIGRATION.md).
- [Canvas boards](specification/CANVAS_BOARDS.md).
- [Design reference](design/DESIGN_HANDOFF.md).

## Verification and publication

- [Verification](implementation/VERIFICATION_REPORT.md).
- [Traceability](implementation/TRACEABILITY.md).
- Historical changes: [0.2.0](implementation/RELEASE_0.2.0.md),
  [0.4.0](implementation/RELEASE_0.4.0.md), [0.7.0](implementation/RELEASE_0.7.0.md).
- [Publication](PUBLICATION.md): repository hygiene and history verification.

Runtime code, migrations and tests resolve conflicts with obsolete planning
claims. Version numbers for document JSON and `.wstudio` archives are separate.
