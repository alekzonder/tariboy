import { useState, type RefObject } from "react";
import { Minus, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import {
  DEFAULT_WORKSPACE_ID,
  addHostToWorkspace,
  createWorkspace,
  hostsIn,
  inWorkspace,
  removeHostFromWorkspace,
  type WorkspacesState,
} from "@/lib/workspaces";
import type { WorkspaceHost } from "./HostWorkspaces";

/** The manager's left column: every workspace, and the form that adds one. */
export function WorkspaceList({
  state,
  hosts,
  selectedId,
  newNameRef,
  onPick,
}: {
  state: WorkspacesState;
  hosts: WorkspaceHost[];
  selectedId: string;
  newNameRef: RefObject<HTMLInputElement | null>;
  onPick: (id: string) => void;
}) {
  const [newName, setNewName] = useState("");
  const create = () => {
    const id = createWorkspace(newName);
    if (!id) return;
    setNewName("");
    onPick(id);
  };
  return (
    <div className="flex w-55 shrink-0 flex-col border-r max-sm:max-h-40 max-sm:w-auto max-sm:border-r-0 max-sm:border-b">
      <div className="grid min-h-0 flex-1 content-start gap-px overflow-auto px-1.5 py-2">
        {state.workspaces.map((workspace) => (
          <button
            key={workspace.id}
            type="button"
            aria-current={workspace.id === selectedId ? "true" : undefined}
            onClick={() => onPick(workspace.id)}
            className={cn(
              "flex h-8 w-full items-center gap-2 rounded-md px-2.5 text-left text-body hover:bg-accent",
              workspace.id === selectedId && "bg-accent font-medium",
            )}
          >
            <span className="min-w-0 flex-1 truncate">{workspace.name}</span>
            <span className="text-meta text-muted-foreground tabular-nums">
              {hostsIn(state, hosts, workspace.id).length}
            </span>
          </button>
        ))}
      </div>
      <div className="grid shrink-0 gap-1.5 border-t p-2.5">
        <input
          ref={newNameRef}
          value={newName}
          onChange={(event) => setNewName(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") create();
          }}
          placeholder="New workspace name"
          aria-label="New workspace name"
          className="h-7 min-w-0 rounded-md bg-muted px-2.25 text-body-sm outline-none placeholder:text-muted-foreground"
        />
        <Button size="sm" variant="secondary" onClick={create}>
          Create workspace
        </Button>
      </div>
    </div>
  );
}

/** One host of the selected workspace, with its "Add to…" picker and its
 *  removal from this workspace only. */
export function WorkspaceHostRow({
  host,
  state,
  selectedId,
  adding,
  onToggleAdd,
  onAdded,
}: {
  host: WorkspaceHost;
  state: WorkspacesState;
  selectedId: string;
  adding: boolean;
  onToggleAdd: () => void;
  onAdded: () => void;
}) {
  const targets = state.workspaces.filter((w) => !inWorkspace(state, host.id, w.id));
  const selected = state.workspaces.find((w) => w.id === selectedId);
  // A host in Default and no other workspace would only fall back to Default.
  const removable =
    selectedId !== DEFAULT_WORKSPACE_ID ||
    state.workspaces.some((w) => w.id !== DEFAULT_WORKSPACE_ID && w.hosts.includes(host.id));
  return (
    <div className="grid gap-0.5 rounded-lg border px-2.5 py-2">
      <div className="flex h-6.5 items-center gap-2">
        <span
          aria-hidden="true"
          className={cn(
            "size-1.5 shrink-0 rounded-full",
            host.ready ? "bg-status-running" : "bg-status-stopped",
          )}
        />
        <span className="min-w-0 flex-1 truncate font-mono text-ui font-medium">{host.label}</span>
        <span className="text-ui text-muted-foreground">{host.ready ? "ready" : "offline"}</span>
        {targets.length > 0 && (
          <button
            type="button"
            aria-label={`Add ${host.label} to…`}
            aria-expanded={adding}
            title="Add to another workspace"
            onClick={onToggleAdd}
            className="inline-flex h-6 shrink-0 items-center gap-1 rounded-control border px-2 text-ui hover:bg-accent"
          >
            <Plus aria-hidden="true" className="size-3" />
            Add to…
          </button>
        )}
        {removable && (
          <button
            type="button"
            aria-label={`Remove ${host.label} from ${selected?.name ?? ""}`}
            title="Remove from this workspace"
            onClick={() => removeHostFromWorkspace(host.id, selectedId)}
            className="inline-flex h-6 shrink-0 items-center gap-1 rounded-control border px-2 text-ui hover:bg-accent"
          >
            <Minus aria-hidden="true" className="size-3" />
            Remove
          </button>
        )}
      </div>
      {adding && (
        <div className="flex flex-wrap items-center gap-1 rounded-md bg-muted px-1.75 py-1.5">
          {targets.map((target) => (
            <button
              key={target.id}
              type="button"
              onClick={() => {
                // List it here first: a host only Default holds would leave
                // Default once another workspace lists it.
                addHostToWorkspace(host.id, selectedId);
                addHostToWorkspace(host.id, target.id);
                onAdded();
              }}
              className="h-6 rounded-sm border bg-card px-2.25 text-ui hover:border-ring"
            >
              {target.name}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

/** The manager's foot: delete the selected workspace, after a confirmation. */
export function WorkspaceDeleteBar({
  name,
  confirming,
  onConfirming,
  onDelete,
}: {
  name: string;
  confirming: boolean;
  onConfirming: (confirming: boolean) => void;
  onDelete: () => void;
}) {
  return (
    <div className="flex shrink-0 items-center gap-2 border-t px-4.5 py-2.5">
      {confirming ? (
        <>
          <span className="min-w-0 flex-1 text-body-sm">
            Delete {name}? Hosts in no other workspace return to Default.
          </span>
          <Button size="sm" variant="secondary" onClick={() => onConfirming(false)}>
            Cancel
          </Button>
          <Button size="sm" variant="destructive" onClick={onDelete}>
            Delete
          </Button>
        </>
      ) : (
        <Button
          size="sm"
          variant="secondary"
          className="text-destructive"
          onClick={() => onConfirming(true)}
        >
          Delete workspace
        </Button>
      )}
    </div>
  );
}
