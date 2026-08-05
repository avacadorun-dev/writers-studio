import { createDictation, stopDictation, uploadAudioChunk } from "~entities/dictation";
import {
  getLocalRecording,
  listAudioChunks,
  markAudioChunkUploaded,
  saveLocalRecording,
} from "~shared/lib";

export async function synchronizeLocalRecording(localId: string): Promise<string> {
  let recording = await getLocalRecording(localId);
  if (!recording) {
    throw new Error("Локальная запись не найдена");
  }
  const chunks = await listAudioChunks(localId);
  if (chunks.length === 0) {
    throw new Error("В записи нет аудиофрагментов");
  }
  let sessionId = recording.serverSessionId;
  if (!sessionId) {
    const created = await createDictation({
      projectId: recording.projectId,
      destinationNodeId: recording.destinationNodeId,
      clientSessionId: recording.localId,
      language: recording.language,
    });
    sessionId = created.session.id;
    recording = { ...recording, serverSessionId: sessionId };
    await saveLocalRecording(recording);
  }
  for (const chunk of chunks) {
    if (chunk.uploaded) {
      continue;
    }
    await uploadAudioChunk(sessionId, chunk.sequence, chunk.blob, chunk.sha256, chunk.durationMs);
    await markAudioChunkUploaded(localId, chunk.sequence);
  }
  await stopDictation(sessionId, {
    durationMs: recording.durationMs,
    recordedDurationMs: recording.recordedDurationMs,
    manifest: {
      format: recording.format,
      chunks: chunks.map((chunk) => ({
        sequence: chunk.sequence,
        sha256: chunk.sha256,
      })),
    },
  });
  await saveLocalRecording({ ...recording, status: "uploaded" });
  return sessionId;
}
