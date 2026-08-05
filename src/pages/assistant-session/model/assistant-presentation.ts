import type { AssistantEvent } from "~entities/assistant";

export interface AssistantActivityItem {
  id: string;
  label: string;
  status: "working" | "done" | "failed";
}

export type AssistantNativeDestination =
  | {
      type: "project_export";
      projectId: string;
      format?: "docx" | "epub" | "pdf" | "markdown" | "txt";
      nodeIds?: string[];
      chapterTitles?: boolean;
    }
  | { type: "project_node"; projectId: string; nodeId: string }
  | {
      type: "node_versions";
      nodeId: string;
      versionId?: string;
      expectedCurrentRevision?: number;
    }
  | { type: "dictation_review"; sessionId: string };

export interface AssistantNativeAction {
  id: string;
  kind: "archive_node" | "restore_version" | "retranscribe" | "export";
  destination: AssistantNativeDestination;
}

export function assistantFailureMessage(errorCode?: string): string {
  switch (errorCode) {
    case "tool_limit_exceeded":
    case "provider_step_limit":
      return "Ассистент исчерпал безопасный лимит шагов. Уже подготовленные изменения не применены; повторите поручение или разделите его на части.";
    case "repeated_tool_failure":
      return "Одна из операций проекта несколько раз завершилась ошибкой. Ничего не применено; уточните проблемную часть поручения и повторите её.";
    case "mutation_not_staged":
      return "Ассистент завершил ответ, но не подготовил проверяемых изменений. Проект не изменён; отправьте поручение ещё раз.";
    case "provider_request_failed":
    case "provider_dialect_failed":
    case "provider_unavailable":
      return "Модель не смогла завершить ответ. Проект не изменён; можно повторить то же поручение.";
    case "provider_timeout":
      return "Модели не хватило времени на отдельный ответ. Проект не изменён; повторите поручение — большой текст будет подготовлен по частям.";
    case "long_form_incomplete":
      return "Модель подготовила только часть большого текста. Ничего не применено; проверьте сохранённый частичный результат.";
    case "context_unavailable":
    case "mcp_unavailable":
    case "change_set_failed":
      return "Не удалось подготовить безопасный набор изменений проекта. Ничего не применено; повторите поручение.";
    case "execution_interrupted":
      return "Выполнение прервалось из-за перезапуска приложения. Проект не изменён; это поручение можно отправить повторно.";
    default:
      return "Проект не изменён. Можно уточнить поручение и отправить его ещё раз.";
  }
}

export function assistantToolLabel(toolName: string): string {
  const exact: Record<string, string> = {
    "assistant.begin_turn": "Понимает поручение",
    "assistant.get_recent_change_set": "Сверяет предыдущее предложение",
    "project.get_overview": "Изучает проект",
    "project.get_structure": "Сверяет структуру и вложенность",
    "project.search": "Ищет точное место в материалах",
    "tree.list": "Просматривает дерево проекта",
    "tree.create_nodes_batch": "Готовит разделы вместе с содержимым",
    "document.get": "Читает текущий документ",
    "document.find_exact": "Находит точный фрагмент без ручного подсчёта",
    "document.find_exact_batch": "Находит точные фрагменты одним проходом",
    "document.get_range": "Проверяет точный фрагмент",
    "document.replace_range": "Готовит точную правку текста",
    "annotation.list": "Собирает заметки по проекту",
    "annotation.get": "Проверяет заметку и её историю",
    "annotation.propose_fixes": "Готовит исправления по заметкам",
    "document.append_chunk": "Пишет следующий фрагмент черновика",
    "document.prepare_replace_all": "Готовит связанную правку по проекту",
    "dictionary.add_batch": "Готовит пакетное обновление словаря",
    "project_memory.add_batch": "Готовит пакетное обновление памяти проекта",
    "tree.prepare_archive": "Передаёт архивацию в проект",
    "version.prepare_restore": "Передаёт восстановление в историю версий",
    "transcript.prepare_retranscribe": "Передаёт повторную обработку стенограммы",
    "export.prepare": "Передаёт создание файла в экспорт",
    "assistant_guide.get": "Сверяется с ASSISTANT.md",
    "assistant.get_capabilities": "Проверяет возможности рабочего пространства",
  };
  if (exact[toolName]) return exact[toolName];
  if (toolName.startsWith("document.") || toolName.startsWith("note.")) {
    return "Готовит изменение документа";
  }
  if (toolName.startsWith("annotation.")) return "Работает с заметками в рукописи";
  if (toolName.startsWith("dictionary.")) return "Работает со словарём";
  if (toolName.startsWith("project_memory.")) return "Работает с памятью проекта";
  if (toolName.startsWith("assistant_guide.")) return "Готовит изменение ASSISTANT.md";
  if (toolName.startsWith("tree.")) return "Работает со структурой проекта";
  if (toolName.startsWith("version.")) return "Проверяет историю версий";
  if (toolName.startsWith("transcript.") || toolName.startsWith("dictation.")) {
    return "Сверяет стенограмму";
  }
  if (toolName.startsWith("export.")) return "Готовит экспорт";
  return "Выполняет безопасный шаг";
}

