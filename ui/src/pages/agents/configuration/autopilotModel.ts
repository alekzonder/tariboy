// The Autopilot group shows the loop interval and the soft timeout in minutes,
// while the draft and the daemon keep whole seconds. These conversions are the
// only place the two units meet, so the requests stay what they were.

export type AutopilotMode = "on-demand" | "every";

const DEFAULT_MINUTES = "30";

// autopilotMode reads a saved or drafted interval: only 0 runs when given
// work alone. Text the box could not convert keeps "every" selected, so the
// operator sees the box and its error instead of a silent switch.
export const autopilotMode = (seconds: string): AutopilotMode =>
  seconds.trim() === "0" ? "on-demand" : "every";

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
  Number(seconds) > 0 ? toMinutes(seconds) : DEFAULT_MINUTES;
