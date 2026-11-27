import type { AssistantRun } from "~entities/assistant";
import { interfaceLanguage, t } from "~shared/i18n";

export type AssistantThreadBucket = "attention" | "working" | "recent";

export function assistantThreadTitle(run: AssistantRun): string {
  const explicit = run.title?.trim();
  if (explicit && explicit !== "Новый чат") return explicit;
  const instruction = run.latestTurn?.instruction?.trim();
  if (instruction) return compactThreadTitle(instruction);
  if (run.inputMode === "mcp") return t("Внешний агент · {0}", run.agent?.clientName ?? "MCP");
  return run.inputMode === "voice" ? t("Голосовое поручение") : t("Новый чат");
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
    return t("ОБНОВИТЬ");
  }
  if (run.status === "awaiting_review") return t("ПРОВЕРИТЬ");
  if (run.status === "working") return t("В РАБОТЕ");
  if (run.status === "failed") return t("ОШИБКА");
  if (run.status === "cancelled") return t("ОСТАНОВЛЕН");
  if (run.status === "completed") return t("ГОТОВО");
  return t("ОТКРЫТ");
}

export function assistantThreadUpdatedLabel(updatedAt: string, now = Date.now()): string {
  const timestamp = Date.parse(updatedAt);
  if (!Number.isFinite(timestamp)) return t("недавно");
  const elapsed = Math.max(0, now - timestamp);
  if (elapsed < 60_000) return t("только что");
  const minutes = Math.floor(elapsed / 60_000);
  if (minutes < 60) return t("{0} мин", minutes);
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return t("{0} ч", hours);
  const days = Math.floor(hours / 24);
  return t("{0} дн", days);
}

export function assistantThreadCountLabel(count: number): string {
  const formattedCount = new Intl.NumberFormat(interfaceLanguage).format(count);
  const plural = new Intl.PluralRules(interfaceLanguage).select(count);
  if (plural === "one") return t("{0} чат", formattedCount);
  if (plural === "few") return t("{0} чата", formattedCount);
  return t("{0} чатов", formattedCount);
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
