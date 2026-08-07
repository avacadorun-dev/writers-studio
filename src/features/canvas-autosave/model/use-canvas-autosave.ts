import { useCallback, useEffect, useRef, useState } from "react";

import { type CanvasDocument, saveCanvasRevision } from "~entities/canvas";
import { APIError } from "~shared/api";
import { markCanvasSnapshotSynced, saveCanvasSnapshot } from "~shared/lib";
import type { SaveState } from "~shared/ui";

interface UseCanvasAutosaveInput {
  canvasId: string;
  projectId: string;
  revision: number;
  onRevision: (revision: number) => void;
}

interface UseCanvasAutosaveResult {
  saveState: SaveState;
  conflictCanvasId?: string;
  queue: (content: CanvasDocument) => void;
  flush: () => Promise<void>;
}

export async function runLocalRecoveryOperation(operation: () => Promise<void>): Promise<boolean> {
  try {
    await operation();
    return true;
  } catch {
    return false;
  }
}

export function useCanvasAutosave({
  canvasId,
  projectId,
  revision,
  onRevision,
}: UseCanvasAutosaveInput): UseCanvasAutosaveResult {
  const [saveState, setSaveState] = useState<SaveState>("saved");
  const [conflictCanvasId, setConflictCanvasId] = useState<string>();
  const baseRevision = useRef(revision);
  const onRevisionRef = useRef(onRevision);
  const latestContent = useRef<CanvasDocument | undefined>(undefined);
  const dirty = useRef(false);
  const inFlight = useRef(false);
  const conflict = useRef(false);
  const localTimer = useRef<number | undefined>(undefined);
  const serverTimer = useRef<number | undefined>(undefined);
  const maxTimer = useRef<number | undefined>(undefined);

  useEffect(() => {
    baseRevision.current = revision;
  }, [revision]);

  useEffect(() => {
    onRevisionRef.current = onRevision;
  }, [onRevision]);

  const persistSnapshot = useCallback(
    (content: CanvasDocument, revision: number) =>
      runLocalRecoveryOperation(() =>
        saveCanvasSnapshot({
          canvasId,
          projectId,
          baseRevision: revision,
          content,
          savedAt: Date.now(),
          synced: false,
        }),
      ),
    [canvasId, projectId],
  );

  const persistLocal = useCallback(async () => {
    if (!latestContent.current) return true;
    return persistSnapshot(latestContent.current, baseRevision.current);
  }, [persistSnapshot]);

  const markLocalSynced = useCallback(
    (revision: number) =>
      runLocalRecoveryOperation(() => markCanvasSnapshotSynced(canvasId, revision)),
    [canvasId],
  );

  const localFailureState = useCallback((persisted: boolean): SaveState => {
    if (!navigator.onLine && persisted) return "local";
    return "pending";
  }, []);

  const flush = useCallback(async () => {
    if (!latestContent.current || !dirty.current || inFlight.current || conflict.current) return;
    window.clearTimeout(serverTimer.current);
    window.clearTimeout(maxTimer.current);
    maxTimer.current = undefined;
    const content = latestContent.current;
    const savingBase = baseRevision.current;
    dirty.current = false;
    inFlight.current = true;
    setSaveState("saving");
    const locallyPersisted = await persistSnapshot(content, savingBase);
    try {
      const response = await saveCanvasRevision(canvasId, {
        baseRevision: savingBase,
        content,
        source: "manual",
      });
      baseRevision.current = response.revision.revision;
      onRevisionRef.current(response.revision.revision);
      conflict.current = false;
      setConflictCanvasId(undefined);
      if (dirty.current) {
        await persistLocal();
        setSaveState("saving");
      } else {
        await markLocalSynced(response.revision.revision);
        setSaveState("saved");
      }
    } catch (error) {
      const changedWhileSaving = dirty.current;
      if (error instanceof APIError && error.code === "canvas_revision_conflict") {
        conflict.current = true;
        setConflictCanvasId(error.conflictCanvasId);
        baseRevision.current = error.currentRevision ?? savingBase;
        dirty.current = changedWhileSaving;
        if (changedWhileSaving) {
          await persistLocal();
        } else {
          await markLocalSynced(baseRevision.current);
        }
        setSaveState("conflict");
      } else {
        dirty.current = true;
        setSaveState(localFailureState(locallyPersisted));
      }
    } finally {
      inFlight.current = false;
      if (dirty.current && !conflict.current) {
        serverTimer.current = window.setTimeout(() => void flush(), 1_500);
      }
    }
  }, [canvasId, localFailureState, markLocalSynced, persistLocal, persistSnapshot]);

  const queue = useCallback(
    (content: CanvasDocument) => {
      latestContent.current = content;
      dirty.current = true;
      setSaveState("saving");
      window.clearTimeout(localTimer.current);
      localTimer.current = window.setTimeout(() => void persistLocal(), 300);
      window.clearTimeout(serverTimer.current);
      serverTimer.current = window.setTimeout(() => void flush(), 1_500);
      if (!maxTimer.current) {
        maxTimer.current = window.setTimeout(() => {
          maxTimer.current = undefined;
          void flush();
        }, 15_000);
      }
    },
    [flush, persistLocal],
  );

  useEffect(() => {
    const handleOnline = () => {
      if (dirty.current && !conflict.current) void flush();
    };
    window.addEventListener("online", handleOnline);
    return () => {
      window.removeEventListener("online", handleOnline);
      window.clearTimeout(localTimer.current);
      window.clearTimeout(serverTimer.current);
      window.clearTimeout(maxTimer.current);
      if (dirty.current && latestContent.current) void persistLocal();
    };
  }, [flush, persistLocal]);

  return { saveState, conflictCanvasId, queue, flush };
}
