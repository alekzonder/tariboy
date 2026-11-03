/** Human-readable timestamp for merged audit/iteration lists.
 *
 * Parses `iso` with `new Date`. For a timestamp dated today (local time)
 * returns `HH:MM`; for another day in the current year returns `MM-DD HH:MM`;
 * for a different year returns `YYYY-MM-DD HH:MM` so timestamps from different
 * years are never ambiguous. Month is 1-based and all fields are zero-padded.
 * On unparseable input the raw string is returned.
 *
 * Mirrors the today-check/pad pattern of the local `fmtDate` helper in
 * ui/src/pages/TasksPage.tsx, but that helper uses an older `MM-DD` (no time)
 * format and is intentionally left unchanged.
 */
export function fmtDateTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const pad = (n: number) => String(n).padStart(2, "0");
  const hm = `${pad(d.getHours())}:${pad(d.getMinutes())}`;
  const now = new Date();
  const sameYear = d.getFullYear() === now.getFullYear();
  const isToday = sameYear && d.getMonth() === now.getMonth() && d.getDate() === now.getDate();
  const md = `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${hm}`;
  if (isToday) return hm;
  return sameYear ? md : `${d.getFullYear()}-${md}`;
}

/** A period `since – until` with the date and time at both ends:
 *  `MM-DD HH:MM`, or `YYYY-MM-DD HH:MM` at both ends when either end is not
 *  in the current year (local time). An unparseable end is shown as given. */
export function fmtPeriod(since: string, until: string): string {
  const ends = [new Date(since), new Date(until)];
  const year = new Date().getFullYear();
  const withYear = ends.some((d) => !Number.isNaN(d.getTime()) && d.getFullYear() !== year);
  const pad = (n: number) => String(n).padStart(2, "0");
  const [a, b] = ends.map((d, i) => {
    if (Number.isNaN(d.getTime())) return i === 0 ? since : until;
    const md = `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
    return withYear ? `${d.getFullYear()}-${md}` : md;
  });
  return `${a} – ${b}`;
}

/** How long ago `iso` was, from `now`: "just now", "5 min ago", "3 h ago",
 *  "2 d ago", or the local date (`YYYY-MM-DD`) after 30 days. "—" when `iso`
 *  is not a time. */
export function relativeTime(iso: string, now: number = Date.now()): string {
  const time = Date.parse(iso);
  if (!Number.isFinite(time)) return "—";
  const minutes = Math.floor((now - time) / 60_000);
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes} min ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} h ago`;
  const days = Math.floor(hours / 24);
  if (days <= 30) return `${days} d ago`;
  const d = new Date(time);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}
