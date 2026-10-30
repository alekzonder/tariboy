import { useId, useState } from "react";
import { X } from "lucide-react";
import { toast } from "sonner";
import { useApi } from "@/components/apiContext";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ApiError, type AgentPool } from "@/lib/api";
import { cn } from "@/lib/utils";
import { EMPTY, FIELD, MONO } from "./panelStyles";
import { errorText } from "@/lib/errors";
import { idempotencyKey } from "@/lib/ids";

/**
 * A queue's agent pools, one row each: the pools the bound workflow requires,
 * even before they exist, then the queue's other pools and the rows added here.
 * A row saves its whole agent list with the pool's revision.
 */
export default function QueuePools({
  queue,
  pools,
  required,
  agents,
  onSaved,
  onReload,
}: {
  queue: string;
  pools: AgentPool[];
  /** Pool name → the workflow statuses that use it; none for a pool a refused bind named. */
  required: Map<string, string[]>;
  /** The server's agents, offered as suggestions. */
  agents: string[];
  onSaved: (pool: AgentPool) => void;
  /** Reads the pools again after a revision conflict. */
  onReload: () => void;
}) {
  const [added, setAdded] = useState<string[]>([]);
  const [adding, setAdding] = useState(false);
  const [newName, setNewName] = useState("");
  const [notice, setNotice] = useState("");
  const listId = useId();
  const names = [...new Set([...required.keys(), ...pools.map((pool) => pool.name), ...added])];

  const addRow = (event: React.FormEvent) => {
    event.preventDefault();
    const name = newName.trim();
    if (name && !names.includes(name)) setAdded((current) => [...current, name]);
    setNewName("");
    setAdding(false);
  };

  return (
    <section className="flex min-w-0 flex-col gap-2.5" aria-label={`Agent pools ${queue}`}>
      <datalist id={listId}>
        {agents.map((agent) => (
          <option key={agent} value={agent} />
        ))}
      </datalist>
      {notice && (
        <p role="status" className="text-ui text-muted-foreground">
          {notice}
        </p>
      )}
      {names.length === 0 && (
        <p className={EMPTY}>No pools. A workflow names the pools its statuses use.</p>
      )}
      {names.map((name) => {
        const pool = pools.find((item) => item.name === name);
        return (
          <PoolRow
            key={`${name}:${pool?.revision ?? 0}`}
            queue={queue}
            name={name}
            pool={pool}
            statuses={required.get(name)}
            listId={listId}
            onSaved={(saved) => {
              setNotice("");
              onSaved(saved);
            }}
            onDeleted={() => {
              setNotice("");
              setAdded((current) => current.filter((item) => item !== name));
              onReload();
            }}
            onConflict={() => {
              setNotice(
                "Configuration changed elsewhere. Current revisions were reloaded; review and retry.",
              );
              onReload();
            }}
          />
        );
      })}
      {adding ? (
        <form className="flex items-center gap-1.5" onSubmit={addRow}>
          <Input
            aria-label="New pool name"
            autoFocus
            className={cn(FIELD, MONO, "h-7 w-48 dark:bg-muted")}
            value={newName}
            onChange={(event) => setNewName(event.target.value)}
            placeholder="pool name"
          />
          <Button
            type="submit"
            size="sm"
            className="h-7"
            aria-label="Create pool row"
            disabled={!newName.trim()}
          >
            Add
          </Button>
          <Button
            type="button"
            size="sm"
            variant="secondary"
            className="h-7"
            onClick={() => setAdding(false)}
          >
            Cancel
          </Button>
        </form>
      ) : (
        <div>
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="h-7 border-dashed"
            onClick={() => setAdding(true)}
          >
            Add pool
          </Button>
        </div>
      )}
    </section>
  );
}

