import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { ChevronDown } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useApiQuery } from "@/hooks/useApiQuery";
import type { AgentRow, QueueWorkflow, TaskPriority } from "@/lib/api";
import { errorText } from "@/lib/errors";
import { idempotencyKey } from "@/lib/ids";
import { hostToParam } from "@/lib/terminalsHost";
import { cn } from "@/lib/utils";
import { SelectField } from "@/pages/tasks/TaskDetailControls";
import { MarkdownEditor } from "@/pages/tasks/TaskMarkdown";
import { taskQueries, writeTask } from "@/pages/tasks/taskQueries";
import { readAgentQueue, rememberQueue } from "@/pages/tasks/taskQueueMemory";
import { composerRequest, defaultQueue } from "./workModel";

const NOTE = "text-caption text-muted-foreground";

/** The bound queue's line after a create: the task is not this agent's until
 *  the workflow assigns it, so the line is the way to it. */
type Dispatched = { key: string; workflow: string };

/**
 * One line that gives the agent a task: Enter creates it in the agent's queue,
 * assigned to the agent unless the queue's workflow dispatches it. Details
 * reveals the description, the priority and the queue.
 */
export function TaskComposer({
  agent,
  base,
  focus,
  refresh,
  onOpenTask,
}: {
  agent: AgentRow;
  base: string;
  focus: boolean;
  refresh: () => void;
  onOpenTask: (key: string) => void;
}) {
  const draft = useComposerDraft(agent, refresh);
  const [details, setDetails] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (focus) input.current?.focus();
  }, [focus]);
  const { title, busy, binding, dispatched, error } = draft;

  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <div className="flex min-w-0 items-center gap-1.5">
        <Input
          ref={input}
          aria-label={`New task for ${agent.name}`}
          placeholder={`Give ${agent.name} a task…`}
          value={title}
          disabled={busy}
          maxLength={500}
          onChange={(event) => draft.setTitle(event.target.value)}
          onKeyDown={(event) => {
            if (event.key !== "Enter" || event.shiftKey || event.nativeEvent.isComposing) return;
            event.preventDefault();
            void draft.submit();
          }}
          className="h-8 min-w-0 flex-1"
        />
        <Button
          type="button"
          size="sm"
          disabled={busy || !title.trim()}
          onClick={() => void draft.submit()}
        >
          {agent.enabled ? "Give" : "Start & give"}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          aria-expanded={details}
          onClick={() => setDetails((open) => !open)}
        >
          Details
          <ChevronDown aria-hidden="true" className={cn("size-3", details && "rotate-180")} />
        </Button>
      </div>
      {details && <ComposerDetails draft={draft} />}
      <ComposerNotes agent={agent} base={base} binding={binding} />
      {dispatched && (
        <Button
          type="button"
          variant="link"
          className="h-auto self-start p-0 text-caption"
          onClick={() => onOpenTask(dispatched.key)}
        >
          {dispatched.key} dispatched by the {dispatched.workflow} workflow
        </Button>
      )}
      {error && (
        <p role="alert" className="text-ui text-destructive">
          {error}
        </p>
      )}
    </div>
  );
}

/** The draft, the queue it goes to, and the create. */
function useComposerDraft(agent: AgentRow, refresh: () => void) {
  const api = useApi();
  const client = useQueryClient();
  const server = hostToParam(api.target?.id ?? "");
  const queues = useApiQuery(taskQueries.queues(api)).data?.queues ?? [];
  const [picked, setPicked] = useState("");
  const queue = picked || defaultQueue(readAgentQueue(server, agent.name), queues);
  const binding = useApiQuery(taskQueries.queueWorkflow(api, queue)).data ?? null;
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [priority, setPriority] = useState<TaskPriority>("P2");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [dispatched, setDispatched] = useState<Dispatched | null>(null);
  // One key per draft: a retry after a lost answer makes no second task.
  const draftKey = useRef(idempotencyKey("task"));

  // A failed start keeps the task: it is created and waits for the agent.
  const start = async (key: string) => {
    try {
      await api.agents.startAgent(agent.name);
    } catch (failure) {
      setError(`${key} is created, but the agent did not start: ${errorText(failure)}`);
    } finally {
      refresh();
    }
  };
  const submit = async () => {
    if (busy || !title.trim()) return;
    setBusy(true);
    setError("");
    try {
      const request = composerRequest({
        title,
        description,
        priority,
        queue,
        agent: agent.name,
        bound: !!binding,
        idempotencyKey: draftKey.current,
      });
      const created = await writeTask(client, api, () => api.tasks.createTask(request));
      draftKey.current = idempotencyKey("task");
      rememberQueue(server, queue, agent.name);
      setTitle("");
      setDescription("");
      setDispatched(binding ? { key: created.key, workflow: binding.name } : null);
      if (!agent.enabled) await start(created.key);
    } catch (failure) {
      setError(errorText(failure));
    } finally {
      setBusy(false);
    }
  };
  return {
    queues,
    queue,
    setQueue: setPicked,
    binding,
    title,
    setTitle,
    description,
    setDescription,
    priority,
    setPriority,
    busy,
    error,
    dispatched,
    submit,
  };
}

function ComposerDetails({ draft }: { draft: ReturnType<typeof useComposerDraft> }) {
  const prefixes = [...new Set([draft.queue, ...draft.queues.map((item) => item.prefix)])];
  return (
    <div className="grid min-w-0 grid-cols-[minmax(0,1fr)] gap-2 rounded-md bg-muted p-2.5 sm:grid-cols-2">
      <SelectField label="Queue" value={draft.queue} onChange={draft.setQueue} mono>
        {prefixes.map((prefix) => (
          <option key={prefix} value={prefix}>
            {prefix}
          </option>
        ))}
      </SelectField>
      <SelectField
        label="Priority"
        value={draft.priority}
        onChange={(value) => draft.setPriority(value as TaskPriority)}
        mono
      >
        <option value="P0">P0 Critical</option>
        <option value="P1">P1 High</option>
        <option value="P2">P2 Normal</option>
        <option value="P3">P3 Low</option>
      </SelectField>
      <div className="min-w-0 sm:col-span-2">
        <MarkdownEditor
          id="composer-description"
          placeholder="Description"
          value={draft.description}
          onChange={draft.setDescription}
          disabled={draft.busy}
          mode="rich"
          surface="card"
        />
      </div>
    </div>
  );
}

/** What happens to a task given here, when it is not the plain case. */
function ComposerNotes({
  agent,
  base,
  binding,
}: {
  agent: AgentRow;
  base: string;
  binding: QueueWorkflow | null;
}) {
  return (
    <>
      {binding && (
        <p className={NOTE}>Dispatched by the {binding.name} workflow; a pool member takes it</p>
      )}
      {!agent.goal_enabled && (
        <p className={NOTE}>
          Goal is off: this agent does not pick up tasks.{" "}
          <Link to={`${base}/settings`}>Turn it on in Settings</Link>
        </p>
      )}
    </>
  );
}
