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

let databasePromise: Promise<IDBPDatabase<RecoverySchema>> | undefined;

function database(): Promise<IDBPDatabase<RecoverySchema>> {
  if (!databasePromise) {
    databasePromise = openDB<RecoverySchema>("writers-studio-recovery", 2, {
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
    });
  }
  return databasePromise;
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
  const db = await database();
  await db.put("canvasSnapshots", snapshot);
}

export async function getCanvasSnapshot(canvasId: string): Promise<CanvasSnapshot | undefined> {
  const db = await database();
  return db.get("canvasSnapshots", canvasId);
}

export async function markCanvasSnapshotSynced(
  canvasId: string,
  baseRevision: number,
): Promise<void> {
  const db = await database();
  const snapshot = await db.get("canvasSnapshots", canvasId);
  if (!snapshot) return;
  await db.put("canvasSnapshots", { ...snapshot, baseRevision, synced: true });
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
  await deleteDB("writers-studio-recovery");
}
