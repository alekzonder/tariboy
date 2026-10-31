import type { Dispatch, SetStateAction } from "react";
import type { PrincipalInfo, Task, TaskStatus, TaskPriority } from "@/lib/api";
import { isWorkflowTask } from "@/lib/tasks";
import { Input } from "@/components/ui/input";
import { SendFilesButton } from "@/components/SendFilesButton";
import { cn } from "@/lib/utils";
import { MarkdownEditor, MarkdownModeSegment, type MarkdownMode } from "./TaskMarkdown";
import { SelectField } from "./TaskDetailControls";
import { FIELD, FIELD_MONO, LABEL, QUIET_ACTION } from "./panelStyles";
import WorkflowAssignee from "./WorkflowAssignee";
import type { TaskDetailProps } from "./TaskDetailTypes";
import type { useTaskDetailDraft } from "./useTaskDetailDraft";

/** The title and the description: what the drawer shows first. */
export function TaskDetailFields({
  draft,
  descriptionMode,
  setDescriptionMode,
  setUploadingDescription,
}: {
  draft: ReturnType<typeof useTaskDetailDraft>;
  descriptionMode: MarkdownMode;
  setDescriptionMode: (mode: MarkdownMode) => void;
  setUploadingDescription: Dispatch<SetStateAction<boolean>>;
}) {
  const { saving, description, setDescription } = draft;
  return (
    <fieldset className="contents" disabled={saving}>
      <TaskDescriptionField
        saving={saving}
        description={description}
        setDescription={setDescription}
        descriptionMode={descriptionMode}
        setDescriptionMode={setDescriptionMode}
        setUploadingDescription={setUploadingDescription}
      />
    </fieldset>
  );
}

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

/** Status, priority, assignee, pull request and block reason. */
export function TaskDetailProperties({
  task,
  draft,
  principals,
  assigneeOptions,
  onTaskChanged,
}: Pick<TaskDetailProps, "principals" | "assigneeOptions" | "onTaskChanged"> & {
  task: Task;
  draft: ReturnType<typeof useTaskDetailDraft>;
}) {
  const workflow = isWorkflowTask(task);
  const {
    saving,
    status,
    setStatus,
    priority,
    setPriority,
    assignee,
    setAssignee,
    pullRequest,
    setPullRequest,
    blockReason,
    setBlockReason,
  } = draft;
  return (
    <fieldset className="contents" disabled={saving}>
      <div className="task-properties grid min-w-0 grid-cols-2 gap-x-4 gap-y-2.5">
        {!workflow && (
          <SelectField
            label="Status"
            value={status}
            onChange={(value) => setStatus(value as TaskStatus)}
          >
            <option value="open">Open</option>
            <option value="in_progress">In progress</option>
            <option value="wait_customer">Waiting for you</option>
            <option value="done">Done</option>
            <option value="cancelled">Cancelled</option>
          </SelectField>
        )}
        <SelectField
          label="Priority"
          value={priority}
          onChange={(value) => setPriority(value as TaskPriority)}
          mono
        >
          <option value="P0">P0 Critical</option>
          <option value="P1">P1 High</option>
          <option value="P2">P2 Normal</option>
          <option value="P3">P3 Low</option>
        </SelectField>
        {workflow && <WorkflowAssignee task={task} onTaskChanged={onTaskChanged} />}
        {!workflow && assigneeOptions && (
          <SelectField label="Assignee" value={assignee} onChange={setAssignee} mono>
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
          </SelectField>
        )}
        {!workflow && !assigneeOptions && (
          <SelectField label="Assignee" value={assignee} onChange={setAssignee} mono>
            {assigneeChoices(principals, task.assignee).map((choice) => (
              <option key={choice.value} value={choice.value} disabled={choice.disabled}>
                {choice.label}
              </option>
            ))}
          </SelectField>
        )}
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

function TaskDescriptionField({
  saving,
  description,
  setDescription,
  descriptionMode,
  setDescriptionMode,
  setUploadingDescription,
}: Pick<ReturnType<typeof useTaskDetailDraft>, "saving" | "description" | "setDescription"> & {
  descriptionMode: MarkdownMode;
  setDescriptionMode: (mode: MarkdownMode) => void;
  setUploadingDescription: Dispatch<SetStateAction<boolean>>;
}) {
  return (
    <div className="task-description grid min-w-0 grid-cols-[minmax(0,1fr)] gap-1.75">
      <div className="flex items-center gap-2">
        <label htmlFor="task-description" className={LABEL}>
          Description
        </label>
        <MarkdownModeSegment
          label="Description editing mode"
          mode={descriptionMode}
          onModeChange={setDescriptionMode}
          disabled={saving}
        />
        <SendFilesButton
          disabled={saving}
          onUploadingChange={setUploadingDescription}
          className={cn(QUIET_ACTION, "ml-auto border-0 bg-transparent")}
          onUploaded={(paths) =>
            setDescription(
              (current) =>
                current + (current && !current.endsWith("\n") ? "\n" : "") + paths.join("\n"),
            )
          }
        />
      </div>
      <MarkdownEditor
        id="task-description"
        placeholder="Description"
        value={description}
        onChange={setDescription}
        disabled={saving}
        mode={descriptionMode}
      />
    </div>
  );
}
