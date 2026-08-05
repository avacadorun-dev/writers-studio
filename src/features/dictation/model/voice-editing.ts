import type { CommandEvent } from "~entities/dictation";

import { groupLiveTranscriptSegments, type LiveFlowSegment } from "./live-transcript-flow";

export interface VoiceEditTarget {
  runId: string;
  segmentIds: string[];
  paragraphNumber: number;
  text: string;
  selectionStart: number;
  selectionEnd: number;
  label: string;
}

export function isNavigationCommand(command: CommandEvent): boolean {
  return (
    command.kind === "navigate_paragraph" ||
    command.kind === "navigate_sentence" ||
    command.kind === "navigate_text" ||
    command.kind === "navigate_end"
  );
}

export function resolveVoiceEditTarget(
  segments: LiveFlowSegment[],
  command: CommandEvent,
): VoiceEditTarget | null {
  if (!command.executed || !isNavigationCommand(command) || command.kind === "navigate_end") {
    return null;
  }

  const paragraphs = groupLiveTranscriptSegments(segments)
    .filter((run) => run.kind === "prose")
    .map((run, index) => ({
      runId: run.id,
      segmentIds: run.segments.map((segment) => segment.id),
      paragraphNumber: index + 1,
      text: run.segments.map((segment) => segment.text).join(" "),
    }));

  if (command.kind === "navigate_paragraph") {
    const paragraph = paragraphs[(command.ordinal ?? 0) - 1];
    if (!paragraph) {
      return null;
    }
    return {
      ...paragraph,
      selectionStart: paragraph.text.length,
      selectionEnd: paragraph.text.length,
      label: `${command.ordinal}-й абзац`,
    };
  }

  if (command.kind === "navigate_sentence" && command.ordinal) {
    let current = 0;
    for (const paragraph of paragraphs) {
      for (const range of sentenceRanges(paragraph.text)) {
        current += 1;
        if (current === command.ordinal) {
          return {
            ...paragraph,
            selectionStart: range.start,
            selectionEnd: range.end,
            label: `${command.ordinal}-е предложение`,
          };
        }
      }
    }
    return null;
  }

  const query = command.target?.trim();
  if (!query) {
    return null;
  }
  for (const paragraph of paragraphs) {
    const match = findTextRange(paragraph.text, query);
    if (!match) {
      continue;
    }
    if (command.kind === "navigate_sentence") {
      const sentence = sentenceRanges(paragraph.text).find(
        (range) => match.start >= range.start && match.start < range.end,
      );
      if (!sentence) {
        return null;
      }
      return {
        ...paragraph,
        selectionStart: sentence.start,
        selectionEnd: sentence.end,
        label: `предложение «${query}»`,
      };
    }
    return {
      ...paragraph,
      selectionStart: match.start,
      selectionEnd: match.end,
      label: `слова «${paragraph.text.slice(match.start, match.end)}»`,
    };
  }
  return null;
}

function sentenceRanges(text: string): Array<{ start: number; end: number }> {
  const ranges: Array<{ start: number; end: number }> = [];
  for (const match of text.matchAll(/[^.!?…]+(?:[.!?…]+|$)/gu)) {
    const value = match[0];
    const leadingWhitespace = value.match(/^\s*/u)?.[0].length ?? 0;
    const trailingWhitespace = value.match(/\s*$/u)?.[0].length ?? 0;
    const start = (match.index ?? 0) + leadingWhitespace;
    const end = (match.index ?? 0) + value.length - trailingWhitespace;
    if (end > start) {
      ranges.push({ start, end });
    }
  }
  return ranges;
}

function findTextRange(text: string, query: string): { start: number; end: number } | null {
  const directIndex = text.toLocaleLowerCase("ru").indexOf(query.toLocaleLowerCase("ru"));
  if (directIndex >= 0) {
    return { start: directIndex, end: directIndex + query.length };
  }

  const tokens = query
    .split(/\s+/u)
    .map((token) => token.replace(/^[\p{P}\p{S}]+|[\p{P}\p{S}]+$/gu, ""))
    .filter(Boolean);
  if (tokens.length === 0) {
    return null;
  }
  const expression = new RegExp(
    tokens.map((token) => escapeRegExp(token)).join("[\\s\\p{P}\\p{S}]+"),
    "iu",
  );
  const match = expression.exec(text);
  return match?.index === undefined
    ? null
    : { start: match.index, end: match.index + match[0].length };
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/gu, "\\$&");
}
