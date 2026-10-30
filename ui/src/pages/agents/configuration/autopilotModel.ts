// The Autopilot group shows the loop interval and the soft timeout in minutes,
// while the draft and the daemon keep whole seconds. These conversions are the
// only place the two units meet, so the requests stay what they were.

export type AutopilotMode = "on-demand" | "every";

const DEFAULT_MINUTES = "30";

// autopilotMode reads a saved or drafted interval: 0 runs only when given work.
export const autopilotMode = (seconds: string): AutopilotMode =>
  Number(seconds) > 0 ? "every" : "on-demand";

// toMinutes shows whole seconds as minutes ("90" → "1.5"); other text is kept
// so an invalid draft stays visible.
export function toMinutes(seconds: string): string {
  const text = seconds.trim();
  const n = Number(text);
  if (text === "" || !Number.isFinite(n)) return text;
  return String(Number((n / 60).toFixed(4)));
}

// toSeconds turns typed minutes into whole seconds. Text that is not zero or
// a positive number passes through unchanged, so validation rejects it.
export function toSeconds(minutes: string): string {
  const text = minutes.trim();
  const n = Number(text);
  if (text === "" || !Number.isFinite(n) || n < 0) return text;
  return String(Math.round(n * 60));
}

// rememberedMinutes is what "Also every N minutes" offers: the saved positive
// interval, or 30 minutes.
export const rememberedMinutes = (seconds: string): string =>
  autopilotMode(seconds) === "every" ? toMinutes(seconds) : DEFAULT_MINUTES;
