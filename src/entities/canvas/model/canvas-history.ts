import type { CanvasDocument } from "./types";

export interface CanvasHistory {
  past: CanvasDocument[];
  present: CanvasDocument;
  future: CanvasDocument[];
}

export const canvasHistoryLimit = 200;
export const canvasHistoryMemoryLimit = 32 * 1024 * 1024;

export function createCanvasHistory(document: CanvasDocument): CanvasHistory {
  return { past: [], present: document, future: [] };
}

export function commitCanvasHistory(
  history: CanvasHistory,
  document: CanvasDocument,
  limit = canvasHistoryLimit,
  memoryLimit = canvasHistoryMemoryLimit,
): CanvasHistory {
  if (
    history.present === document ||
    JSON.stringify(history.present) === JSON.stringify(document)
  ) {
    return history;
  }
  const past = [...history.past, history.present].slice(-Math.max(1, limit));
  let weight = past.reduce((total, item) => total + canvasDocumentWeight(item), 0);
  while (past.length > 1 && weight > memoryLimit) {
    const removed = past.shift();
    if (removed) weight -= canvasDocumentWeight(removed);
  }
  return {
    past,
    present: document,
    future: [],
  };
}

export function canvasDocumentWeight(document: CanvasDocument): number {
  return new TextEncoder().encode(JSON.stringify(document)).byteLength;
}

export function undoCanvasHistory(history: CanvasHistory): CanvasHistory {
  const previous = history.past.at(-1);
  if (!previous) return history;
  return {
    past: history.past.slice(0, -1),
    present: previous,
    future: [history.present, ...history.future],
  };
}

export function redoCanvasHistory(history: CanvasHistory): CanvasHistory {
  const next = history.future[0];
  if (!next) return history;
  return {
    past: [...history.past, history.present].slice(-canvasHistoryLimit),
    present: next,
    future: history.future.slice(1),
  };
}
