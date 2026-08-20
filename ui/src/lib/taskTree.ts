import type { Task } from "./tasks"

export interface TaskTreeNode {
  task: Task
  children: TaskTreeNode[]
  orphaned: boolean
}

export interface VisibleTaskRow {
  task: Task
  depth: number
  hasChildren: boolean
  orphaned: boolean
}

export function canReorderTaskBeside(moving: Task | undefined, target: Task): boolean {
  return Boolean(moving && moving.priority === target.priority)
}

export function buildTaskForest(rows: Task[]): TaskTreeNode[] {
  const nodes = new Map<string, TaskTreeNode>()
  rows.forEach((task) => {
    nodes.set(task.key, { task, children: [], orphaned: false })
  })

  const roots: TaskTreeNode[] = []
  for (const task of rows) {
    const node = nodes.get(task.key)!
    const parent = task.parent_key ? nodes.get(task.parent_key) : undefined
    if (parent && parent !== node) {
      parent.children.push(node)
    } else {
      node.orphaned = Boolean(task.parent_key)
      roots.push(node)
    }
  }

  const stablePosition = (left: TaskTreeNode, right: TaskTreeNode) =>
    left.task.priority.localeCompare(right.task.priority)
      || left.task.position - right.task.position
      || left.task.key.localeCompare(right.task.key)
  const sort = (nodesToSort: TaskTreeNode[]) => {
    nodesToSort.sort(stablePosition)
    nodesToSort.forEach((node) => sort(node.children))
  }
  sort(roots)
  return roots
}

export function flattenVisible(
  forest: TaskTreeNode[],
  expanded: ReadonlySet<string>,
): VisibleTaskRow[] {
  const result: VisibleTaskRow[] = []
  const visit = (nodes: TaskTreeNode[], depth: number) => {
    for (const node of nodes) {
      result.push({
        task: node.task,
        depth,
        hasChildren: node.children.length > 0,
        orphaned: node.orphaned,
      })
      if (expanded.has(node.task.key)) visit(node.children, depth + 1)
    }
  }
  visit(forest, 0)
  return result
}

export function canDropTaskInside(
  forest: TaskTreeNode[],
  draggedKey: string,
  targetKey: string,
): boolean {
  if (!draggedKey || draggedKey === targetKey) return false
  const descendants = new Set<string>()
  const find = (nodes: TaskTreeNode[]): boolean => {
    for (const node of nodes) {
      if (node.task.key === draggedKey) {
        const collect = (children: TaskTreeNode[]) => {
          for (const child of children) {
            descendants.add(child.task.key)
            collect(child.children)
          }
        }
        collect(node.children)
        return true
      }
      if (find(node.children)) return true
    }
    return false
  }
  if (!find(forest)) return false
  return !descendants.has(targetKey)
}
