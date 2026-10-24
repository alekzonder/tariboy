import type { Task } from "./api";

export interface TaskTreeNode {
  task: Task;
  children: TaskTreeNode[];
  orphaned: boolean;
}

export interface VisibleTaskRow {
  task: Task;
  depth: number;
  hasChildren: boolean;
  orphaned: boolean;
}

export function canReorderTaskBeside(moving: Task | undefined, target: Task): boolean {
  return Boolean(moving && moving.priority === target.priority);
}

export function buildTaskForest(rows: Task[]): TaskTreeNode[] {
  const nodes = new Map<string, TaskTreeNode>();
  rows.forEach((task) => {
    nodes.set(task.key, { task, children: [], orphaned: false });
  });

  const roots: TaskTreeNode[] = [];
  for (const task of rows) {
    const node = nodes.get(task.key)!;
    const parent = task.parent_key ? nodes.get(task.parent_key) : undefined;
    if (parent && parent !== node) {
      parent.children.push(node);
    } else {
      node.orphaned = Boolean(task.parent_key);
      roots.push(node);
    }
  }

  const stablePosition = (left: TaskTreeNode, right: TaskTreeNode) =>
    left.task.priority.localeCompare(right.task.priority) ||
    left.task.position - right.task.position ||
    left.task.key.localeCompare(right.task.key);
  const sort = (nodesToSort: TaskTreeNode[]) => {
    nodesToSort.sort(stablePosition);
    nodesToSort.forEach((node) => sort(node.children));
  };
  sort(roots);
  return roots;
}

export function flattenVisible(
  forest: TaskTreeNode[],
  expanded: ReadonlySet<string>,
): VisibleTaskRow[] {
  const result: VisibleTaskRow[] = [];
  const visit = (nodes: TaskTreeNode[], depth: number) => {
    for (const node of nodes) {
      result.push({
        task: node.task,
        depth,
        hasChildren: node.children.length > 0,
        orphaned: node.orphaned,
      });
      if (expanded.has(node.task.key)) visit(node.children, depth + 1);
    }
  };
  visit(forest, 0);
  return result;
}

export function canDropTaskInside(
  forest: TaskTreeNode[],
  draggedKey: string,
  targetKey: string,
): boolean {
  if (!draggedKey || draggedKey === targetKey) return false;
  const descendants = new Set<string>();
  const find = (nodes: TaskTreeNode[]): boolean => {
    for (const node of nodes) {
      if (node.task.key === draggedKey) {
        const collect = (children: TaskTreeNode[]) => {
          for (const child of children) {
            descendants.add(child.task.key);
            collect(child.children);
          }
        };
        collect(node.children);
        return true;
      }
      if (find(node.children)) return true;
    }
    return false;
  };
  if (!find(forest)) return false;
  return !descendants.has(targetKey);
}

export interface TaskDrop {
  key: string;
  parentKey: string;
  beforeKey: string;
}

function keyAfter(tasks: Task[], movingKey: string, target: Task): string {
  const siblings = tasks
    .filter((task) => task.parent_key === target.parent_key && task.key !== movingKey)
    .sort((left, right) => left.position - right.position);
  return siblings[siblings.findIndex((task) => task.key === target.key) + 1]?.key ?? "";
}

function dropDestination(
  tasks: Task[],
  moving: Task | undefined,
  placement: string,
  target: Task,
): Omit<TaskDrop, "key"> | null {
  if (placement !== "before" && placement !== "after") {
    return { parentKey: target.key, beforeKey: "" };
  }
  if (!canReorderTaskBeside(moving, target)) return null;
  const beforeKey = placement === "before" ? target.key : keyAfter(tasks, moving!.key, target);
  return { parentKey: target.parent_key, beforeKey };
}

/**
 * Turns a drop onto a row zone (`before:KEY`, `inside:KEY`, `after:KEY`) or
 * the tree root (`__root__`) into a move, or null when the drop is not allowed.
 */
export function resolveTaskDrop({
  tasks,
  forest,
  activeKey,
  overId,
}: {
  tasks: Task[];
  forest: TaskTreeNode[];
  activeKey: string;
  overId: string;
}): TaskDrop | null {
  if (activeKey === overId) return null;
  if (overId === "__root__") return { key: activeKey, parentKey: "", beforeKey: "" };
  const byKey = new Map(tasks.map((task) => [task.key, task]));
  const [placement, targetKey] = overId.split(":", 2);
  const target = byKey.get(targetKey);
  if (targetKey === activeKey || !target) return null;
  const moving = byKey.get(activeKey);
  const destination = dropDestination(tasks, moving, placement, target);
  if (!destination || !moving) return null;
  const { parentKey } = destination;
  const parent = parentKey ? byKey.get(parentKey) : undefined;
  if (parent && parent.queue !== moving.queue) return null;
  if (parentKey && !canDropTaskInside(forest, activeKey, parentKey)) return null;
  return { key: activeKey, ...destination };
}
