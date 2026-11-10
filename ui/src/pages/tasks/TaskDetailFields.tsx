import { useId, useState, type Dispatch, type SetStateAction } from "react";
import { ExternalLink, Pencil } from "lucide-react";
import type { PrincipalInfo, Task, TaskStatus, TaskPriority } from "@/types";
import { isWorkflowTask, taskStatusLabel } from "@/lib/tasks";
import { TONE_FILL, taskTone } from "@/lib/statusTone";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { StatusPill } from "@/components/ui/status";
import { SendFilesButton } from "@/components/SendFilesButton";
import { cn } from "@/lib/utils";
import {
  MarkdownContent,
  MarkdownEditor,
  MarkdownModeSegment,
  type MarkdownMode,
} from "./TaskMarkdown";
import { CHIP, CHIP_VALUE, ChipSelect } from "./TaskDetailControls";
import { useTaskBudget } from "@/hooks/useTaskBudget";
import { fmtMoney } from "@/lib/metricsFormat";
import { COUNT, FIELD, FIELD_MONO, LABEL, QUIET_ACTION } from "./panelStyles";
import { openMarkdownLink } from "./taskMarkdownShared";
import { formatTaskTime } from "./taskTime";
import WorkflowAssignee from "./WorkflowAssignee";
import type { TaskDetailProps } from "./TaskDetailTypes";
import type { useTaskDetailDraft } from "./useTaskDetailDraft";

type Draft = ReturnType<typeof useTaskDetailDraft>;

/** The closed list of assignees: nobody, the customer as "me", and each
 *  agent. A current assignee outside the list stays shown but cannot be
 *  picked again, so no principal is ever typed. */
function assigneeChoices(principals: PrincipalInfo | null, current: string) {
  const choices = [{ value: "", label: "Unassigned", disabled: false }];
  if (principals?.customer)
    choices.push({ value: principals.customer, label: "me", disabled: false });
  for (const agent of principals?.agents ?? []) {
    const value = agent.includes(":") ? agent : `agent:${agent}`;
    choices.push({ value, label: value.replace(/^agent:/, ""), disabled: false });
  }
  if (current && !choices.some((choice) => choice.value === current))
    choices.push({ value: current, label: current, disabled: true });
  return choices;
}

/** The assignee of a task without a workflow, from the owner's closed list
 *  or from me and the agents. */
