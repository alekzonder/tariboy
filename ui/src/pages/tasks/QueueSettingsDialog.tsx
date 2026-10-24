import { useCallback, useEffect, useId, useRef, useState } from "react";
import { ChevronLeft, Plus } from "lucide-react";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import type {
  AgentPool,
  ApiClient,
  CreateQueueInput,
  QueueWorkflow,
  TaskQueue,
  UpdateQueueInput,
  WorkflowDefinition,
} from "@/lib/api";
import { cn } from "@/lib/utils";
import { EMPTY, LABEL, MONO } from "./panelStyles";
import QueuePools from "./QueuePools";
import QueueEnvironment from "./QueueEnvironment";
import QueueSources from "./QueueSources";
import QueueWorkflowSettings from "./QueueWorkflowSettings";
import { emptyPools, requiredPools } from "./workflowPools";
import { errorText } from "@/lib/errors";

interface QueueState {
  binding: QueueWorkflow | null;
  definition: WorkflowDefinition | null;
  pools: AgentPool[];
}
type Entry = { state: QueueState; error?: undefined } | { state?: undefined; error: string };
type Tab = "general" | "workflow" | "pools" | "environment" | "sources";
const TAB_LABELS: Record<Tab, string> = {
  general: "General",
  workflow: "Workflow",
  pools: "Pools",
  environment: "Environment",
  sources: "Sources",
};

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

