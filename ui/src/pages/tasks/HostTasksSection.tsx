import { useEffect, useMemo, useState } from "react";
import { useApi } from "@/components/apiContext";
import type { Queue } from "@/types";
import { errorText } from "@/lib/errors";
import { hostToParam } from "@/lib/terminalsHost";
import { parentQueueOf } from "@/lib/taskTree";
import type { HostTaskSummary } from "./allHostsTasksModel";
import TaskForm from "./TaskForm";
import TaskTree from "./TaskTree";
import { useTasksWorkspaceActions } from "./useTasksWorkspaceActions";
import { useTasksWorkspaceData } from "./useTasksWorkspaceData";
import type { TasksWorkspaceView } from "./useTasksWorkspaceView";

const NOTE = "px-4 py-1.5 text-ui text-muted-foreground";
// A host without queues yet reports the same empty list on every render.
const NO_QUEUES: readonly Queue[] = [];

/** A host's name above its rows, with what the group has to say. */
function HostHeading({ label, note }: { label: string; note?: string }) {
  return (
    <h2 className="flex items-baseline gap-1.5 border-b border-border/60 px-4 pt-3 pb-1.5 text-body-sm font-semibold">
      {label}
      {note && <span className="text-ui font-normal text-muted-foreground">{note}</span>}
    </h2>
  );
}

/** One host's group in Tasks across hosts, below that host's ApiProvider. */
export function HostTasksSection({
  hostId,
  label,
  view,
  selectedKey,
  onSelect,
  onSummary,
  onNotificationsChanged,
}: {
  hostId: string;
  label: string;
  /** The shared toolbar's filters; selection and expansion are this group's. */
  view: TasksWorkspaceView;
  /** The open task when it lives on this host. */
  selectedKey: string;
  onSelect: (key: string) => void;
  onSummary: (hostId: string, summary: HostTaskSummary) => void;
  onNotificationsChanged?: () => void;
}) {
  const api = useApi();
  const server = hostToParam(api.target?.id ?? "");
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [creatingParent, setCreatingParent] = useState<string | null>(null);
  const state: TasksWorkspaceView = {
    ...view,
    // The open task reads in the drawer, so the group asks for no detail.
    selectedKey: "",
    loadDetail: async (key: string) => onSelect(key),
    expanded,
    setExpanded,
    toggleExpanded: (key: string) =>
      setExpanded((current) => {
        const next = new Set(current);
        if (!next.delete(key)) next.add(key);
        return next;
      }),
    creatingParent,
    setCreatingParent,
  };
  const data = useTasksWorkspaceData(undefined, state, onNotificationsChanged);
  const actions = useTasksWorkspaceActions(server, state, data);
  useReportSummary(hostId, data, onSummary);

  const failure = data.principalMetadataError && !data.principalFilter;
  const error = failure
    ? data.principalMetadataError
    : data.treeQuery.error && !data.treeQuery.data
      ? errorText(data.treeQuery.error)
      : "";
  if (!api.ready) return <HostHeading label={label} note="Connecting…" />;
  if (error) {
    return (
      <section aria-label={label}>
        <HostHeading label={label} note="unavailable" />
        <p role="alert" className={NOTE}>
          {error}
        </p>
      </section>
    );
  }
  if (data.loading) return <HostHeading label={label} note="Loading tasks…" />;
  if (data.visibleTasks.length === 0 && !creatingParent) return null;
  return (
    <section aria-label={label}>
      <HostHeading label={label} note={String(data.visibleTasks.length)} />
      <TaskTree
        grouped
        tasks={data.visibleTasks}
        knownTasks={data.listedTasks}
        queues={data.queues}
        mode="all"
        activeQuestionTaskKeys={data.activeQuestionTaskKeys}
        expanded={expanded}
        selectedKey={selectedKey}
        onToggle={state.toggleExpanded}
        onSelect={onSelect}
        onAddChild={setCreatingParent}
        onMove={(key, parentKey, beforeKey) => void actions.performMove(key, parentKey, beforeKey)}
        onPriorityChange={actions.changePriority}
      />
      {creatingParent && (
        <TaskForm
          queues={data.queues}
          initialQueue={parentQueueOf(creatingParent, data.tasks)}
          parentKey={creatingParent}
          onCreate={actions.create}
          onCancel={() => setCreatingParent(null)}
        />
      )}
    </section>
  );
}

/** Hands the toolbar what it lists of this host whenever that changes. */
function useReportSummary(
  hostId: string,
  data: ReturnType<typeof useTasksWorkspaceData>,
  onSummary: (hostId: string, summary: HostTaskSummary) => void,
) {
  const { queueCounts, principals, loading, refreshing } = data;
  const queues = data.queues.length ? data.queues : NO_QUEUES;
  const shown = data.visibleTasks.length;
  const agents = useMemo(() => principals?.agents ?? [], [principals]);
  useEffect(() => {
    onSummary(hostId, { queues, queueCounts, agents, shown, loading, refreshing });
  }, [agents, hostId, loading, onSummary, queueCounts, queues, refreshing, shown]);
}