function AssigneeChip({
  task,
  draft,
  principals,
  assigneeOptions,
}: Pick<TaskDetailProps, "principals" | "assigneeOptions"> & { task: Task; draft: Draft }) {
  const { assignee, setAssignee } = draft;
  if (!assigneeOptions) {
    return (
      <ChipSelect label="Assignee" value={assignee} onChange={setAssignee}>
        {assigneeChoices(principals, task.assignee).map((choice) => (
          <option key={choice.value} value={choice.value} disabled={choice.disabled}>
            {choice.label}
          </option>
        ))}
      </ChipSelect>
    );
  }
  return (
    <ChipSelect label="Assignee" value={assignee} onChange={setAssignee}>
      <option value="">Unassigned</option>
      {[task.assignee, principals?.customer ?? ""]
        .filter(
          (value, index, all) =>
            value &&
            all.indexOf(value) === index &&
            !assigneeOptions.some((option) => option.value === value),
        )
        .map((value) => (
          <option key={value} value={value}>
            {value}
          </option>
        ))}
      {assigneeOptions.map((option) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </ChipSelect>
  );
}

/**
 * The parameters of the task as one row of chips, the closed Details block of
 * the header: the status, the priority, the assignee, the budget with what is
 * spent, the pull request as a link, and when the task last changed. A chip
 * changes its value in place and the change joins the draft, as a field does.
 */
export function TaskDetailChips({
  task,
  draft,
  principals,
  assigneeOptions,
  onTaskChanged,
}: Pick<TaskDetailProps, "principals" | "assigneeOptions" | "onTaskChanged"> & {
  task: Task;
  draft: Draft;
}) {
  const workflow = isWorkflowTask(task);
  const editable = task.access !== "context" && task.access !== "respond";
  if (!editable) {
    return (
      <div className="flex min-w-0 flex-1 flex-wrap items-center gap-1.5 text-ui">
        <StatusPill tone={taskTone(task.category)}>{taskStatusLabel(task)}</StatusPill>
        <span className={CHIP}>
          assignee <span className={CHIP_VALUE}>{task.assignee || "unassigned"}</span>
        </span>
        <Updated at={task.updated_at} />
      </div>
    );
  }
  return (
    <fieldset
      className="flex min-w-0 flex-1 flex-wrap items-center gap-1.5 text-ui"
      disabled={draft.saving}
      aria-label="Task parameters"
    >
      {workflow ? (
        <StatusPill tone={taskTone(task.category)}>{taskStatusLabel(task)}</StatusPill>
      ) : (
        <ChipSelect
          label="Status"
          bare
          className={TONE_FILL[taskTone(draft.status)]}
          value={draft.status}
          onChange={(value) => draft.setStatus(value as TaskStatus)}
        >
          <option value="open">Open</option>
          <option value="in_progress">In progress</option>
          <option value="wait_customer">Waiting for you</option>
          <option value="done">Done</option>
          <option value="cancelled">Cancelled</option>
        </ChipSelect>
      )}
      <ChipSelect
        label="Priority"
        bare
        value={draft.priority}
        onChange={(value) => draft.setPriority(value as TaskPriority)}
      >
        <option value="P0">P0 Critical</option>
        <option value="P1">P1 High</option>
        <option value="P2">P2 Normal</option>
        <option value="P3">P3 Low</option>
      </ChipSelect>
      {workflow ? (
        <WorkflowAssignee task={task} onTaskChanged={onTaskChanged} />
      ) : (
        <AssigneeChip {...{ task, draft, principals, assigneeOptions }} />
      )}
      <TaskBudgetChip task={task} value={draft.budget} onChange={draft.setBudget} />
      <PullRequestChip link={task.pull_request ?? ""} />
      <Updated at={task.updated_at} />
    </fieldset>
  );
}

function Updated({ at }: { at: string }) {
  return (
    <span className="ml-auto inline-flex shrink-0 items-center gap-1.5 text-muted-foreground">
      updated
      <time dateTime={at} className={CHIP_VALUE}>
        {formatTaskTime(at)}
      </time>
    </span>
  );
}

/** `#318` out of a pull request link; the link without its scheme otherwise. */
function pullRequestLabel(link: string): string {
  const number = /\/pull\/(\d+)/.exec(link)?.[1];
  return number ? `#${number}` : link.replace(/^https?:\/\//, "");
}

/** The saved pull request as a link. It is edited in the open Details block. */
function PullRequestChip({ link }: { link: string }) {
  if (!/^https?:\/\//.test(link)) return null;
  return (
    <a
      href={link}
      target="_blank"
      rel="noreferrer"
      title={link}
      onClick={(event) => openMarkdownLink(event, link)}
      className={cn(
        CHIP,
        "outline-none hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring",
      )}
    >
      PR <span className={CHIP_VALUE}>{pullRequestLabel(link)}</span>
      <ExternalLink aria-hidden="true" className="size-3 shrink-0 opacity-60" />
    </a>
  );
}

/** The customer's AI spending limit for the task and its subtree, with what
 *  they spent so far. An empty field means no limit. */
function TaskBudgetChip({
  task,
  value,
  onChange,
}: {
  task: Task;
  value: string;
  onChange: (value: string) => void;
}) {
  const budget = useTaskBudget(task);
  const spentId = useId();
  return (
    <span className={CHIP}>
      <span className="flex min-w-0 items-center gap-1.5">
        budget $
        <Input
          aria-label="Budget, USD"
          className={cn(
            CHIP_VALUE,
            "h-5 w-18 rounded-sm border-0 bg-transparent px-0.5 shadow-none focus-visible:ring-0 md:text-meta",
          )}
          type="number"
          min={0}
          step="0.01"
          inputMode="decimal"
          placeholder="No limit"
          aria-describedby={budget ? spentId : undefined}
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
      </span>
      {budget && (
        <span
          id={spentId}
          className={cn(COUNT, "shrink-0", budget.exhausted && "text-status-failed opacity-100")}
        >
          Spent {fmtMoney(budget.spent_usd)}
          {budget.budget_usd > 0 && ` of ${fmtMoney(budget.budget_usd)}`}
          {budget.exhausted && " · exhausted"}
        </span>
      )}
    </span>
  );
}

/** The fields of the open Details block: the pull request link and the manual
 *  block reason. */
export function TaskDetailProperties({ draft }: { draft: Draft }) {
  const { saving, pullRequest, setPullRequest, blockReason, setBlockReason } = draft;
  return (
    <fieldset className="contents" disabled={saving}>
      <div className="task-properties grid min-w-0 grid-cols-2 gap-x-4 gap-y-2.5">
        <label className="flex min-w-0 flex-col gap-1.25">
          <span className={LABEL}>Pull request</span>
          <Input
            className={cn(FIELD, FIELD_MONO)}
            value={pullRequest}
            onChange={(event) => setPullRequest(event.target.value)}
          />
        </label>
        <label className="flex min-w-0 flex-col gap-1.25">
          <span className={LABEL}>Manual block reason</span>
          <Input
            className={FIELD}
            value={blockReason}
            onChange={(event) => setBlockReason(event.target.value)}
          />
        </label>
      </div>
    </fieldset>
  );
}

/**
 * The description. It reads as a document; Edit puts it in the editor, and a
 * task with no description starts in the editor. An unsaved change keeps the
 * editor open, and a save returns to the text.
 */
export function TaskDescription({
  draft,
  descriptionMode,
  setDescriptionMode,
  setUploadingDescription,
}: {
  draft: Draft;
  descriptionMode: MarkdownMode;
  setDescriptionMode: (mode: MarkdownMode) => void;
  setUploadingDescription: Dispatch<SetStateAction<boolean>>;
}) {
  const { saving, description, setDescription, baseline } = draft;
  const [opened, setOpened] = useState(false);
  const [wasSaving, setWasSaving] = useState(saving);
  const changed = description !== baseline.description;
  if (wasSaving !== saving) {
    setWasSaving(saving);
    if (!saving && !changed) setOpened(false);
  }
  const editing = opened || changed || !baseline.description;
  return (
    <fieldset className="contents" disabled={saving}>
      <div className="task-description grid min-w-0 grid-cols-[minmax(0,1fr)] gap-1.75">
        <div className="flex min-h-6 items-center gap-2">
          {editing ? (
            <label htmlFor="task-description" className={LABEL}>
              Description
            </label>
          ) : (
            <span className={LABEL}>Description</span>
          )}
          {editing ? (
            <DescriptionTools
              draft={draft}
              mode={descriptionMode}
              onModeChange={setDescriptionMode}
              setUploading={setUploadingDescription}
              onCancel={() => {
                setDescription(baseline.description);
                setOpened(false);
              }}
            />
          ) : (
            <Button
              type="button"
              variant="ghost"
              className={cn(QUIET_ACTION, "ml-auto gap-1")}
              onClick={() => setOpened(true)}
            >
              <Pencil aria-hidden="true" className="size-3" />
              Edit description
            </Button>
          )}
        </div>
        {editing ? (
          <MarkdownEditor
            id="task-description"
            placeholder="Description"
            value={description}
            onChange={setDescription}
            disabled={saving}
            mode={descriptionMode}
          />
        ) : (
          <div className="min-w-0 text-body">
            <MarkdownContent>{description}</MarkdownContent>
          </div>
        )}
      </div>
    </fieldset>
  );
}

/** The editor's row: the mode, the file upload, and the way back to the text. */
function DescriptionTools({
  draft,
  mode,
  onModeChange,
  setUploading,
  onCancel,
}: {
  draft: Draft;
  mode: MarkdownMode;
  onModeChange: (mode: MarkdownMode) => void;
  setUploading: Dispatch<SetStateAction<boolean>>;
  onCancel: () => void;
}) {
  return (
    <>
      <MarkdownModeSegment
        label="Description editing mode"
        mode={mode}
        onModeChange={onModeChange}
        disabled={draft.saving}
      />
      <SendFilesButton
        disabled={draft.saving}
        onUploadingChange={setUploading}
        className={cn(QUIET_ACTION, "ml-auto border-0 bg-transparent")}
        onUploaded={(paths) =>
          draft.setDescription(
            (current) =>
              current + (current && !current.endsWith("\n") ? "\n" : "") + paths.join("\n"),
          )
        }
      />
      {/* Nothing to go back to while the task has no saved description. */}
      {draft.baseline.description && (
        <Button type="button" variant="ghost" className={QUIET_ACTION} onClick={onCancel}>
          Cancel editing
        </Button>
      )}
    </>
  );
}
