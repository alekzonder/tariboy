import { useState } from "react";
import { ChevronLeft } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import type { AgentPool, CreateQueueInput, Queue, UpdateQueueInput } from "@/lib/api";
import { cn } from "@/lib/utils";
import { EMPTY, LABEL, MONO } from "./panelStyles";
import QueuePools from "./QueuePools";
import QueueEnvironment from "./QueueEnvironment";
import QueueSources from "./QueueSources";
import QueueWorkflowSettings from "./QueueWorkflowSettings";
import {
  generalInput,
  newQueueInput,
  queueTabs,
  type QueueEntry,
  type QueueState,
  type QueueTab,
} from "./queueSettingsModel";

const TAB_LABELS: Record<QueueTab, string> = {
  general: "General",
  workflow: "Workflow",
  pools: "Pools",
  environment: "Environment",
  sources: "Sources",
};

/** What the selected queue's tabs read and report back to the dialog. */
export interface QueueDetailProps {
  queue: Queue;
  entry: QueueEntry | undefined;
  tab: QueueTab;
  onTab: (tab: QueueTab) => void;
  agentList: string;
  agents: string[];
  /** The pools the workflow needs, and those a refused bind named. */
  required: Map<string, string[]>;
  /** Whether a pool the workflow needs has no agents. */
  poolsEmpty: boolean;
  /** The secrets a refused bind named for this queue. */
  secretsMissing?: { names: string[]; message: string };
  onBack: () => void;
  onUpdate: (prefix: string, input: UpdateQueueInput) => Promise<void>;
  onReload: () => void;
  onPoolSaved: (pool: AgentPool) => void;
  onPoolsMissing: (names: string[] | null) => void;
  onSecretsMissing: (missing: { names: string[]; message: string } | null) => void;
}

/** The selected queue: its header, a load error with Retry, and its tabs. */
export function QueueDetail(props: QueueDetailProps) {
  const { queue, entry, onReload } = props;
  return (
    <>
      <DetailHeader onBack={props.onBack}>
        <strong className={cn(MONO, "text-body font-medium")}>{queue.prefix}</strong>
        <h2 className="min-w-0 truncate text-title font-semibold">{queue.name}</h2>
      </DetailHeader>
      {entry?.error && (
        <div
          role="alert"
          aria-label={`Queue ${queue.prefix} error`}
          className="mx-5 mt-3 flex items-center gap-2 text-ui text-destructive"
        >
          <span>{entry.error}</span>
          <Button type="button" size="sm" variant="secondary" className="h-6.5" onClick={onReload}>
            Retry
          </Button>
        </div>
      )}
      <QueueTabs {...props} />
    </>
  );
}

/** The Environment tab: keyed by the binding, so a new one starts it fresh. */
function EnvironmentTab({
  queue,
  state,
  secretsMissing,
}: {
  queue: string;
  state: QueueState | undefined;
  secretsMissing: QueueDetailProps["secretsMissing"];
}) {
  return (
    <QueueEnvironment
      key={`${queue}:${state?.binding?.digest ?? ""}`}
      queue={queue}
      definition={state?.definition}
      missing={secretsMissing?.names}
      missingMessage={secretsMissing?.message}
    />
  );
}

function QueueTabs({
  queue,
  entry,
  tab,
  onTab,
  agentList,
  agents,
  required,
  poolsEmpty,
  secretsMissing,
  onUpdate,
  onReload,
  onPoolSaved,
  onPoolsMissing,
  onSecretsMissing,
}: QueueDetailProps) {
  const state = entry?.state;
  return (
    <Tabs
      value={tab}
      onValueChange={(value) => onTab(value as QueueTab)}
      className="min-h-0 flex-1 gap-0"
    >
      <TabsList className="mx-5 mt-2 h-auto w-auto justify-start gap-0.5 overflow-x-auto rounded-none border-b bg-transparent p-0">
        {queueTabs(state?.binding).map((value) => (
          <TabsTrigger
            key={value}
            value={value}
            className="rounded-none px-3 py-2 text-body-sm data-[state=active]:font-semibold data-[state=active]:shadow-[inset_0_-2px_0_var(--primary)]"
          >
            {TAB_LABELS[value]}
            {value === "pools" && poolsEmpty && (
              <span aria-label="needs agents" className="size-1.5 rounded-full bg-status-failed" />
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
            onOpenTab={onTab}
            onChanged={onReload}
            onPoolsMissing={onPoolsMissing}
            onSecretsMissing={onSecretsMissing}
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
              onSaved={onPoolSaved}
              onReload={onReload}
            />
          ) : (
            !entry?.error && <span className={EMPTY}>Loading agent pools…</span>
          )}
        </TabsContent>
        <TabsContent value="environment">
          <EnvironmentTab queue={queue.prefix} state={state} secretsMissing={secretsMissing} />
        </TabsContent>
        {state?.binding && (
          <TabsContent value="sources">
            <QueueSources key={`${queue.prefix}:${state.binding.digest}`} queue={queue.prefix} />
          </TabsContent>
        )}
      </div>
    </Tabs>
  );
}

/** The New queue form with its header. */
export function NewQueuePane({
  agentList,
  onCreate,
  onClose,
}: {
  agentList: string;
  onCreate: (input: CreateQueueInput) => Promise<void>;
  onClose: () => void;
}) {
  return (
    <>
      <DetailHeader onBack={onClose}>
        <span className="text-title font-semibold">New queue</span>
      </DetailHeader>
      <div className="min-h-0 flex-1 overflow-auto px-5 py-4">
        <NewQueueForm agentList={agentList} onCancel={onClose} onCreate={onCreate} />
      </div>
    </>
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
  queue: Queue;
  agentList: string;
  onUpdate: (prefix: string, input: UpdateQueueInput) => Promise<void>;
}) {
  return (
    <form
      className="flex max-w-xl flex-col gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        void onUpdate(
          queue.prefix,
          generalInput(new FormData(event.currentTarget), queue.revision),
        );
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
        void onCreate(newQueueInput(prefix, name, new FormData(event.currentTarget)));
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
