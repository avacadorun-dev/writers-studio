import type { LiveTranscriptEvent } from "~entities/dictation";

export interface LiveFlowSegment {
  id: string;
  text: string;
  startMs: number;
  endMs: number;
  command?: LiveTranscriptEvent["command"];
}

export type LiveTranscriptRun =
  | { kind: "prose"; id: string; segments: LiveFlowSegment[] }
  | {
      kind: "command";
      id: string;
      segment: LiveFlowSegment & { command: NonNullable<LiveFlowSegment["command"]> };
    };

export function groupLiveTranscriptSegments(segments: LiveFlowSegment[]): LiveTranscriptRun[] {
  const runs: LiveTranscriptRun[] = [];
  let dialogueNext = false;
  for (const segment of segments) {
    if (segment.command) {
      runs.push({
        kind: "command",
        id: segment.id,
        segment: { ...segment, command: segment.command },
      });
      if (segment.command.executed && segment.command.kind === "dialogue_start") {
        dialogueNext = true;
      }
      continue;
    }
    const visibleSegment = dialogueNext
      ? { ...segment, text: `— ${segment.text.replace(/^[—–\s-]+/u, "")}` }
      : segment;
    dialogueNext = false;
    const current = runs.at(-1);
    if (current?.kind === "prose") {
      current.segments.push(visibleSegment);
    } else {
      runs.push({ kind: "prose", id: segment.id, segments: [visibleSegment] });
    }
  }
  return runs;
}
