import { type DBSchema, deleteDB, type IDBPDatabase, openDB } from "idb";

import type { TiptapNode } from "~entities/project";

export interface EditorSnapshot {
  nodeId: string;
  projectId: string;
  baseRevision: number;
  content: TiptapNode;
  plainText: string;
  savedAt: number;
  synced: boolean;
}

export interface CanvasSnapshot {
  canvasId: string;
  projectId: string;
  baseRevision: number;
  content: Record<string, unknown>;
  savedAt: number;
  synced: boolean;
}

export interface LocalRecording {
  localId: string;
  serverSessionId?: string;
  projectId: string;
  destinationNodeId?: string;
  destinationTitle?: string;
  projectTitle: string;
  language: "ru" | "en";
  commandsEnabled?: boolean;
  commandPrefix?: string;
  liveTranscript?: boolean;
  format: string;
  status: "recording" | "paused" | "stopping" | "finalizing" | "uploaded";
  startedAt: number;
  durationMs: number;
  recordedDurationMs: number;
  lastAcknowledgedSequence: number;
}

export interface LocalAudioChunk {
  localId: string;
  sequence: number;
  blob: Blob;
  sha256: string;
  durationMs: number;
  uploaded: boolean;
  createdAt: number;
}

interface RecoverySchema extends DBSchema {
  editorSnapshots: {
    key: string;
    value: EditorSnapshot;
  };
  canvasSnapshots: {
    key: string;
    value: CanvasSnapshot;
  };
  recordings: {
    key: string;
    value: LocalRecording;
    indexes: { "by-status": LocalRecording["status"] };
  };
  audioChunks: {
    key: [string, number];
    value: LocalAudioChunk;
    indexes: {
      "by-recording": string;
    };
  };
}

interface CanvasRecoverySchema extends DBSchema {
  canvasSnapshots: {
    key: string;
    value: CanvasSnapshot;
  };
}

export const LOCAL_RECOVERY_TIMEOUT_MS = 1_500;

let databasePromise: Promise<IDBPDatabase<RecoverySchema>> | undefined;
let canvasDatabasePromise: Promise<IDBPDatabase<CanvasRecoverySchema>> | undefined;

export async function withLocalRecoveryTimeout<T>(
  operation: Promise<T>,
  timeoutMs = LOCAL_RECOVERY_TIMEOUT_MS,
): Promise<T> {
  let timeout: number | undefined;
  try {
    return await Promise.race([
      operation,
      new Promise<never>((_, reject) => {
        timeout = window.setTimeout(
          () => reject(new Error("Локальное хранилище восстановления не ответило вовремя")),
          timeoutMs,
        );
      }),
    ]);
  } finally {
    window.clearTimeout(timeout);
  }
}

function database(): Promise<IDBPDatabase<RecoverySchema>> {
  if (!databasePromise) {
    let opening: Promise<IDBPDatabase<RecoverySchema>>;
    opening = openDB<RecoverySchema>("writers-studio-recovery", 2, {
      upgrade(db, oldVersion) {
        if (oldVersion < 1) {
          db.createObjectStore("editorSnapshots", { keyPath: "nodeId" });
          const recordings = db.createObjectStore("recordings", { keyPath: "localId" });
          recordings.createIndex("by-status", "status");
          const chunks = db.createObjectStore("audioChunks", {
            keyPath: ["localId", "sequence"],
          });
          chunks.createIndex("by-recording", "localId");
        }
        if (oldVersion < 2) {
          db.createObjectStore("canvasSnapshots", { keyPath: "canvasId" });
        }
      },
      blocking() {
        void opening.then((db) => db.close()).catch(() => undefined);
        if (databasePromise === opening) databasePromise = undefined;
      },
      terminated() {
        if (databasePromise === opening) databasePromise = undefined;
      },
    }).catch((error: unknown) => {
      if (databasePromise === opening) databasePromise = undefined;
      throw error;
    });
    databasePromise = opening;
  }
  return databasePromise;
}

function canvasDatabase(): Promise<IDBPDatabase<CanvasRecoverySchema>> {
  if (!canvasDatabasePromise) {
    let opening: Promise<IDBPDatabase<CanvasRecoverySchema>>;
    opening = openDB<CanvasRecoverySchema>("writers-studio-canvas-recovery", 1, {
      upgrade(db) {
        db.createObjectStore("canvasSnapshots", { keyPath: "canvasId" });
      },
      blocking() {
        void opening.then((db) => db.close()).catch(() => undefined);
        if (canvasDatabasePromise === opening) canvasDatabasePromise = undefined;
      },
      terminated() {
        if (canvasDatabasePromise === opening) canvasDatabasePromise = undefined;
      },
    }).catch((error: unknown) => {
      if (canvasDatabasePromise === opening) canvasDatabasePromise = undefined;
      throw error;
    });
    canvasDatabasePromise = opening;
  }
  return canvasDatabasePromise;
}

