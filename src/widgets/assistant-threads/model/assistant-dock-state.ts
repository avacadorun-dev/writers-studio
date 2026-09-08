export type AssistantDockView = "session" | "threads" | "new";

export interface AssistantDockState {
  runId?: string;
  view: AssistantDockView;
}

export function assistantDockStateKey(projectId: string): string {
  return `writers-studio:assistant-dock:${projectId}`;
}

export function loadAssistantDockState(projectId: string): AssistantDockState {
  if (typeof localStorage === "undefined") return { view: "new" };
  try {
    const value = JSON.parse(localStorage.getItem(assistantDockStateKey(projectId)) ?? "null");
    if (!value || !["session", "threads", "new"].includes(value.view)) return { view: "new" };
    const runId = typeof value.runId === "string" && value.runId.trim() ? value.runId : undefined;
    return { runId, view: runId || value.view !== "session" ? value.view : "threads" };
  } catch {
    return { view: "new" };
  }
}

export function saveAssistantDockState(projectId: string, state: AssistantDockState): void {
  if (typeof localStorage === "undefined") return;
  localStorage.setItem(assistantDockStateKey(projectId), JSON.stringify(state));
}