const splitList = (value: FormDataEntryValue | null) =>
  String(value ?? "")
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);

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
  const [tab, setTab] = useState<Tab>("general");
  const [entries, setEntries] = useState<Record<string, Entry>>({});
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
  const state = queue ? entries[queue.prefix]?.state : undefined;
  const loadError = queue ? entries[queue.prefix]?.error : undefined;
  const empty = state?.definition ? emptyPools(state.definition, state.pools) : [];
  const required = new Map(state?.definition ? requiredPools(state.definition) : []);
  if (poolsMissing?.queue === queue?.prefix)
    poolsMissing?.names.forEach((name) => {
      if (!required.has(name)) required.set(name, []);
    });
  const activeTab: Tab = tab === "sources" && !state?.binding ? "general" : tab;

  return (
    <div className="flex min-h-0 flex-1">
      <datalist id={agentList}>
        {agents.map((agent) => (
          <option key={agent} value={agent} />
        ))}
      </datalist>
      <nav
        aria-label="Queue list"
        className={cn(
          "flex w-67.5 shrink-0 flex-col border-r bg-muted/30 max-sm:w-full max-sm:border-r-0",
          showDetail && "max-sm:hidden",
        )}
      >
        <ul className="flex min-h-0 flex-1 flex-col gap-0.5 overflow-auto p-2">
          {queues.map((item) => {
            const entry = entries[item.prefix];
            const summary = !entry
              ? "…"
              : !entry.state
                ? "unknown"
                : entry.state.binding
                  ? `${entry.state.binding.name}@${entry.state.binding.version}`
                  : "flexible";
            const warn = entry?.state?.definition
              ? emptyPools(entry.state.definition, entry.state.pools).length > 0
              : false;
            return (
              <li key={item.prefix}>
                <button
                  type="button"
                  aria-current={queue?.prefix === item.prefix ? "true" : undefined}
                  aria-label={`Queue ${item.prefix}: ${item.name}, ${summary}${warn ? ", a pool has no agents" : ""}`}
                  className={cn(
                    "flex w-full flex-col gap-0.5 rounded-panel px-2.5 py-2 text-left hover:bg-muted",
                    queue?.prefix === item.prefix &&
                      "bg-[color-mix(in_oklab,var(--primary)_10%,transparent)] hover:bg-[color-mix(in_oklab,var(--primary)_14%,transparent)]",
                  )}
                  onClick={() => choose(item.prefix)}
                >
                  <span className="flex min-w-0 items-baseline gap-2">
                    <strong className={cn(MONO, "text-ui font-medium")}>{item.prefix}</strong>
                    <span className="min-w-0 truncate text-body-sm">{item.name}</span>
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
          })}
        </ul>
        <div className="border-t p-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="w-full border-dashed"
            aria-pressed={creating}
            onClick={() => {
              setCreating(true);
              setShowDetail(true);
            }}
          >
            <Plus />
            New queue
          </Button>
        </div>
      </nav>
      <main className={cn("flex min-w-0 flex-1 flex-col", !showDetail && "max-sm:hidden")}>
        {creating ? (
          <>
            <DetailHeader
              onBack={() => {
                setCreating(false);
                setShowDetail(false);
              }}
            >
              <span className="text-title font-semibold">New queue</span>
            </DetailHeader>
            <div className="min-h-0 flex-1 overflow-auto px-5 py-4">
              <NewQueueForm
                agentList={agentList}
                onCancel={() => {
                  setCreating(false);
                  setShowDetail(false);
                }}
                onCreate={async (input) => {
                  await onCreate(input);
                  choose(input.prefix);
                }}
              />
            </div>
          </>
        ) : !queue ? (
          <p className={cn(EMPTY, "p-5")}>No queues yet. Create one with New queue.</p>
        ) : (
          <>
            <DetailHeader onBack={() => setShowDetail(false)}>
              <strong className={cn(MONO, "text-body font-medium")}>{queue.prefix}</strong>
              <h2 className="min-w-0 truncate text-title font-semibold">{queue.name}</h2>
            </DetailHeader>
            {loadError && (
              <div
                role="alert"
                aria-label={`Queue ${queue.prefix} error`}
                className="mx-5 mt-3 flex items-center gap-2 text-ui text-destructive"
              >
                <span>{loadError}</span>
                <Button
                  type="button"
                  size="sm"
                  variant="secondary"
                  className="h-6.5"
                  onClick={() => void load(queue.prefix)}
                >
                  Retry
                </Button>
              </div>
            )}
            <Tabs
              value={activeTab}
              onValueChange={(value) => setTab(value as Tab)}
              className="min-h-0 flex-1 gap-0"
            >
              <TabsList className="mx-5 mt-2 h-auto w-auto justify-start gap-0.5 overflow-x-auto rounded-none border-b bg-transparent p-0">
                {(
                  [
                    "general",
                    "workflow",
                    "pools",
                    "environment",
                    ...(state?.binding ? ["sources"] : []),
                  ] as Tab[]
                ).map((value) => (
                  <TabsTrigger
                    key={value}
                    value={value}
                    className="rounded-none px-3 py-2 text-body-sm data-[state=active]:font-semibold data-[state=active]:shadow-[inset_0_-2px_0_var(--primary)]"
                  >
                    {TAB_LABELS[value]}
                    {value === "pools" && empty.length > 0 && (
                      <span
                        aria-label="needs agents"
                        className="size-1.5 rounded-full bg-status-failed"
                      />
                    )}
                  </TabsTrigger>
                ))}
              </TabsList>
              <div className="min-h-0 flex-1 overflow-auto px-5 py-4">
                <TabsContent value="general">
                  <GeneralForm
                    key={`${queue.prefix}:${queue.revision}`}
                    queue={queue}
                    agentList={agentList}
                    onUpdate={onUpdate}
                  />
                </TabsContent>
                {/* Mounted while hidden, so a refused bind and pool drafts survive a trip to another tab. */}
                <TabsContent value="workflow" forceMount className="data-[state=inactive]:hidden">
                  <QueueWorkflowSettings
                    key={queue.prefix}
                    queue={queue.prefix}
                    pools={state?.pools ?? []}
                    onOpenTab={setTab}
                    onChanged={() => void load(queue.prefix)}
                    onPoolsMissing={(names) =>
                      setPoolsMissing(names && { queue: queue.prefix, names })
                    }
                    onSecretsMissing={(missing) =>
                      setSecretsMissing(missing && { queue: queue.prefix, ...missing })
                    }
                  />
                </TabsContent>
                <TabsContent value="pools" forceMount className="data-[state=inactive]:hidden">
                  {state ? (
                    <QueuePools
                      key={queue.prefix}
                      queue={queue.prefix}
                      pools={state.pools}
                      required={required}
                      agents={agents}
                      onSaved={(pool) => savePool(queue.prefix, pool)}
                      onReload={() => void load(queue.prefix)}
                    />
                  ) : (
                    !loadError && <span className={EMPTY}>Loading agent pools…</span>
                  )}
                </TabsContent>
                <TabsContent value="environment">
                  <QueueEnvironment
                    key={`${queue.prefix}:${state?.binding?.digest ?? ""}`}
                    queue={queue.prefix}
                    missing={
                      secretsMissing?.queue === queue.prefix ? secretsMissing.names : undefined
                    }
                    missingMessage={
                      secretsMissing?.queue === queue.prefix ? secretsMissing.message : undefined
                    }
                  />
                </TabsContent>
                {state?.binding && (
                  <TabsContent value="sources">
                    <QueueSources
                      key={`${queue.prefix}:${state.binding.digest}`}
                      queue={queue.prefix}
                    />
                  </TabsContent>
                )}
              </div>
            </Tabs>
          </>
        )}
      </main>
    </div>
  );
}

