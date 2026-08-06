import type { CanvasDocument } from "./types";

export interface CanvasHistory {
  past: CanvasDocument[];
  present: CanvasDocument;
  future: CanvasDocument[];
}

export function createCanvasHistory(document: CanvasDocument): CanvasHistory {
  return { past: [], present: document, future: [] };
}

export function commitCanvasHistory(
  history: CanvasHistory,
  document: CanvasDocument,
  limit = 100,
): CanvasHistory {
  if (
    history.present === document ||
    JSON.stringify(history.present) === JSON.stringify(document)
  ) {
    return history;
  }
  return {
    past: [...history.past, history.present].slice(-limit),
    present: document,
    future: [],
  };
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
    past: [...history.past, history.present],
    present: next,
    future: history.future.slice(1),
  };
}