export function buildAssistantNativeActions(events: AssistantEvent[]): AssistantNativeAction[] {
  const actions = new Map<string, { action: AssistantNativeAction; sequence: number }>();
  for (const event of events) {
    if (event.type !== "TOOL_CALL_RESULT") continue;
    const callId = stringValue(event.data.toolCallId);
    const result = recordValue(event.data.result);
    const flow = recordValue(result?.nativeFlow);
    const destination = nativeDestination(recordValue(flow?.destination));
    const kind = stringValue(flow?.kind);
    if (!callId || !destination || !isNativeActionKind(kind)) continue;
    actions.set(callId, {
      action: { id: callId, kind, destination },
      sequence: event.sequence,
    });
  }
  return [...actions.values()]
    .sort((left, right) => left.sequence - right.sequence)
    .slice(-4)
    .map(({ action }) => action);
}

function nativeDestination(
  value?: Record<string, unknown>,
): AssistantNativeDestination | undefined {
  const type = stringValue(value?.type);
  if (type === "project_export") {
    const projectId = stringValue(value?.projectId);
    const format = exportFormat(value?.format);
    const nodeIds = stringArrayValue(value?.nodeIds);
    const chapterTitles = booleanValue(value?.chapterTitles);
    return projectId
      ? {
          type,
          projectId,
          ...(format ? { format } : {}),
          ...(nodeIds ? { nodeIds } : {}),
          ...(chapterTitles === undefined ? {} : { chapterTitles }),
        }
      : undefined;
  }
  if (type === "project_node") {
    const projectId = stringValue(value?.projectId);
    const nodeId = stringValue(value?.nodeId);
    return projectId && nodeId ? { type, projectId, nodeId } : undefined;
  }
  if (type === "node_versions") {
    const nodeId = stringValue(value?.nodeId);
    const versionId = stringValue(value?.versionId);
    const expectedCurrentRevision = integerValue(value?.expectedCurrentRevision);
    return nodeId
      ? {
          type,
          nodeId,
          ...(versionId ? { versionId } : {}),
          ...(expectedCurrentRevision === undefined ? {} : { expectedCurrentRevision }),
        }
      : undefined;
  }
  if (type === "dictation_review") {
    const sessionId = stringValue(value?.sessionId);
    return sessionId ? { type, sessionId } : undefined;
  }
  return undefined;
}

function isNativeActionKind(value: string): value is AssistantNativeAction["kind"] {
  return (
    value === "archive_node" ||
    value === "restore_version" ||
    value === "retranscribe" ||
    value === "export"
  );
}

function recordValue(value: unknown): Record<string, unknown> | undefined {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function stringArrayValue(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const items = value.map(stringValue).filter(Boolean);
  return items.length === value.length ? items : undefined;
}

function booleanValue(value: unknown): boolean | undefined {
  return typeof value === "boolean" ? value : undefined;
}

function integerValue(value: unknown): number | undefined {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? value : undefined;
}

function exportFormat(value: unknown): "docx" | "epub" | "pdf" | "markdown" | "txt" | undefined {
  return value === "docx" ||
    value === "epub" ||
    value === "pdf" ||
    value === "markdown" ||
    value === "txt"
    ? value
    : undefined;
}

export function buildAssistantActivity(events: AssistantEvent[]): AssistantActivityItem[] {
  const calls = new Map<
    string,
    { id: string; toolName: string; sequence: number; status: AssistantActivityItem["status"] }
  >();
  for (const event of events) {
    const callId = String(event.data.toolCallId ?? "");
    if (!callId) continue;
    if (event.type === "TOOL_CALL_START") {
      calls.set(callId, {
        id: callId,
        toolName: String(event.data.toolName ?? ""),
        sequence: event.sequence,
        status: "working",
      });
    }
    if (event.type === "TOOL_CALL_END") {
      const previous = calls.get(callId);
      if (previous) {
        calls.set(callId, {
          ...previous,
          status: event.data.isError === true ? "failed" : "done",
        });
      }
    }
  }
  return [...calls.values()]
    .sort((left, right) => left.sequence - right.sequence)
    .slice(-5)
    .map((call) => ({
      id: call.id,
      label: assistantToolLabel(call.toolName),
      status: call.status,
    }));
}
