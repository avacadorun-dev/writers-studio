import type { AssistantRun } from "~entities/assistant";

export type AssistantThreadBucket = "attention" | "working" | "recent";

export function assistantThreadTitle(run: AssistantRun): string {
  const explicit = run.title?.trim();
  if (explicit && explicit !== "Новый чат") return explicit;
  const instruction = run.latestTurn?.instruction?.trim();
  if (instruction) return compactThreadTitle(instruction);
  if (run.inputMode === "mcp") return `Внешний агент · ${run.agent?.clientName ?? "MCP"}`;
  return run.inputMode === "voice" ? "Голосовое поручение" : "Новый чат";
}

export function compactThreadTitle(value: string, maxLength = 58): string {
  const normalized = value.trim().replace(/\s+/g, " ");
  if (normalized.length <= maxLength) return normalized;
  return `${normalized.slice(0, Math.max(1, maxLength - 1)).trim()}…`;
}

export function assistantThreadBucket(run: AssistantRun): AssistantThreadBucket {
  const currentTurnIsStale =
    run.latestChangeSet?.state === "stale" && run.latestChangeSet.turnId === run.latestTurn?.id;
  if (
    run.unread ||
    run.status === "awaiting_review" ||
    run.status === "failed" ||
    currentTurnIsStale ||
    (run.errorCode === "stale_change_set" && run.latestTurn?.errorCode === "stale_change_set")
  ) {
    return "attention";
  }
  if (run.status === "working") return "working";
  return "recent";
}

export function assistantThreadStatus(run: AssistantRun): string {
  if (
    (run.latestChangeSet?.state === "stale" && run.latestChangeSet.turnId === run.latestTurn?.id) ||
    (run.errorCode === "stale_change_set" && run.latestTurn?.errorCode === "stale_change_set")
  ) {
    return "ОБНОВИТЬ";
  }
  if (run.status === "awaiting_review") return "ПРОВЕРИТЬ";
  if (run.status === "working") return "В РАБОТЕ";
  if (run.status === "failed") return "ОШИБКА";
  if (run.status === "cancelled") return "ОСТАНОВЛЕН";
  if (run.status === "completed") return "ГОТОВО";
  return "ОТКРЫТ";
}

export function assistantThreadUpdatedLabel(updatedAt: string, now = Date.now()): string {
  const timestamp = Date.parse(updatedAt);
  if (!Number.isFinite(timestamp)) return "недавно";
  const elapsed = Math.max(0, now - timestamp);
  if (elapsed < 60_000) return "только что";
  const minutes = Math.floor(elapsed / 60_000);
  if (minutes < 60) return `${minutes} мин`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} ч`;
  const days = Math.floor(hours / 24);
  return `${days} дн`;
}

export function assistantThreadCountLabel(count: number): string {
  const absolute = Math.abs(count);
  const lastTwo = absolute % 100;
  const last = absolute % 10;
  if (lastTwo >= 11 && lastTwo <= 14) return `${count} чатов`;
  if (last === 1) return `${count} чат`;
  if (last >= 2 && last <= 4) return `${count} чата`;
  return `${count} чатов`;
}

export function groupAssistantThreads(
  runs: AssistantRun[],
): Record<AssistantThreadBucket, AssistantRun[]> {
  const groups: Record<AssistantThreadBucket, AssistantRun[]> = {
    attention: [],
    working: [],
    recent: [],
  };
  for (const run of runs) groups[assistantThreadBucket(run)].push(run);
  return groups;
}
