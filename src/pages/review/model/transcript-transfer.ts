import type { TiptapNode } from "~entities/project";

export function mergeTranscriptAtEnd(destination: TiptapNode, transcript: TiptapNode): TiptapNode {
  return {
    ...destination,
    type: "doc",
    content: [...(destination.content ?? []), ...(transcript.content ?? [])],
  };
}
