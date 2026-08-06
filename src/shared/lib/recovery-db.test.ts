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

const createdIds: string[] = [];

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
});
