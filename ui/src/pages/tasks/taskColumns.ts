/** Which columns a row carries. `agent` is one agent's tasks (priority, how
 *  long the work has run); `all` is a whole server's (who owns it); `servers`
 *  is every server's at once (who owns it and where it lives). The table
 *  header renders the same three shapes. */
export type TaskRowMode = "agent" | "all" | "servers"

/** The one set of column widths. The header and every row read it, so a column
 *  cannot drift from its heading. `Key` includes the tree indents and the
 *  chevron, which is why it is the widest fixed column. */
export const TASK_COLUMNS = {
  key: 168,
  priority: 30,
  agent: 104,
  server: 96,
  status: 110,
  duration: 74,
  updated: 76,
  add: 24,
} as const

/** One indent step per level of nesting, capped so a deep tree still leaves
 *  room for the title. */
export const INDENT_PX = 14
export const MAX_INDENT_DEPTH = 6
