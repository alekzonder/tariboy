import type { Queue, Task } from "./api";
import { isWorkflowTask } from "./tasks";

/** The parent of an orphaned root in another queue: the row links to it. */
export interface TaskParentLink {
  key: string;
  queue: string;
  /** The parent is loaded, so the panel can open it. */
  canOpen: boolean;
}

/** Where a row stands against its parent's queue. */
interface TaskQueuePlace {
  /** The parent is in the tree and in another queue. */
  crossQueue?: boolean;
  /** The queue of the parent in the tree. */
  parentQueue?: string;
  parentLink?: TaskParentLink;
}

export interface TaskTreeNode extends TaskQueuePlace {
  task: Task;
  children: TaskTreeNode[];
  orphaned: boolean;
}

export interface VisibleTaskRow extends TaskQueuePlace {
  task: Task;
  depth: number;
  hasChildren: boolean;
  orphaned: boolean;
}

/** The queue prefix of a task key: everything before the last `-`. */
export function queueOfKey(key: string): string {
  const dash = key.lastIndexOf("-");
  return dash > 0 ? key.slice(0, dash) : key;
}

/** The queue of a parent: the loaded task's, else the prefix of its key. */
export function parentQueueOf(parentKey: string, tasks: readonly Task[]): string {
  return tasks.find((task) => task.key === parentKey)?.queue ?? queueOfKey(parentKey);
}

/** The title of the queue marker of a child in another queue. */
export function queueMarkerLabel(task: Task, parentQueue: string, queues: readonly Queue[]) {
  const name = queues.find((queue) => queue.prefix === task.queue)?.name;
  const kind = isWorkflowTask(task)
    ? `workflow ${task.workflow_name ?? ""}`.trim()
    : "flexible tasks";
  const parts = name ? [`Queue ${task.queue}`, name, kind] : [`Queue ${task.queue}`];
  return `${parts.join(" · ")} (parent queue ${parentQueue})`;
}

/** The title of the marker before a parent in another queue. */
export function parentQueueLabel(queue: string, queues: readonly Queue[]) {
  const name = queues.find((item) => item.prefix === queue)?.name;
  return name ? `Parent queue ${queue} · ${name}` : `Parent queue ${queue}`;
}

/**
 * The tasks of `queue` and their whole subtrees from any queue, in input order.
 * Ancestors outside the queue are left out, so such a task becomes a root.
 */
export function filterTasksByQueue(tasks: Task[], queue: string): Task[] {
  const byKey = new Map(tasks.map((task) => [task.key, task]));
  const kept = new Map<string, boolean>();
  const keeps = (task: Task, seen: Set<string>): boolean => {
    const known = kept.get(task.key);
    if (known !== undefined) return known;
    const parent = byKey.get(task.parent_key);
    const result =
      task.queue === queue ||
      (!!parent && !seen.has(parent.key) && keeps(parent, seen.add(task.key)));
    kept.set(task.key, result);
    return result;
  };
  return tasks.filter((task) => keeps(task, new Set()));
}

/** A sibling reorder keeps the priority, and a child from another queue
 *  (`movingParent` in another queue) cannot move among its siblings. */
export function canReorderTaskBeside(
  moving: Task | undefined,
  target: Task,
  movingParent?: Task,
): boolean {
  if (!moving || moving.priority !== target.priority) return false;
  return !movingParent || movingParent.queue === moving.queue;
}

function parentLinkOf(task: Task, known: ReadonlyMap<string, Task>): TaskParentLink | undefined {
  const parent = known.get(task.parent_key);
  const queue = parentQueueOf(task.parent_key, parent ? [parent] : []);
  if (queue === task.queue) return undefined;
  return { key: task.parent_key, queue, canOpen: !!parent };
}

/**
 * Builds the tree of `rows`. `known` is every loaded task: a root whose parent
 * was filtered out still links to it.
 */
export function buildTaskForest(rows: Task[], known: Task[] = rows): TaskTreeNode[] {
  const nodes = new Map<string, TaskTreeNode>();
  rows.forEach((task) => {
    nodes.set(task.key, { task, children: [], orphaned: false });
  });
  const loaded = new Map(known.map((task) => [task.key, task]));

  const roots: TaskTreeNode[] = [];
  for (const task of rows) {
    const node = nodes.get(task.key)!;
    const parent = task.parent_key ? nodes.get(task.parent_key) : undefined;
    if (parent && parent !== node) {
      parent.children.push(node);
      node.parentQueue = parent.task.queue;
      node.crossQueue = parent.task.queue !== task.queue;
    } else {
      node.orphaned = Boolean(task.parent_key);
      node.crossQueue = false;
      if (node.orphaned) node.parentLink = parentLinkOf(task, loaded);
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
        crossQueue: node.crossQueue,
        parentQueue: node.parentQueue,
        parentLink: node.parentLink,
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
  const movingParent = tasks.find((task) => task.key === moving?.parent_key);
  if (!canReorderTaskBeside(moving, target, movingParent)) return null;
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
