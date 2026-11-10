import { AlertTriangle, ChevronRight, Pencil } from "lucide-react";
import { Button } from "@/components/ui/button";
import { StatusPill } from "@/components/ui/status";
import { Textarea } from "@/components/ui/textarea";
import { taskTone } from "@/lib/statusTone";
import { taskStatusLabel } from "@/lib/tasks";
import { cn } from "@/lib/utils";
import { explainProblem, type Problem } from "@/pages/tasks/explainWorkflowProblem";
import { COUNT, EMPTY, LABEL, MONO, QUIET_ACTION } from "@/pages/tasks/panelStyles";
import { TaskCommentList } from "@/pages/tasks/TaskCommentList";
import {
  MarkdownContent,
  MarkdownEditor,
  MarkdownModeSegment,
  type MarkdownMode,
} from "@/pages/tasks/TaskMarkdown";
import { formatTaskTime } from "@/pages/tasks/taskTime";
import { isClosed, requiredBy, stepArtifacts } from "@/pages/tasks/workflowGraph";
import type { Task, TaskDetail, WorkflowView } from "@/types";

interface DescriptionProps {
  editing: boolean;
  value: string;
  mode: MarkdownMode;
  onEdit: () => void;
  onCancel: () => void;
  onChange: (value: string) => void;
  onModeChange: (mode: MarkdownMode) => void;
}

export interface TaskDrawerBodyProps {
  detail: TaskDetail;
  view?: WorkflowView;
  /** The artifacts whose value is open. */
  openArtifacts: ReadonlySet<string>;
  onToggleArtifact: (name: string) => void;
  description: DescriptionProps;
  comment: string;
  onCommentChange: (value: string) => void;
  onComment: () => void;
  onOpenTask: (key: string) => void;
  onProblemAction: (action: "retry" | "return" | "log") => void;
}

/** What is wrong, in full, as the first thing of the body: the title, the
 *  script's own words, what to do next, and the actions. */
function ProblemBlock({
  problem,
  onAction,
}: {
  problem: Problem;
  onAction: TaskDrawerBodyProps["onProblemAction"];
}) {
  const danger = problem.tone === "danger";
  return (
    <section
      role="alert"
      className={cn(
        "flex min-w-0 flex-col gap-1.5 rounded-md px-2.5 py-2",
        danger ? "bg-status-failed/12" : "bg-primary/10",
      )}
    >
      <div
        className={cn(
          "flex min-w-0 items-start gap-2 text-body-sm font-medium",
          danger ? "text-status-failed" : "text-primary",
        )}
      >
        <AlertTriangle aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
        <span className="min-w-0 flex-1">{problem.title}</span>
        {problem.counter && <span className={cn(MONO, "shrink-0")}>{problem.counter}</span>}
      </div>
      {problem.detail && (
        <p className="rounded-control bg-card px-2 py-1.5 font-mono text-meta break-words whitespace-pre-wrap">
          {problem.detail}
        </p>
      )}
      <p className="text-ui text-muted-foreground text-pretty">{problem.advice}</p>
      <div className="flex flex-wrap gap-1">
        {problem.run && (
          <Button variant="ghost" className={QUIET_ACTION} onClick={() => onAction("log")}>
            Show log of {problem.run.script}
          </Button>
        )}
        {problem.kind === "failed" && (
          <Button variant="ghost" className={QUIET_ACTION} onClick={() => onAction("retry")}>
            Retry
          </Button>
        )}
        <Button variant="ghost" className={QUIET_ACTION} onClick={() => onAction("return")}>
          Return to agent…
        </Button>
      </div>
    </section>
  );
}

/** The artifacts of the workflow, each closed to one line. Those the current
 *  decision needs are marked and come first. */
function Artifacts({
  view,
  open,
  onToggle,
}: {
  view: WorkflowView;
  open: ReadonlySet<string>;
  onToggle: (name: string) => void;
}) {
  const needed = isClosed(view)
    ? new Set<string>()
    : new Set([...stepArtifacts(view, view.status).enter.keys(), ...requiredBy(view).keys()]);
  const names = [...new Set([...needed, ...view.declared_artifacts.map((item) => item.name)])];
  if (!names.length) return null;
  return (
    <section className="flex min-w-0 flex-col gap-1">
      <div className="flex items-center gap-1.5">
        <span className={LABEL}>Artifacts</span>
        <span className={COUNT}>{view.artifacts.length}</span>
      </div>
      {names.map((name) => {
        const artifact = [...view.artifacts].reverse().find((item) => item.name === name);
        const shown = open.has(name);
        return (
          <div key={name} id={`artifact-${name}`} className="min-w-0 rounded-md bg-muted">
            <button
              type="button"
              aria-expanded={shown}
              onClick={() => onToggle(name)}
              className="flex w-full min-w-0 items-center gap-2 rounded-md px-2.5 py-1.5 text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <ChevronRight
                aria-hidden="true"
                className={cn("size-3 shrink-0 text-muted-foreground", shown && "rotate-90")}
              />
              <span className={cn(MONO, "font-medium")}>{name}</span>
              {needed.has(name) && view.owner === "customer" && (
                <span className="rounded-chip bg-primary/12 px-1.5 text-tiny font-medium text-primary">
                  for this decision
                </span>
              )}
              <span className="ml-auto shrink-0 font-mono text-caption tabular-nums text-muted-foreground">
                {artifact
                  ? `${artifact.author.replace(/^agent:/, "")} · ${formatTaskTime(artifact.created_at)}`
                  : "no value yet"}
              </span>
            </button>
            {shown && artifact && (
              <div className="max-h-96 overflow-auto px-3 pb-2.5 text-body-sm">
                <MarkdownContent>{artifact.value}</MarkdownContent>
              </div>
            )}
          </div>
        );
      })}
    </section>
  );
}

