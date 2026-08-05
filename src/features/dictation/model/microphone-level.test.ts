import { describe, expect, test } from "bun:test";

import { calculateMicrophoneLevel, smoothMicrophoneLevel } from "./microphone-level";

describe("microphone level", () => {
  test("keeps digital silence at zero", () => {
    expect(calculateMicrophoneLevel(new Float32Array(512))).toBe(0);
  });

  test("maps quiet speech and loud input into a visible normalized range", () => {
    const quiet = new Float32Array(512).map((_, index) => (index % 2 === 0 ? 0.01 : -0.01));
    const loud = new Float32Array(512).map((_, index) => (index % 2 === 0 ? 0.5 : -0.5));

    expect(calculateMicrophoneLevel(quiet)).toBeGreaterThan(0.5);
    expect(calculateMicrophoneLevel(quiet)).toBeLessThan(0.6);
    expect(calculateMicrophoneLevel(loud)).toBeGreaterThan(0.95);
  });

  test("filters input below the ambient-noise floor", () => {
    const veryQuiet = new Float32Array(512).map((_, index) => (index % 2 === 0 ? 0.0001 : -0.0001));

    expect(calculateMicrophoneLevel(veryQuiet)).toBe(0);
  });

  test("keeps quiet speech above the noise floor visible", () => {
    const quietSpeech = new Float32Array(512).map((_, index) => (index % 2 === 0 ? 0.001 : -0.001));

    expect(calculateMicrophoneLevel(quietSpeech)).toBeGreaterThan(0.1);
    expect(calculateMicrophoneLevel(quietSpeech)).toBeLessThan(0.3);
  });

  test("responds quickly to speech and releases the meter gradually", () => {
    const attack = smoothMicrophoneLevel(0, 0.8);
    const release = smoothMicrophoneLevel(attack, 0);

    expect(attack).toBeGreaterThan(0.3);
    expect(release).toBeGreaterThan(0);
    expect(release).toBeLessThan(attack);
  });
});
