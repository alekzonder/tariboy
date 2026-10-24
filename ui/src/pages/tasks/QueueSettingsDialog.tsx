import { useCallback, useEffect, useId, useRef, useState } from "react";
import { Plus } from "lucide-react";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import type {
  AgentPool,
  ApiClient,
  CreateQueueInput,
  TaskQueue,
  UpdateQueueInput,
} from "@/lib/api";
import { cn } from "@/lib/utils";
import { EMPTY, MONO } from "./panelStyles";
import { NewQueuePane, QueueDetail } from "./QueueSettingsDetail";
import {
  queueSummary,
  queueWarns,
  requiredWithMissing,
  shownTab,
  type QueueEntry,
  type QueueState,
  type QueueTab,
} from "./queueSettingsModel";
import { errorText } from "@/lib/errors";

async function loadQueue(prefix: string, api: ApiClient): Promise<QueueState> {
  const [binding, pools] = await Promise.all([
    api.workflows.getQueueWorkflow(prefix),
    api.tasks.listAgentPools(prefix),
  ]);
  // A bound digest stays readable: the daemon refuses to remove a bound image.
  const definition = binding
    ? (await api.workflows.getWorkflowImage(binding.name, binding.digest)).definition
    : null;
  return { binding, definition, pools: pools.items ?? [] };
}

/**
 * Queue settings in one large dialog: the queues on the left with their
 * workflow and a warning for a required pool without agents, the selected
 * queue on the right in tabs. On a phone the list and the queue take turns.
 * Closing the dialog drops every unsaved draft.
 */
export default function QueueSettingsDialog({
  open,
  onOpenChange,
  ...props
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  queues: TaskQueue[];
  onCreate: (input: CreateQueueInput) => Promise<void>;
  onUpdate: (prefix: string, input: UpdateQueueInput) => Promise<void>;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex h-[min(760px,calc(100dvh-2rem))] w-[min(1100px,calc(100vw-2rem))] max-w-none flex-col gap-0 overflow-hidden p-0 sm:max-w-none max-sm:h-dvh max-sm:w-screen max-sm:rounded-none">
        <div className="border-b px-4 py-3 pr-12">
          <DialogTitle className="text-title font-semibold">Queues</DialogTitle>
          <DialogDescription className="sr-only">
            Create and rename queues, bind a workflow, and fill its agent pools and environment.
          </DialogDescription>
        </div>
        <QueueSettings {...props} />
      </DialogContent>
    </Dialog>
  );
}

function QueueSettings({
  queues,
  onCreate,
  onUpdate,
}: {
  queues: TaskQueue[];
  onCreate: (input: CreateQueueInput) => Promise<void>;
  onUpdate: (prefix: string, input: UpdateQueueInput) => Promise<void>;
}) {
  const api = useApi();
  const [selected, setSelected] = useState(queues[0]?.prefix ?? "");
  const [creating, setCreating] = useState(false);
  // On a phone the list shows until a queue is chosen.
  const [showDetail, setShowDetail] = useState(false);
  const [tab, setTab] = useState<QueueTab>("general");
  const [entries, setEntries] = useState<Record<string, QueueEntry>>({});
  const [agents, setAgents] = useState<string[]>([]);
  const [poolsMissing, setPoolsMissing] = useState<{ queue: string; names: string[] } | null>(null);
  const [secretsMissing, setSecretsMissing] = useState<{
    queue: string;
    names: string[];
    message: string;
  } | null>(null);
  const requested = useRef(new Set<string>());
  const agentList = useId();

  const load = useCallback(
    async (prefix: string) => {
      try {
        const state = await loadQueue(prefix, api);
        setEntries((current) => ({ ...current, [prefix]: { state } }));
      } catch (err) {
        setEntries((current) => ({ ...current, [prefix]: { error: errorText(err) } }));
      }
    },
    [api],
  );

  useEffect(() => {
    const fresh = queues
      .map((queue) => queue.prefix)
      .filter((prefix) => !requested.current.has(prefix));
    fresh.forEach((prefix) => requested.current.add(prefix));
    void Promise.resolve().then(() => Promise.all(fresh.map(load)));
  }, [queues, load]);
  useEffect(() => {
    // Without the agent list the agent fields stay plain inputs.
    void api.tasks.listTaskPrincipals().then(
      (principals) => setAgents(principals.agents),
      () => setAgents([]),
    );
  }, [api]);

  const savePool = (prefix: string, pool: AgentPool) =>
    setEntries((current) => {
      const state = current[prefix]?.state;
      if (!state) return current;
      return {
        ...current,
        [prefix]: {
          state: {
            ...state,
            pools: [...state.pools.filter((item) => item.name !== pool.name), pool],
          },
        },
      };
    });

  const choose = (prefix: string) => {
    setSelected(prefix);
    setCreating(false);
    setShowDetail(true);
  };

  const queue = creating ? undefined : queues.find((item) => item.prefix === selected);
  const closeNew = () => {
    setCreating(false);
    setShowDetail(false);
  };

  return (
    <div className="flex min-h-0 flex-1">
      <datalist id={agentList}>
        {agents.map((agent) => (
          <option key={agent} value={agent} />
        ))}
      </datalist>
      <QueueList
        queues={queues}
        entries={entries}
        current={queue?.prefix}
        creating={creating}
        hidden={showDetail}
        onChoose={choose}
        onNew={() => {
          setCreating(true);
          setShowDetail(true);
        }}
      />
      <main className={cn("flex min-w-0 flex-1 flex-col", !showDetail && "max-sm:hidden")}>
        {creating && (
          <NewQueuePane
            agentList={agentList}
            onClose={closeNew}
            onCreate={async (input) => {
              await onCreate(input);
              choose(input.prefix);
            }}
          />
        )}
        {!creating && !queue && (
          <p className={cn(EMPTY, "p-5")}>No queues yet. Create one with New queue.</p>
        )}
        {queue && (
          <QueueDetail
            {...detailOf(queue.prefix, entries[queue.prefix], poolsMissing, secretsMissing)}
            queue={queue}
            tab={shownTab(tab, entries[queue.prefix]?.state?.binding)}
            onTab={setTab}
            agentList={agentList}
            agents={agents}
            onBack={() => setShowDetail(false)}
            onUpdate={onUpdate}
            onReload={() => void load(queue.prefix)}
            onPoolSaved={(pool) => savePool(queue.prefix, pool)}
            onPoolsMissing={(names) => setPoolsMissing(names && { queue: queue.prefix, names })}
            onSecretsMissing={(missing) =>
              setSecretsMissing(missing && { queue: queue.prefix, ...missing })
            }
          />
        )}
      </main>
    </div>
  );
}

