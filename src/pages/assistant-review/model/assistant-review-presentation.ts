import type { AssistantChangeOperation, AssistantResolvedTarget } from "~entities/assistant";

const groupLabels: Record<string, string> = {
  document: "Текст и структура",
  dictionary: "Словарь",
  memory: "Память проекта",
  guide: "Руководство ассистента",
  version: "Именованная версия",
};

const operationLabels: Record<string, string> = {
  "document.replace_range": "Точная правка текста",
  "annotation.propose_fixes": "Исправления по заметкам",
  "document.append": "Продолжение документа",
  "document.append_chunk": "Большой фрагмент документа",
  "document.insert": "Вставка в документ",
  "document.set_title": "Новый заголовок",
  "document.apply_marks": "Форматирование текста",
  "tree.create_node": "Новый материал проекта",
  "tree.rename_node": "Переименование материала",
  "tree.move_node": "Изменение структуры",
  "dictionary.add": "Новая запись словаря",
  "dictionary.update": "Обновление словаря",
  "project_memory.add": "Новая запись памяти",
  "project_memory.update": "Обновление памяти проекта",
  "assistant_guide.propose_create": "Новое руководство ассистента",
  "assistant_guide.propose_update": "Обновление руководства ассистента",
};

export interface ExactTextEdit {
  blockId: string;
  start: number;
  end: number;
  before: string;
  after: string;
}

export function operationLabel(toolName: string): string {
  if (operationLabels[toolName]) return operationLabels[toolName];
  if (toolName.startsWith("document.") || toolName.startsWith("note.")) {
    return "Изменение документа";
  }
  if (toolName.startsWith("annotation.")) return "Изменение по заметкам";
  if (toolName.startsWith("tree.")) return "Изменение структуры";
  if (toolName.startsWith("dictionary.")) return "Изменение словаря";
  if (toolName.startsWith("project_memory.")) return "Изменение памяти проекта";
  if (toolName.startsWith("assistant_guide.")) return "Изменение руководства";
  return "Предложенное изменение";
}

export function changeGroupLabel(operations: AssistantChangeOperation[], group: string): string {
  const fixedLabel = groupLabels[group];
  if (fixedLabel) return fixedLabel;
  const operation = operations.find((item) => item.groupKey === group);
  const title = operation?.beforeSnapshot?.title;
  if (title) return `Текст · ${title}`;
  const trace = operation?.trace;
  if (trace && "targetTitle" in trace && typeof trace.targetTitle === "string") {
    return `Текст · ${trace.targetTitle}`;
  }
  if (group.startsWith("document:")) return "Отдельный документ";
  return "Связанное изменение";
}

export function withRequiredChangeGroups(
  operations: AssistantChangeOperation[],
  selectedGroups: Iterable<string>,
): Set<string> {
  const available = new Set(operations.map((operation) => operation.groupKey));
  const dependencies = new Map<string, Set<string>>();
  for (const operation of operations) {
    const current = dependencies.get(operation.groupKey) ?? new Set<string>();
    for (const dependency of operation.dependencies ?? []) {
      if (available.has(dependency)) current.add(dependency);
    }
    dependencies.set(operation.groupKey, current);
  }
  const selected = new Set(Array.from(selectedGroups).filter((group) => available.has(group)));
  const pending = [...selected];
  while (pending.length > 0) {
    const group = pending.pop();
    if (!group) continue;
    for (const dependency of dependencies.get(group) ?? []) {
      if (selected.has(dependency)) continue;
      selected.add(dependency);
      pending.push(dependency);
    }
  }
  return selected;
}

export function selectedChangeGroupDependents(
  operations: AssistantChangeOperation[],
  group: string,
  selectedGroups: ReadonlySet<string>,
): string[] {
  const result = new Set<string>();
  let changed = true;
  while (changed) {
    changed = false;
    for (const operation of operations) {
      if (!selectedGroups.has(operation.groupKey) || result.has(operation.groupKey)) continue;
      if (
        (operation.dependencies ?? []).some(
          (dependency) => dependency === group || result.has(dependency),
        )
      ) {
        result.add(operation.groupKey);
        changed = true;
      }
    }
  }
  result.delete(group);
  return [...result];
}

export function exactTextEdits(operation: AssistantChangeOperation): ExactTextEdit[] {
  if (
    operation.toolName !== "document.replace_range" &&
    operation.toolName !== "annotation.propose_fixes"
  )
    return [];
  const edits = operation.arguments.edits;
  if (!Array.isArray(edits)) return [];
  return edits.flatMap((value) => {
    if (!value || typeof value !== "object") return [];
    const edit = value as Record<string, unknown>;
    if (
      typeof edit.blockId !== "string" ||
      typeof edit.start !== "number" ||
      typeof edit.end !== "number" ||
      typeof edit.expectedText !== "string" ||
      typeof edit.replacement !== "string"
    ) {
      return [];
    }
    return [
      {
        blockId: edit.blockId,
        start: edit.start,
        end: edit.end,
        before: edit.expectedText,
        after: edit.replacement,
      },
    ];
  });
}

export interface AnnotationFixPreview {
  annotationId: string;
  note: string;
  originalText: string;
  replacement: string;
  resolve: boolean;
}

export function annotationFixes(operation: AssistantChangeOperation): AnnotationFixPreview[] {
  if (operation.toolName !== "annotation.propose_fixes") return [];
  const values = operation.arguments.annotationFixes;
  if (!Array.isArray(values)) return [];
  return values.flatMap((value) => {
    if (!value || typeof value !== "object") return [];
    const fix = value as Record<string, unknown>;
    if (
      typeof fix.annotationId !== "string" ||
      typeof fix.note !== "string" ||
      typeof fix.originalText !== "string" ||
      typeof fix.replacement !== "string" ||
      typeof fix.resolve !== "boolean"
    )
      return [];
    return [fix as unknown as AnnotationFixPreview];
  });
}

export function resolvedTarget(
  operation: AssistantChangeOperation,
): AssistantResolvedTarget | undefined {
  const trace = operation.trace;
  if (!trace) return undefined;
  if (isResolvedTarget(trace)) return trace;
  if ("resolvedTarget" in trace && isResolvedTarget(trace.resolvedTarget)) {
    return trace.resolvedTarget;
  }
  return undefined;
}

function isResolvedTarget(value: unknown): value is AssistantResolvedTarget {
  if (!value || typeof value !== "object") return false;
  const target = value as Partial<AssistantResolvedTarget>;
  return (
    typeof target.nodeId === "string" &&
    typeof target.sourcePhrase === "string" &&
    Array.isArray(target.ranges) &&
    Array.isArray(target.protectedRanges)
  );
}

export function snapshotText(value: unknown): string {
  if (!value || typeof value !== "object") return "";
  const record = value as Record<string, unknown>;
  if (typeof record.plainText === "string") return record.plainText;
  if (typeof record.statement === "string") return record.statement;
  if (typeof record.canonical === "string") return record.canonical;
  if (typeof record.content === "string") return record.content;
  if (record.content && typeof record.content === "object" && !Array.isArray(record.content)) {
    return snapshotText(record.content);
  }
  const own = typeof record.text === "string" ? record.text : "";
  if (!Array.isArray(record.content)) return own;
  const separator =
    record.type === "doc"
      ? "\n\n"
      : record.type === "bulletList" || record.type === "orderedList"
        ? "\n"
        : "";
  const children = record.content.map(snapshotText).filter(Boolean).join(separator);
  return own + children;
}
