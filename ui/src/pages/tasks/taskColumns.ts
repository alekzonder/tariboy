/** Which columns a row carries. `agent` is one agent's tasks (priority, how
 *  long the work has run); `all` is a whole server's (who owns it). The table
 *  header renders the same two shapes. */
export type TaskRowMode = "agent" | "all";

/** The one set of column widths. The header and every row read it, so a column
 *  cannot drift from its heading. `Key` includes the tree indents and the
 *  chevron, which is why it is the widest fixed column. */
export const TASK_COLUMNS = {
  key: 168,
  priority: 30,
  agent: 104,
  // A workflow status pill, its `k/n` step, and a problem mark fit side by side.
  status: 148,
  duration: 74,
  updated: 76,
  add: 24,
} as const;

/** One indent step per level of nesting, capped so a deep tree still leaves
 *  room for the title. */
const INDENT_PX = 14;
export const MAX_INDENT_DEPTH = 6;
/** A phone's 96px Key column keeps room for a queue marker and the start of
 *  the key: a smaller step, and depths past 3 look alike. */
const PHONE_INDENT_STEP_PX = 6;
const PHONE_MAX_INDENT_DEPTH = 3;
/** The gap the Key cell puts after every item, the indent included. */
const KEY_GAP_PX = 6;

/** The width of the one indent span before a key. With the gap after it, the
 *  key moves one step per level. A row at depth 0 has no indent span. */
export function keyIndentPx(depth: number, phone: boolean): number {
  const step = phone ? PHONE_INDENT_STEP_PX : INDENT_PX + KEY_GAP_PX;
  const levels = Math.min(depth, phone ? PHONE_MAX_INDENT_DEPTH : MAX_INDENT_DEPTH);
  return Math.max(0, levels * step - KEY_GAP_PX);
}