function DetailHeader({ onBack, children }: { onBack: () => void; children: React.ReactNode }) {
  return (
    <div className="flex min-w-0 items-center gap-2 px-5 pt-4 max-sm:px-3">
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        className="sm:hidden"
        aria-label="Back to queues"
        onClick={onBack}
      >
        <ChevronLeft />
      </Button>
      {children}
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="flex flex-col gap-1.5">
      <span className={LABEL}>{label}</span>
      {children}
    </label>
  );
}

function GeneralForm({
  queue,
  agentList,
  onUpdate,
}: {
  queue: TaskQueue;
  agentList: string;
  onUpdate: (prefix: string, input: UpdateQueueInput) => Promise<void>;
}) {
  return (
    <form
      className="flex max-w-xl flex-col gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        const values = new FormData(event.currentTarget);
        void onUpdate(queue.prefix, {
          name: String(values.get("name") ?? "").trim(),
          description: String(values.get("description") ?? "").trim(),
          owners: splitList(values.get("owners")),
          responsible_agent: String(values.get("responsible") ?? "").trim(),
          revision: queue.revision,
        });
      }}
    >
      <Field label="Queue name">
        <Input name="name" aria-label={`Queue name ${queue.prefix}`} defaultValue={queue.name} />
      </Field>
      <Field label="Description">
        <Textarea
          name="description"
          aria-label={`Queue description ${queue.prefix}`}
          defaultValue={queue.description}
        />
      </Field>
      <Field label="Owner agents">
        <Input
          name="owners"
          aria-label={`Queue owners ${queue.prefix}`}
          defaultValue={queue.owners.join(", ")}
          placeholder="alice, agent:bob"
        />
      </Field>
      <Field label="Responsible agent">
        <Input
          name="responsible"
          list={agentList}
          aria-label={`Queue triager ${queue.prefix}`}
          defaultValue={queue.responsible_agent}
        />
      </Field>
      <div>
        <Button type="submit" size="sm" aria-label={`Save ${queue.prefix}`}>
          Save
        </Button>
      </div>
    </form>
  );
}

function NewQueueForm({
  agentList,
  onCreate,
  onCancel,
}: {
  agentList: string;
  onCreate: (input: CreateQueueInput) => Promise<void>;
  onCancel: () => void;
}) {
  const [prefix, setPrefix] = useState("");
  const [name, setName] = useState("");
  return (
    <form
      className="flex max-w-xl flex-col gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        const values = new FormData(event.currentTarget);
        void onCreate({
          prefix: prefix.trim(),
          name: name.trim(),
          owners: splitList(values.get("owners")),
          responsible_agent: String(values.get("responsible") ?? "").trim(),
        });
      }}
    >
      <Field label="Queue prefix">
        <Input
          aria-label="Queue prefix"
          className={MONO}
          value={prefix}
          placeholder="NEW-QUEUE"
          onChange={(event) => setPrefix(event.target.value.toUpperCase())}
        />
      </Field>
      <Field label="Queue name">
        <Input
          aria-label="New queue name"
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
      </Field>
      <Field label="Owner agents">
        <Input name="owners" placeholder="alice, agent:bob" />
      </Field>
      <Field label="Responsible agent">
        <Input name="responsible" list={agentList} />
      </Field>
      <div className="flex gap-2">
        <Button type="submit" size="sm" disabled={!prefix.trim() || !name.trim()}>
          Add queue
        </Button>
        <Button type="button" size="sm" variant="secondary" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </form>
  );
}
