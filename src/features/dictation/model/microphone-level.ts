const MINIMUM_VISIBLE_DECIBELS = -72;
const MAXIMUM_VISIBLE_DECIBELS = -12;

export function calculateMicrophoneLevel(samples: Float32Array): number {
  if (samples.length === 0) {
    return 0;
  }

  let sumOfSquares = 0;
  let sampleCount = 0;
  let peak = 0;
  for (const sample of samples) {
    if (!Number.isFinite(sample)) {
      continue;
    }
    sumOfSquares += sample * sample;
    peak = Math.max(peak, Math.abs(sample));
    sampleCount += 1;
  }
  if (sampleCount === 0) {
    return 0;
  }

  const rms = Math.sqrt(sumOfSquares / sampleCount);
  const amplitude = Math.max(rms, peak * 0.35);
  if (amplitude === 0) {
    return 0;
  }

  const decibels = 20 * Math.log10(amplitude);
  return Math.min(
    1,
    Math.max(
      0,
      (decibels - MINIMUM_VISIBLE_DECIBELS) / (MAXIMUM_VISIBLE_DECIBELS - MINIMUM_VISIBLE_DECIBELS),
    ),
  );
}

export function smoothMicrophoneLevel(current: number, target: number): number {
  const factor = target > current ? 0.45 : 0.12;
  const next = current + (target - current) * factor;
  return next < 0.001 ? 0 : Math.min(1, Math.max(0, next));
}
