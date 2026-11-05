/** The one clock of the fixtures: "5 min ago" is the same on each run. A story
 *  that passes `now` to a component passes this value. */
export const NOW = "2026-11-05T10:00:00Z";

const ago = (ms: number): string => new Date(Date.parse(NOW) - ms).toISOString();

export const minutesAgo = (n: number): string => ago(n * 60_000);
export const hoursAgo = (n: number): string => ago(n * 3_600_000);
