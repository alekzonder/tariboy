import type { ReactNode } from "react";
import type { Queue } from "@/lib/api";
import { errorText } from "@/lib/errors";
import QueueSettingsDialog from "./QueueSettingsDialog";
import { TaskFilterBar } from "./TaskFilterBar";
import TaskDetail, { TaskDetailLoading } from "./TaskDetail";
import TaskForm from "./TaskForm";
import TaskTree from "./TaskTree";
import TaskPanelResizeHandle from "./TaskPanelResizeHandle";
import { readRememberedQueue } from "./taskQueueMemory";
import type { TasksWorkspaceView } from "./useTasksWorkspaceView";
import type { TasksWorkspaceData } from "./useTasksWorkspaceData";
import type { TasksWorkspaceActions } from "./useTasksWorkspaceActions";
import type { useTasksWorkspaceLayout } from "./useTasksWorkspaceLayout";

// A toolbar filter is the explicit context; otherwise the queue of the last task made here.
const newTaskQueue = (queue: string, queues: Queue[], server: string) =>
  queue || queues.find((item) => item.prefix === readRememberedQueue(server))?.prefix || "";

export function TasksWorkspaceContent({
  scopeAgent,
  goalTaskKey,
  server,
  state,
  data,
  actions,
  layout,
}: {
  scopeAgent?: string;
  goalTaskKey?: string;
  server: string;
  state: TasksWorkspaceView;
  data: TasksWorkspaceData;
  actions: TasksWorkspaceActions;
  layout: ReturnType<typeof useTasksWorkspaceLayout>;
}) {
  const { workspaceRef, effectiveDetailWidth, detailMaximum, setDetailWidth } = layout;
  const {
    query,
    setQuery,
    statusView,
    setStatusView,
    view,
    setView,
    queue,
    selectQueue,
    queuesOpen,
    setQueuesOpen,
    creatingParent,
    setCreatingParent,
    startCreate,
    closeDetail,
  } = state;
  const { queues, queueCounts, refreshing, principalFilter, loadMetadata, loadTree, tasks } = data;
  const { create, createQueue, updateQueue } = actions;
  const detailResizeHandle = (
    <TaskPanelResizeHandle
      width={effectiveDetailWidth}
      maximum={detailMaximum}
      workspaceRef={workspaceRef}
      onResize={setDetailWidth}
    />
  );
  const refresh = () => {
    if (!scopeAgent && view !== "" && !principalFilter) void loadMetadata();
    else void loadTree();
  };

  return (
    <div
      ref={workspaceRef}
      className="grid h-full min-h-0 grid-cols-[minmax(0,1fr)]"
      data-testid="tasks-workspace"
      data-scope-agent={scopeAgent ?? ""}
      data-goal-task-key={goalTaskKey ?? ""}
    >
      <main className="min-h-0 min-w-0 overflow-auto">
        <TaskFilterBar
          mode={scopeAgent ? "agent" : "all"}
          query={query}
          onQuery={setQuery}
          statusView={statusView}
          onStatusView={setStatusView}
          principalFilter={view}
          onPrincipalFilter={setView}
          queues={queues}
          queue={queue}
          onQueue={selectQueue}
          queueCounts={queueCounts}
          onManageQueues={() => setQueuesOpen(true)}
          scopeAgent={scopeAgent}
          refreshing={refreshing}
          onRefresh={refresh}
          onCreate={startCreate}
        />
        {creatingParent === "" && (
          <TaskForm
            queues={queues}
            initialQueue={newTaskQueue(queue, queues, server)}
            onCreate={create}
            onCancel={() => setCreatingParent(null)}
          />
        )}
        <TasksWorkspaceList
          scopeAgent={scopeAgent}
          goalTaskKey={goalTaskKey}
          state={state}
          data={data}
          actions={actions}
        />
        {creatingParent && (
          <TaskForm
            queues={queues}
            initialQueue={tasks.find((task) => task.key === creatingParent)?.queue || queue}
            parentKey={creatingParent}
            onCreate={create}
            onCancel={() => setCreatingParent(null)}
          />
        )}
      </main>
      <QueueSettingsDialog
        open={queuesOpen}
        onOpenChange={setQueuesOpen}
        queues={queues}
        onCreate={createQueue}
        onUpdate={updateQueue}
      />
      <TasksWorkspaceDetail
        data={data}
        actions={actions}
        width={effectiveDetailWidth}
        resizeHandle={detailResizeHandle}
        onClose={closeDetail}
      />
    </div>
  );
}

/** The task tree, or why it is not there yet. */
function TasksWorkspaceList({
  scopeAgent,
  goalTaskKey,
  state,
  data,
  actions,
}: {
  scopeAgent?: string;
  goalTaskKey?: string;
  state: TasksWorkspaceView;
  data: TasksWorkspaceData;
  actions: TasksWorkspaceActions;
}) {
  const { expanded, selectedKey, loadDetail, setCreatingParent, toggleExpanded } = state;
  const { principalMetadataError, principalFilter, treeQuery, loading } = data;
  const { performMove, changePriority } = actions;
  if (principalMetadataError && !principalFilter) {
    return (
      <div className="px-5 py-10 text-center text-body text-muted-foreground" role="alert">
        {principalMetadataError}
      </div>
    );
  }
  if (treeQuery.error && !treeQuery.data) {
    return (
      <div className="px-5 py-10 text-center text-body text-muted-foreground" role="alert">
        {errorText(treeQuery.error)}
      </div>
    );
  }
  if (loading) {
    return (
      <div className="px-5 py-10 text-center text-body text-muted-foreground">Loading tasks…</div>
    );
  }
  return (
    <TaskTree
      tasks={data.visibleTasks}
      mode={scopeAgent ? "agent" : "all"}
      activeQuestionTaskKeys={data.activeQuestionTaskKeys}
      expanded={expanded}
      selectedKey={selectedKey}
      goalTaskKey={goalTaskKey}
      onToggle={toggleExpanded}
      onSelect={(key) => void loadDetail(key)}
      onAddChild={setCreatingParent}
      onMove={(key, parentKey, beforeKey) => void performMove(key, parentKey, beforeKey)}
      onPriorityChange={changePriority}
    />
  );
}

/** The open task's panel, or its loading state. */
function TasksWorkspaceDetail({
  data,
  actions,
  width,
  resizeHandle,
  onClose,
}: {
  data: TasksWorkspaceData;
  actions: TasksWorkspaceActions;
  width: number;
  resizeHandle: ReactNode;
  onClose: () => void;
}) {
  const { loadingKey, detailQuery, historyQuery, detail, principals, events } = data;
  if (loadingKey) {
    return (
      <TaskDetailLoading
        taskKey={loadingKey}
        error={
          detailQuery.error || historyQuery.error
            ? errorText(detailQuery.error ?? historyQuery.error)
            : undefined
        }
        width={width}
        resizeHandle={resizeHandle}
        onClose={onClose}
      />
    );
  }
  if (!detail) return null;
  return (
    <TaskDetail
      key={detail.task.key}
      detail={detail}
      principals={principals}
      width={width}
      resizeHandle={resizeHandle}
      onClose={onClose}
      events={events}
      onSave={actions.saveDetail}
      onComment={actions.comment}
      onAddRelation={actions.addRelation}
      onTransfer={actions.transferDetail}
      onRequeue={actions.requeueDetail}
      onDeleteRelation={actions.deleteRelation}
    />
  );
}