function PoolRow({
  queue,
  name,
  pool,
  statuses,
  listId,
  onSaved,
  onDeleted,
  onConflict,
}: {
  queue: string;
  name: string;
  pool?: AgentPool;
  statuses?: string[];
  listId: string;
  onSaved: (pool: AgentPool) => void;
  onDeleted: () => void;
  onConflict: () => void;
}) {
  const api = useApi();
  const saved = pool?.agents ?? [];
  const [draft, setDraft] = useState(saved);
  const [entry, setEntry] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const changed = draft.join("\n") !== saved.join("\n");
  const empty = saved.length === 0;

  const add = () => {
    const agent = entry.trim();
    if (agent && !draft.includes(agent)) setDraft((current) => [...current, agent]);
    setEntry("");
  };

  const save = async () => {
    setBusy(true);
    setError("");
    try {
      const updated = await api.tasks.rebindAgentPool(
        queue,
        name,
        draft,
        pool?.revision ?? 0,
        idempotencyKey("pool"),
      );
      onSaved(updated);
      toast.success(`Pool ${name} saved`);
    } catch (err) {
      if (err instanceof ApiError && err.code === "revision_conflict") onConflict();
      else setError(errorText(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div
      role="group"
      aria-label={`Pool ${name}`}
      className={cn(
        "grid gap-2 rounded-lg border p-3 sm:grid-cols-[180px_minmax(0,1fr)_auto] sm:items-start",
        statuses &&
          empty &&
          "border-[color-mix(in_oklab,var(--status-failed)_35%,transparent)] bg-[color-mix(in_oklab,var(--status-failed)_5%,transparent)]",
      )}
    >
      <PoolLabel name={name} statuses={statuses} empty={empty} />
      <div className="flex min-w-0 flex-wrap items-center gap-1.5">
        {draft.map((agent) => (
          <span
            key={agent}
            className={cn(
              MONO,
              "inline-flex items-center gap-0.5 rounded-sm bg-muted py-0.5 pr-0.5 pl-2",
            )}
          >
            {agent}
            <button
              type="button"
              aria-label={`Remove ${agent} from ${name}`}
              className="grid size-5 place-items-center rounded text-muted-foreground hover:bg-accent hover:text-foreground"
              onClick={() => setDraft((current) => current.filter((item) => item !== agent))}
            >
              <X className="size-3" />
            </button>
          </span>
        ))}
        <span className="inline-flex items-center gap-1">
          <Input
            aria-label={`Add agent to ${name}`}
            list={listId}
            className={cn(FIELD, "h-6.5 w-40 dark:bg-muted")}
            value={entry}
            placeholder="Add agent…"
            onChange={(event) => setEntry(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                add();
              }
            }}
          />
          <Button
            type="button"
            size="sm"
            variant="secondary"
            className="h-6.5"
            disabled={!entry.trim()}
            onClick={add}
          >
            Add
          </Button>
        </span>
      </div>
      <div className="flex gap-1.5 justify-self-start">
        <Button
          type="button"
          size="sm"
          className="h-7"
          aria-label={`Save pool ${name}`}
          disabled={busy || !changed}
          onClick={() => void save()}
        >
          Save
        </Button>
        {pool && !statuses && (
          <DeletePool
            queue={queue}
            pool={pool}
            onDeleted={onDeleted}
            onConflict={onConflict}
            onError={setError}
          />
        )}
      </div>
      {error && (
        <p role="alert" className="text-ui text-destructive sm:col-span-3">
          {error}
        </p>
      )}
    </div>
  );
}

/** Deletes an existing pool the workflow does not use, after a confirmation. */
function DeletePool({
  queue,
  pool,
  onDeleted,
  onConflict,
  onError,
}: {
  queue: string;
  pool: AgentPool;
  onDeleted: () => void;
  onConflict: () => void;
  onError: (message: string) => void;
}) {
  const api = useApi();
  const [busy, setBusy] = useState(false);

  const remove = async () => {
    setBusy(true);
    onError("");
    try {
      await api.tasks.deleteAgentPool(queue, pool.name, pool.revision, idempotencyKey("pool-del"));
      toast.success(`Pool ${pool.name} deleted`);
      onDeleted();
    } catch (err) {
      if (err instanceof ApiError && err.code === "revision_conflict") onConflict();
      else onError(errorText(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="h-7"
          aria-label={`Delete pool ${pool.name}`}
          disabled={busy}
        >
          Delete
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete pool {pool.name}?</AlertDialogTitle>
          <AlertDialogDescription>
            Removes the pool and its agent list from {queue}. The agents stay.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction onClick={() => void remove()}>Delete</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

/** A pool's name, the workflow statuses that use it, and a no-agents mark. */
function PoolLabel({
  name,
  statuses,
  empty,
}: {
  name: string;
  statuses?: string[];
  empty: boolean;
}) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5">
      <strong className={cn(MONO, "text-body-sm font-medium")}>{name}</strong>
      <span className="text-caption text-muted-foreground">
        {!statuses
          ? "not used by the workflow"
          : statuses.length
            ? statuses.join(", ")
            : "needed to bind the workflow"}
      </span>
      {empty && (
        <span
          className={cn("text-caption", statuses ? "text-status-failed" : "text-muted-foreground")}
        >
          no agents
        </span>
      )}
    </div>
  );
}
