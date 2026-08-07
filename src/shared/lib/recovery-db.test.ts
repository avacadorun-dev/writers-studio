import { afterEach, describe, expect, test } from "bun:test";

import {
  clearLocalRecoveryData,
  deleteLocalRecording,
  getCanvasSnapshot,
  getLocalRecording,
  listAudioChunks,
  markAudioChunkUploaded,
  markCanvasSnapshotSynced,
  saveAudioChunk,
  saveCanvasSnapshot,
  saveLocalRecording,
  sha256Hex,
} from "./index";
import { withLocalRecoveryTimeout } from "./recovery-db";

const createdIds: string[] = [];

async function openLegacyRecoveryDatabase(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open("writers-studio-recovery", 1);
    request.onupgradeneeded = () => {
      const db = request.result;
      db.createObjectStore("editorSnapshots", { keyPath: "nodeId" });
      const recordings = db.createObjectStore("recordings", { keyPath: "localId" });
      recordings.createIndex("by-status", "status");
      const chunks = db.createObjectStore("audioChunks", {
        keyPath: ["localId", "sequence"],
      });
      chunks.createIndex("by-recording", "localId");
    };
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
}

afterEach(async () => {
  for (const localId of createdIds.splice(0)) {
    await deleteLocalRecording(localId);
  }
});

describe("recovery database", () => {
  test("durably stores, acknowledges, and deletes local audio chunks", async () => {
    const localId = crypto.randomUUID();
    createdIds.push(localId);
    const blob = new Blob(["writer-audio-fixture"], { type: "audio/webm" });
    const sha256 = await sha256Hex(blob);

    await saveLocalRecording({
      localId,
      projectId: crypto.randomUUID(),
      projectTitle: "Дом у воды",
      language: "ru",
      format: "webm",
      status: "recording",
      startedAt: Date.now(),
      durationMs: 1_000,
      recordedDurationMs: 1_000,
      lastAcknowledgedSequence: -1,
    });
    await saveAudioChunk({
      localId,
      sequence: 0,
      blob,
      sha256,
      durationMs: 1_000,
      uploaded: false,
      createdAt: Date.now(),
    });

    expect((await getLocalRecording(localId))?.status).toBe("recording");
    const stored = await listAudioChunks(localId);
    expect(stored).toHaveLength(1);
    expect(stored[0]?.sha256).toBe(sha256);
    expect(stored[0]?.uploaded).toBeFalse();

    await markAudioChunkUploaded(localId, 0);
    expect((await listAudioChunks(localId))[0]?.uploaded).toBeTrue();

    await deleteLocalRecording(localId);
    createdIds.pop();
    expect(await getLocalRecording(localId)).toBeUndefined();
    expect(await listAudioChunks(localId)).toEqual([]);
  });

  test("clears every local recovery record after account deletion", async () => {
    const localId = crypto.randomUUID();
    await saveLocalRecording({
      localId,
      projectId: crypto.randomUUID(),
      projectTitle: "Удаляемый проект",
      language: "ru",
      format: "webm",
      status: "recording",
      startedAt: Date.now(),
      durationMs: 500,
      recordedDurationMs: 500,
      lastAcknowledgedSequence: -1,
    });

    await clearLocalRecoveryData();

    expect(await getLocalRecording(localId)).toBeUndefined();
  });

  test("stores and acknowledges a canvas snapshot", async () => {
    const canvasId = crypto.randomUUID();
    await saveCanvasSnapshot({
      canvasId,
      projectId: crypto.randomUUID(),
      baseRevision: 3,
      content: { nodes: [], edges: [], future: { preserved: true } },
      savedAt: Date.now(),
      synced: false,
    });
    expect(await getCanvasSnapshot(canvasId)).toMatchObject({
      baseRevision: 3,
      synced: false,
    });
    await markCanvasSnapshotSynced(canvasId, 4);
    expect(await getCanvasSnapshot(canvasId)).toMatchObject({
      baseRevision: 4,
      synced: true,
    });
    await markCanvasSnapshotSynced("missing-canvas", 1);
  });

  test("fails open when IndexedDB does not answer", async () => {
    const pending = new Promise<never>(() => undefined);
    await expect(withLocalRecoveryTimeout(pending, 10)).rejects.toThrow(
      "Локальное хранилище восстановления не ответило вовремя",
    );
  });

  test("opens Canvas while an old tab blocks the shared database upgrade", async () => {
    await clearLocalRecoveryData();
    const legacyConnection = await openLegacyRecoveryDatabase();
    const startedAt = Date.now();

    expect(await getCanvasSnapshot("blocked-upgrade-canvas")).toBeUndefined();
    expect(Date.now() - startedAt).toBeLessThan(1_000);
    await saveCanvasSnapshot({
      canvasId: "blocked-upgrade-canvas",
      projectId: crypto.randomUUID(),
      baseRevision: 1,
      content: { nodes: [], edges: [] },
      savedAt: Date.now(),
      synced: false,
    });
    expect(await getCanvasSnapshot("blocked-upgrade-canvas")).toMatchObject({
      baseRevision: 1,
      synced: false,
    });

    legacyConnection.close();
    await clearLocalRecoveryData();
  });
});