/** The description reads as a document. The editor replaces it on Edit. */
function Description({ description }: { description: DescriptionProps }) {
  const { editing, value } = description;
  return (
    <section className="flex min-w-0 flex-col gap-1.5">
      <div className="flex min-h-6 items-center gap-2">
        <span className={LABEL}>Description</span>
        {editing ? (
          <>
            <MarkdownModeSegment
              label="Description editing mode"
              mode={description.mode}
              onModeChange={description.onModeChange}
            />
            <Button
              variant="ghost"
              className={cn(QUIET_ACTION, "ml-auto")}
              onClick={description.onCancel}
            >
              Cancel
            </Button>
          </>
        ) : (
          <Button
            variant="ghost"
            className={cn(QUIET_ACTION, "ml-auto gap-1")}
            onClick={description.onEdit}
          >
            <Pencil aria-hidden="true" className="size-3" />
            {value ? "Edit" : "Add description"}
          </Button>
        )}
      </div>
      {editing && (
        <MarkdownEditor
          id="task-description"
          placeholder="Description"
          value={value}
          onChange={description.onChange}
          mode={description.mode}
        />
      )}
      {!editing && value && (
        <div className="min-w-0 text-body leading-[1.55]">
          <MarkdownContent>{value}</MarkdownContent>
        </div>
      )}
      {!editing && !value && <span className={EMPTY}>No description.</span>}
    </section>
  );
}

function Subtasks({ items, onOpen }: { items: Task[]; onOpen: (key: string) => void }) {
  if (!items.length) return null;
  return (
    <section className="flex min-w-0 flex-col gap-0.5">
      <div className="flex items-center gap-1.5">
        <span className={LABEL}>Subtasks</span>
        <span className={COUNT}>{items.length}</span>
      </div>
      {items.map((task) => (
        <button
          key={task.key}
          type="button"
          onClick={() => onOpen(task.key)}
          className="-mx-1.5 flex min-w-0 items-center gap-2 rounded-md px-2 py-1.5 text-left text-body outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
        >
          <span className={cn(MONO, "shrink-0 text-muted-foreground")}>{task.key}</span>
          <span className="min-w-0 flex-1 truncate">{task.title}</span>
          <StatusPill tone={taskTone(task.category)} size="sm">
            {taskStatusLabel(task)}
          </StatusPill>
        </button>
      ))}
    </section>
  );
}

/**
 * The content of the task under the sticky header, in the order it is read:
 * what is wrong, what the workflow produced, the description, the subtasks,
 * and the conversation. The log and the rare fields follow in `TaskDrawerMore`.
 */
export function TaskDrawerBody({
  detail,
  view,
  openArtifacts,
  onToggleArtifact,
  description,
  comment,
  onCommentChange,
  onComment,
  onOpenTask,
  onProblemAction,
}: TaskDrawerBodyProps) {
  const problem = view ? explainProblem(view) : null;
  const loud = problem && (problem.kind === "failed" || problem.kind === "rejected");
  return (
    <>
      {loud && <ProblemBlock problem={problem} onAction={onProblemAction} />}
      {view && <Artifacts view={view} open={openArtifacts} onToggle={onToggleArtifact} />}
      <Description description={description} />
      <Subtasks items={detail.children} onOpen={onOpenTask} />
      <section className="flex min-w-0 flex-col gap-1.5">
        <div className="flex items-center gap-1.5">
          <span className={LABEL}>Comments</span>
          <span className={COUNT}>{detail.comments.length}</span>
        </div>
        <div className="flex min-w-0 items-end gap-2">
          <Textarea
            aria-label="Comment"
            placeholder="Write a comment"
            value={comment}
            onChange={(event) => onCommentChange(event.target.value)}
            className="min-h-9 rounded-md border-0 bg-muted text-body-sm md:text-body-sm"
          />
          <Button size="sm" className="h-7" disabled={!comment.trim()} onClick={onComment}>
            Comment
          </Button>
        </div>
        <TaskCommentList comments={detail.comments} asking={new Set()} />
      </section>
    </>
  );
}
