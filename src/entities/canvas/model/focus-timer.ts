export function canvasFocusTimerEnd(now: number, minutes: number): number {
  if (!Number.isFinite(now) || !Number.isFinite(minutes) || minutes <= 0) return now;
  return Math.round(now + minutes * 60_000);
}

export function canvasFocusRemainingSeconds(endsAt: number | undefined, now: number): number {
  if (endsAt === undefined || !Number.isFinite(endsAt) || !Number.isFinite(now)) return 0;
  return Math.max(0, Math.ceil((endsAt - now) / 1_000));
}

export function formatCanvasFocusTime(seconds: number): string {
  const safe = Math.max(0, Math.trunc(Number.isFinite(seconds) ? seconds : 0));
  const minutes = Math.floor(safe / 60);
  const remainder = safe % 60;
  return `${String(minutes).padStart(2, "0")}:${String(remainder).padStart(2, "0")}`;
}