export async function saveEditorSnapshot(snapshot: EditorSnapshot): Promise<void> {
  const db = await database();
  await db.put("editorSnapshots", snapshot);
}

export async function getEditorSnapshot(nodeId: string): Promise<EditorSnapshot | undefined> {
  const db = await database();
  return db.get("editorSnapshots", nodeId);
}

export async function markEditorSnapshotSynced(
  nodeId: string,
  baseRevision: number,
): Promise<void> {
  const db = await database();
  const snapshot = await db.get("editorSnapshots", nodeId);
  if (!snapshot) {
    return;
  }
  await db.put("editorSnapshots", { ...snapshot, baseRevision, synced: true });
}

export async function saveCanvasSnapshot(snapshot: CanvasSnapshot): Promise<void> {
  await withLocalRecoveryTimeout(
    canvasDatabase().then((db) => db.put("canvasSnapshots", snapshot)),
  );
}

export async function getCanvasSnapshot(canvasId: string): Promise<CanvasSnapshot | undefined> {
  const snapshot = await withLocalRecoveryTimeout(
    canvasDatabase().then((db) => db.get("canvasSnapshots", canvasId)),
  );
  if (snapshot) return snapshot;

  // Canvas recovery originally lived in version 2 of the shared database. A
  // short best-effort migration preserves those snapshots without letting an
  // old tab that still holds version 1 block the Canvas UI.
  const legacy = await withLocalRecoveryTimeout(
    database().then((db) => db.get("canvasSnapshots", canvasId)),
    250,
  ).catch(() => undefined);
  if (legacy) await saveCanvasSnapshot(legacy).catch(() => undefined);
  return legacy;
}

export async function markCanvasSnapshotSynced(
  canvasId: string,
  baseRevision: number,
): Promise<void> {
  await withLocalRecoveryTimeout(
    canvasDatabase().then(async (db) => {
      const snapshot = await db.get("canvasSnapshots", canvasId);
      if (!snapshot) return;
      await db.put("canvasSnapshots", { ...snapshot, baseRevision, synced: true });
    }),
  );
}

export async function saveLocalRecording(recording: LocalRecording): Promise<void> {
  const db = await database();
  await db.put("recordings", recording);
}

export async function getLocalRecording(localId: string): Promise<LocalRecording | undefined> {
  const db = await database();
  return db.get("recordings", localId);
}

export async function listLocalRecordings(): Promise<LocalRecording[]> {
  const db = await database();
  return db.getAll("recordings");
}

export async function saveAudioChunk(chunk: LocalAudioChunk): Promise<void> {
  const db = await database();
  await db.put("audioChunks", chunk);
}

export async function listAudioChunks(localId: string): Promise<LocalAudioChunk[]> {
  const db = await database();
  return db.getAllFromIndex("audioChunks", "by-recording", localId);
}

export async function markAudioChunkUploaded(localId: string, sequence: number): Promise<void> {
  const db = await database();
  const chunk = await db.get("audioChunks", [localId, sequence]);
  if (chunk) {
    await db.put("audioChunks", { ...chunk, uploaded: true });
  }
}

export async function deleteLocalRecording(localId: string): Promise<void> {
  const db = await database();
  const transaction = db.transaction(["recordings", "audioChunks"], "readwrite");
  const chunks = await transaction
    .objectStore("audioChunks")
    .index("by-recording")
    .getAllKeys(localId);
  for (const key of chunks) {
    await transaction.objectStore("audioChunks").delete(key);
  }
  await transaction.objectStore("recordings").delete(localId);
  await transaction.done;
}

export async function clearLocalRecoveryData(): Promise<void> {
  if (databasePromise) {
    const db = await databasePromise;
    db.close();
    databasePromise = undefined;
  }
  if (canvasDatabasePromise) {
    const db = await canvasDatabasePromise;
    db.close();
    canvasDatabasePromise = undefined;
  }
  await Promise.all([
    deleteDB("writers-studio-recovery"),
    deleteDB("writers-studio-canvas-recovery"),
  ]);
}
