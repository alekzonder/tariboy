/** Timestamps in the task table are read down a column, so they are short and
 *  tabular: the clock for today, the weekday for the last week, and a date
 *  before that. An unparseable or empty timestamp reads as an em dash rather
 *  than "Invalid Date". */
export function formatTaskTime(iso: string, now = new Date()): string {
  if (!iso) return "—"
  const at = new Date(iso)
  if (Number.isNaN(at.getTime())) return "—"
  const sameDay = at.getFullYear() === now.getFullYear()
    && at.getMonth() === now.getMonth()
    && at.getDate() === now.getDate()
  if (sameDay) {
    return `${String(at.getHours()).padStart(2, "0")}:${String(at.getMinutes()).padStart(2, "0")}`
  }
  const days = (now.getTime() - at.getTime()) / 86_400_000
  if (days >= 0 && days < 7) return at.toLocaleDateString("en-US", { weekday: "short" })
  return at.toLocaleDateString("en-US", { day: "numeric", month: "short" })
}