/**
 * What the selected queue's tabs show from the dialog's state: its load, the
 * pools it needs, and the pools and secrets a refused bind named for it.
 */
function detailOf(
  prefix: string,
  entry: QueueEntry | undefined,
  poolsMissing: { queue: string; names: string[] } | null,
  secretsMissing: { queue: string; names: string[]; message: string } | null,
) {
  const state = entry?.state;
  const missing = poolsMissing?.queue === prefix ? poolsMissing.names : [];
  return {
    entry,
    required: requiredWithMissing(state?.definition, missing),
    poolsEmpty: queueWarns(entry),
    secretsMissing: secretsMissing?.queue === prefix ? secretsMissing : undefined,
  };
}

/** The queues with their workflow, and New queue. */
function QueueList({
  queues,
  entries,
  current,
  creating,
  hidden,
  onChoose,
  onNew,
}: {
  queues: TaskQueue[];
  entries: Record<string, QueueEntry>;
  current?: string;
  creating: boolean;
  /** On a phone the list hides while a queue shows. */
  hidden: boolean;
  onChoose: (prefix: string) => void;
  onNew: () => void;
}) {
  return (
    <nav
      aria-label="Queue list"
      className={cn(
        "flex w-67.5 shrink-0 flex-col border-r bg-muted/30 max-sm:w-full max-sm:border-r-0",
        hidden && "max-sm:hidden",
      )}
    >
      <ul className="flex min-h-0 flex-1 flex-col gap-0.5 overflow-auto p-2">
        {queues.map((item) => (
          <QueueListItem
            key={item.prefix}
            queue={item}
            entry={entries[item.prefix]}
            current={current === item.prefix}
            onChoose={() => onChoose(item.prefix)}
          />
        ))}
      </ul>
      <div className="border-t p-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="w-full border-dashed"
          aria-pressed={creating}
          onClick={onNew}
        >
          <Plus />
          New queue
        </Button>
      </div>
    </nav>
  );
}

function QueueListItem({
  queue,
  entry,
  current,
  onChoose,
}: {
  queue: TaskQueue;
  entry: QueueEntry | undefined;
  current: boolean;
  onChoose: () => void;
}) {
  const summary = queueSummary(entry);
  const warn = queueWarns(entry);
  return (
    <li>
      <button
        type="button"
        aria-current={current ? "true" : undefined}
        aria-label={`Queue ${queue.prefix}: ${queue.name}, ${summary}${warn ? ", a pool has no agents" : ""}`}
        className={cn(
          "flex w-full flex-col gap-0.5 rounded-panel px-2.5 py-2 text-left hover:bg-muted",
          current &&
            "bg-[color-mix(in_oklab,var(--primary)_10%,transparent)] hover:bg-[color-mix(in_oklab,var(--primary)_14%,transparent)]",
        )}
        onClick={onChoose}
      >
        <span className="flex min-w-0 items-baseline gap-2">
          <strong className={cn(MONO, "text-ui font-medium")}>{queue.prefix}</strong>
          <span className="min-w-0 truncate text-body-sm">{queue.name}</span>
          {warn && (
            <span
              role="img"
              aria-label="A pool has no agents"
              title="A pool the workflow uses has no agents"
              className="ml-auto size-2 shrink-0 self-center rounded-full bg-status-failed"
            />
          )}
        </span>
        <span
          className={cn(
            "truncate text-caption text-muted-foreground",
            summary.includes("@") && MONO,
          )}
        >
          {summary}
        </span>
      </button>
    </li>
  );
}
