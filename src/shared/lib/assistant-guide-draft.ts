export interface AssistantGuideDraft {
  projectId: string;
  baseRevision: number;
  content: string;
  updatedAt: number;
}

const keyPrefix = "writers-studio-assistant-guide-draft:";
const maximumGuideCharacters = 24_000;

export function loadAssistantGuideDraft(projectId: string): AssistantGuideDraft | undefined {
  try {
    const value = JSON.parse(localStorage.getItem(draftKey(projectId)) ?? "null");
    if (
      !isRecord(value) ||
      value.projectId !== projectId ||
      !Number.isInteger(value.baseRevision) ||
      (value.baseRevision as number) < 0 ||
      typeof value.content !== "string" ||
      [...value.content].length > maximumGuideCharacters ||
      typeof value.updatedAt !== "number" ||
      !Number.isFinite(value.updatedAt)
    ) {
      return undefined;
    }
    return {
      projectId,
      baseRevision: value.baseRevision as number,
      content: value.content,
      updatedAt: value.updatedAt,
    };
  } catch {
    return undefined;
  }
}

export function saveAssistantGuideDraft(
  projectId: string,
  baseRevision: number,
  content: string,
): AssistantGuideDraft {
  const draft = { projectId, baseRevision, content, updatedAt: Date.now() };
  localStorage.setItem(draftKey(projectId), JSON.stringify(draft));
  return draft;
}

export function clearAssistantGuideDraft(projectId: string): void {
  try {
    localStorage.removeItem(draftKey(projectId));
  } catch {
    // The durable server revision remains the source of truth when storage is unavailable.
  }
}

function draftKey(projectId: string): string {
  return `${keyPrefix}${projectId}`;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
