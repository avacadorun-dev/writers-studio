import type {
  DocumentNode,
  ExportPreset,
  ExportProfile,
  ProjectStructure,
} from "~entities/project";

export interface ExportQueueItem {
  id: string;
  title: string;
  kind: string;
  breadcrumb: string;
  note: boolean;
}

export type PublicationPreviewPage =
  | { id: "cover"; kind: "cover"; label: string }
  | { id: "title"; kind: "title"; label: string }
  | { id: "toc"; kind: "toc"; label: string }
  | { id: `document:${string}`; kind: "document"; label: string; documentId: string };

export function buildPublicationPreviewPages(
  profile: Pick<ExportProfile, "titlePage" | "tableOfContents">,
  hasCover: boolean,
  documents: Array<{ id: string; title: string }>,
): PublicationPreviewPage[] {
  const pages: PublicationPreviewPage[] = [];
  if (hasCover) pages.push({ id: "cover", kind: "cover", label: "Обложка" });
  if (profile.titlePage) pages.push({ id: "title", kind: "title", label: "Титульная страница" });
  if (profile.tableOfContents) pages.push({ id: "toc", kind: "toc", label: "Оглавление" });
  for (const [index, document] of documents.entries()) {
    pages.push({
      id: `document:${document.id}`,
      kind: "document",
      label: `${index + 1}. ${document.title}`,
      documentId: document.id,
    });
  }
  return pages;
}

export function buildExportQueue(
  nodes: DocumentNode[],
  structure: ProjectStructure,
): ExportQueueItem[] {
  const active = nodes
    .filter((node) => !node.trashedAt)
    .toSorted(
      (left, right) =>
        left.position - right.position || left.createdAt.localeCompare(right.createdAt),
    );
  const byParent = new Map<string | undefined, DocumentNode[]>();
  for (const node of active) {
    const children = byParent.get(node.parentId) ?? [];
    children.push(node);
    byParent.set(node.parentId, children);
  }
  const contentKinds = new Set(
    structure.nodeTypes.filter((type) => type.contentBearing).map((type) => type.key),
  );
  const result: ExportQueueItem[] = [];
  const visited = new Set<string>();
  const visit = (node: DocumentNode, parents: string[]) => {
    if (visited.has(node.id)) return;
    visited.add(node.id);
    if (contentKinds.has(node.kind) && node.currentRevision > 0) {
      result.push({
        id: node.id,
        title: node.title,
        kind: node.kind,
        breadcrumb: [...parents, node.title].join(" / "),
        note: node.kind === "note",
      });
    }
    for (const child of byParent.get(node.id) ?? []) visit(child, [...parents, node.title]);
  };
  for (const root of byParent.get(undefined) ?? []) visit(root, []);
  for (const node of active) visit(node, []);
  return result;
}

export function reorderQueue(ids: string[], activeId: string, overId: string): string[] {
  const from = ids.indexOf(activeId);
  const to = ids.indexOf(overId);
  if (from < 0 || to < 0 || from === to) return ids;
  const next = [...ids];
  const [moved] = next.splice(from, 1);
  if (moved) next.splice(to, 0, moved);
  return next;
}

export function applyExportPreset(
  profile: ExportProfile,
  preset: Exclude<ExportPreset, "custom">,
): ExportProfile {
  const styles: Record<typeof preset, ExportProfile["style"]> = {
    book: {
      preset: "book",
      fontFamily: "literata",
      fontSizePt: 11,
      lineHeight: 1.5,
      paragraphMode: "indent",
      firstLineIndentEm: 1.25,
      paragraphSpaceEm: 0,
      textAlign: "justify",
      pageSize: "a5",
      marginMm: 20,
      sceneSeparator: "asterisks",
    },
    modern: {
      preset: "modern",
      fontFamily: "source-serif",
      fontSizePt: 11,
      lineHeight: 1.55,
      paragraphMode: "spaced",
      firstLineIndentEm: 0,
      paragraphSpaceEm: 0.75,
      textAlign: "left",
      pageSize: "a5",
      marginMm: 20,
      sceneSeparator: "dots",
    },
    manuscript: {
      preset: "manuscript",
      fontFamily: "ibm-plex-mono",
      fontSizePt: 12,
      lineHeight: 2,
      paragraphMode: "spaced",
      firstLineIndentEm: 0,
      paragraphSpaceEm: 0.5,
      textAlign: "left",
      pageSize: "a4",
      marginMm: 25,
      sceneSeparator: "asterisks",
    },
  };
  return { ...profile, style: styles[preset] };
}
