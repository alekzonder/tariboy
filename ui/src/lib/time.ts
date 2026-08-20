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
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const pad = (n: number) => String(n).padStart(2, "0")
  const hm = `${pad(d.getHours())}:${pad(d.getMinutes())}`
  const now = new Date()
  const sameYear = d.getFullYear() === now.getFullYear()
  const isToday =
    sameYear &&
    d.getMonth() === now.getMonth() &&
    d.getDate() === now.getDate()
  const md = `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${hm}`
  if (isToday) return hm
  return sameYear ? md : `${d.getFullYear()}-${md}`
}
