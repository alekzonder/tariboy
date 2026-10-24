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

export function TasksWorkspaceContent({
  scopeAgent,
  server,
  state,
  data,
  actions,
  layout,
}: {
  scopeAgent?: string;
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
    expanded,
    selectedKey,
    startCreate,
    loadDetail,
    closeDetail,
    toggleExpanded,
  } = state;
  const {
    queues,
    principals,
    queueCounts,
    refreshing,
    principalFilter,
    principalMetadataError,
    loadMetadata,
    loadTree,
    treeQuery,
    loading,
    visibleTasks,
    activeQuestionTaskKeys,
    tasks,
    loadingKey,
    detailQuery,
    historyQuery,
    detail,
    events,
  } = data;
  const {
    create,
    performMove,
    changePriority,
    createQueue,
    updateQueue,
    saveDetail,
    comment,
    addRelation,
    transferDetail,
    requeueDetail,
    deleteRelation,
  } = actions;
  const detailResizeHandle = (
    <TaskPanelResizeHandle
      width={effectiveDetailWidth}
      maximum={detailMaximum}
      workspaceRef={workspaceRef}
      onResize={setDetailWidth}
    />
  );

  return (
    <div
      ref={workspaceRef}
      className="grid h-full min-h-0 grid-cols-[minmax(0,1fr)]"
      data-testid="tasks-workspace"
      data-scope-agent={scopeAgent ?? ""}
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
          onRefresh={() => {
            if (!scopeAgent && view !== "" && !principalFilter) void loadMetadata();
            else void loadTree();
          }}
          onCreate={startCreate}
        />
        {creatingParent === "" && (
          <TaskForm
            queues={queues}
            // A toolbar filter is the explicit context; otherwise the queue of the last task made here.
            initialQueue={
              queue ||
              queues.find((item) => item.prefix === readRememberedQueue(server))?.prefix ||
              ""
            }
            onCreate={create}
            onCancel={() => setCreatingParent(null)}
          />
        )}
        {principalMetadataError && !principalFilter ? (
          <div className="px-5 py-10 text-center text-body text-muted-foreground" role="alert">
            {principalMetadataError}
          </div>
        ) : treeQuery.error && !treeQuery.data ? (
          <div className="px-5 py-10 text-center text-body text-muted-foreground" role="alert">
            {errorText(treeQuery.error)}
          </div>
        ) : loading ? (
          <div className="px-5 py-10 text-center text-body text-muted-foreground">
            Loading tasks…
          </div>
        ) : (
          <TaskTree
            tasks={visibleTasks}
            mode={scopeAgent ? "agent" : "all"}
            activeQuestionTaskKeys={activeQuestionTaskKeys}
            expanded={expanded}
            selectedKey={selectedKey}
            onToggle={toggleExpanded}
            onSelect={(key) => void loadDetail(key)}
            onAddChild={setCreatingParent}
            onMove={(key, parentKey, beforeKey) => void performMove(key, parentKey, beforeKey)}
            onPriorityChange={changePriority}
          />
        )}
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
      {loadingKey ? (
        <TaskDetailLoading
          taskKey={loadingKey}
          error={
            detailQuery.error || historyQuery.error
              ? errorText(detailQuery.error ?? historyQuery.error)
              : undefined
          }
          width={effectiveDetailWidth}
          resizeHandle={detailResizeHandle}
          onClose={closeDetail}
        />
      ) : (
        detail && (
          <TaskDetail
            key={detail.task.key}
            detail={detail}
            principals={principals}
            width={effectiveDetailWidth}
            resizeHandle={detailResizeHandle}
            onClose={closeDetail}
            events={events}
            onSave={saveDetail}
            onComment={comment}
            onAddRelation={addRelation}
            onTransfer={transferDetail}
            onRequeue={requeueDetail}
            onDeleteRelation={deleteRelation}
          />
        )
      )}
    </div>
  );
}
