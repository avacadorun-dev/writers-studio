import { describe, expect, test } from "bun:test";

import type { AssistantEvent } from "~entities/assistant";

import {
  assistantFailureMessage,
  assistantToolLabel,
  buildAssistantActivity,
  buildAssistantNativeActions,
} from "./assistant-presentation";

function event(sequence: number, type: string, data: Record<string, unknown>): AssistantEvent {
  return {
    id: `event-${sequence}`,
    runId: "run",
    sequence,
    type,
    data,
    createdAt: new Date(0).toISOString(),
  };
}

describe("assistant activity presentation", () => {
  test("translates technical tools into calm author-facing activity", () => {
    expect(assistantToolLabel("assistant.begin_turn")).toBe("Понимает поручение");
    expect(assistantToolLabel("document.replace_range")).toBe("Готовит точную правку текста");
    expect(assistantToolLabel("document.append_chunk")).toBe("Пишет следующий фрагмент черновика");
    expect(assistantToolLabel("annotation.list")).toBe("Собирает заметки по проекту");
    expect(assistantToolLabel("annotation.propose_fixes")).toBe("Готовит исправления по заметкам");
    expect(assistantToolLabel("dictionary.add")).toBe("Работает со словарём");
    expect(assistantToolLabel("dictionary.add_batch")).toBe("Готовит пакетное обновление словаря");
    expect(assistantToolLabel("tree.create_nodes_batch")).toBe(
      "Готовит разделы вместе с содержимым",
    );
    expect(assistantToolLabel("unknown.private_tool")).toBe("Выполняет безопасный шаг");
  });

  test("explains terminal errors instead of hiding them behind a generic retry", () => {
    expect(assistantFailureMessage("tool_limit_exceeded")).toContain("лимит шагов");
    expect(assistantFailureMessage("repeated_tool_failure")).toContain("операций проекта");
    expect(assistantFailureMessage("provider_request_failed")).toContain("Модель");
    expect(assistantFailureMessage("provider_timeout")).toContain("по частям");
    expect(assistantFailureMessage("long_form_incomplete")).toContain("часть большого текста");
    expect(assistantFailureMessage("execution_interrupted")).toContain("перезапуска");
    expect(assistantFailureMessage()).toContain("Проект не изменён");
  });

  test("pairs durable tool events and preserves failure state", () => {
    const activity = buildAssistantActivity([
      event(1, "TOOL_CALL_START", {
        toolCallId: "read",
        toolName: "document.get",
      }),
      event(2, "TOOL_CALL_END", { toolCallId: "read", isError: false }),
      event(3, "TOOL_CALL_START", {
        toolCallId: "edit",
        toolName: "document.replace_range",
      }),
      event(4, "TOOL_CALL_END", { toolCallId: "edit", isError: true }),
    ]);
    expect(activity).toEqual([
      { id: "read", label: "Читает текущий документ", status: "done" },
      { id: "edit", label: "Готовит точную правку текста", status: "failed" },
    ]);
  });

  test("keeps only the latest five visible steps", () => {
    const events = Array.from({ length: 7 }, (_, index) =>
      event(index + 1, "TOOL_CALL_START", {
        toolCallId: `call-${index}`,
        toolName: "project.get_overview",
      }),
    );
    expect(buildAssistantActivity(events)).toHaveLength(5);
    expect(buildAssistantActivity(events)[0]?.id).toBe("call-2");
  });

  test("builds only server-authored native destinations and ignores arbitrary links", () => {
    const actions = buildAssistantNativeActions([
      event(1, "TOOL_CALL_RESULT", {
        toolCallId: "export",
        result: {
          nativeFlow: {
            kind: "export",
            destination: {
              type: "project_export",
              projectId: "project-1",
              format: "markdown",
              nodeIds: ["node-1", "node-2"],
              chapterTitles: false,
            },
          },
        },
      }),
      event(2, "TOOL_CALL_RESULT", {
        toolCallId: "version",
        result: {
          nativeFlow: {
            kind: "restore_version",
            destination: {
              type: "node_versions",
              nodeId: "node-1",
              versionId: "version-1",
              expectedCurrentRevision: 4,
            },
          },
        },
      }),
      event(3, "TOOL_CALL_RESULT", {
        toolCallId: "unsafe",
        result: {
          nativeFlow: {
            kind: "export",
            destination: { type: "external_url", url: "https://example.invalid" },
          },
        },
      }),
      event(4, "TOOL_CALL_RESULT", {
        toolCallId: "malformed",
        result: { nativeFlow: { kind: "archive_node", destination: { type: "project_node" } } },
      }),
    ]);
    expect(actions).toEqual([
      {
        id: "export",
        kind: "export",
        destination: {
          type: "project_export",
          projectId: "project-1",
          format: "markdown",
          nodeIds: ["node-1", "node-2"],
          chapterTitles: false,
        },
      },
      {
        id: "version",
        kind: "restore_version",
        destination: {
          type: "node_versions",
          nodeId: "node-1",
          versionId: "version-1",
          expectedCurrentRevision: 4,
        },
      },
    ]);
  });

  test("supports native archive and retranscription handoffs", () => {
    expect(
      buildAssistantNativeActions([
        event(1, "TOOL_CALL_RESULT", {
          toolCallId: "archive",
          result: {
            nativeFlow: {
              kind: "archive_node",
              destination: {
                type: "project_node",
                projectId: "project-1",
                nodeId: "node-2",
              },
            },
          },
        }),
        event(2, "TOOL_CALL_RESULT", {
          toolCallId: "retranscribe",
          result: {
            nativeFlow: {
              kind: "retranscribe",
              destination: { type: "dictation_review", sessionId: "dictation-1" },
            },
          },
        }),
      ]),
    ).toHaveLength(2);
  });
});
