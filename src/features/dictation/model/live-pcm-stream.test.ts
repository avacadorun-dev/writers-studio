import { describe, expect, test } from "bun:test";

import { encodePCM16, encodePCM16WAV } from "./live-pcm-stream";

describe("live PCM encoding", () => {
  test("creates a standalone mono PCM16 WAV chunk", () => {
    const wav = encodePCM16WAV(new Float32Array([-1, -0.5, 0, 0.5, 1]), 16_000);
    const view = new DataView(wav);
    const ascii = (offset: number, length: number) =>
      String.fromCharCode(...new Uint8Array(wav, offset, length));

    expect(ascii(0, 4)).toBe("RIFF");
    expect(ascii(8, 4)).toBe("WAVE");
    expect(ascii(12, 4)).toBe("fmt ");
    expect(ascii(36, 4)).toBe("data");
    expect(view.getUint16(20, true)).toBe(1);
    expect(view.getUint16(22, true)).toBe(1);
    expect(view.getUint32(24, true)).toBe(16_000);
    expect(view.getUint16(34, true)).toBe(16);
    expect(view.getUint32(40, true)).toBe(10);
    expect(view.getInt16(44, true)).toBe(-32_768);
    expect(view.getInt16(52, true)).toBe(32_767);
  });

  test("creates headerless little-endian PCM16 frames for a streaming session", () => {
    const pcm = encodePCM16(new Float32Array([-1, -0.5, 0, 0.5, 1]));
    const view = new DataView(pcm);

    expect(pcm.byteLength).toBe(10);
    expect(view.getInt16(0, true)).toBe(-32_768);
    expect(view.getInt16(2, true)).toBe(-16_384);
    expect(view.getInt16(4, true)).toBe(0);
    expect(view.getInt16(6, true)).toBe(16_383);
    expect(view.getInt16(8, true)).toBe(32_767);
  });
});
