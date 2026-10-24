import type { Dispatch, SetStateAction } from "react";
import type { Task, TaskStatus, TaskPriority } from "@/lib/api";
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

export function TaskDetailFields({
  task,
  draft,
  principals,
  assigneeOptions,
  onTaskChanged,
  descriptionMode,
  setDescriptionMode,
  setUploadingDescription,
}: Pick<TaskDetailProps, "principals" | "assigneeOptions" | "onTaskChanged"> & {
  task: Task;
  draft: ReturnType<typeof useTaskDetailDraft>;
  descriptionMode: MarkdownMode;
  setDescriptionMode: (mode: MarkdownMode) => void;
  setUploadingDescription: Dispatch<SetStateAction<boolean>>;
}) {
  const workflow = isWorkflowTask(task);
  const {
    saving,
    title,
    setTitle,
    description,
    setDescription,
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
      <label className="flex min-w-0 flex-col gap-1.5">
        <span className={LABEL}>Title</span>
        <Input className={FIELD} value={title} onChange={(event) => setTitle(event.target.value)} />
      </label>
      <div className="task-description flex min-w-0 flex-col gap-[7px]">
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
      <div className="task-properties grid min-w-0 grid-cols-2 gap-x-4 gap-y-2.5">
        {!workflow && (
          <SelectField
            label="Status"
            value={status}
            onChange={(value) => setStatus(value as TaskStatus)}
          >
            <option value="open">Open</option>
            <option value="in_progress">In progress</option>
            <option value="wait_customer">Wait customer</option>
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
          <label className="flex min-w-0 flex-col gap-[5px]">
            <span className={LABEL}>Assignee</span>
            <Input
              aria-label="Assignee"
              list="task-assignees"
              className={cn(FIELD, FIELD_MONO)}
              value={assignee}
              onChange={(event) => setAssignee(event.target.value)}
            />
            <datalist id="task-assignees">
              {principals?.agents.map((agent) => (
                <option key={agent} value={agent} />
              ))}
            </datalist>
          </label>
        )}
        <label className="flex min-w-0 flex-col gap-[5px]">
          <span className={LABEL}>Pull request</span>
          <Input
            className={cn(FIELD, FIELD_MONO)}
            value={pullRequest}
            onChange={(event) => setPullRequest(event.target.value)}
          />
        </label>
        <label className="flex min-w-0 flex-col gap-[5px]">
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
