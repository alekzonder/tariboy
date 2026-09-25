import { useRef, useState } from "react";
import { ArrowLeftRight, Check, ChevronDown, Plus, Settings } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import {
  DEFAULT_WORKSPACE_ID,
  createWorkspace,
  deleteWorkspace,
  moveHost,
  renameWorkspace,
  selectWorkspace,
  useWorkspaces,
  workspaceOf,
  type WorkspacesState,
} from "@/lib/workspaces";

export interface WorkspaceHost {
  id: string;
  label: string;
  ready: boolean;
}

const hostCount = (n: number) => `${n} host${n === 1 ? "" : "s"}`;

function hostsIn(state: WorkspacesState, hosts: WorkspaceHost[], id: string) {
  return hosts.filter((host) => workspaceOf(state, host.id) === id);
}

/** Titlebar control: the current workspace and a menu to switch or manage. */
export function WorkspaceSwitcher({ hosts, onSelect, onManage }: {
  hosts: WorkspaceHost[];
  onSelect: (id: string) => void;
  /** Opens the manager on the current workspace; `focusNew` for "New workspace…". */
  onManage: (focusNew: boolean) => void;
}) {
  const state = useWorkspaces();
  const current = state.workspaces.find((w) => w.id === state.active) ?? state.workspaces[0];
  const itemClass = "h-[30px] gap-2 rounded-[7px] px-[9px] text-[13px]";
  // The manager dialog takes focus; handing it back to this trigger as the
  // menu closes would steal it.
  const managing = useRef(false);
  const manage = (focusNew: boolean) => {
    managing.current = true;
    onManage(focusNew);
  };
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label={`Workspace: ${current.name}`}
          className="flex h-[26px] min-w-0 shrink-0 items-center gap-1.5 rounded-[7px] px-2 text-[13px] hover:bg-accent"
        >
          <span className="max-w-40 truncate font-medium">{current.name}</span>
          <ChevronDown aria-hidden="true" className="size-2.5 shrink-0 opacity-45" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        sideOffset={5}
        className="w-[260px] rounded-[12px] p-[5px] shadow-[var(--lift)] ring-0"
        onCloseAutoFocus={(event) => {
          if (managing.current) event.preventDefault();
          managing.current = false;
        }}
      >
        <DropdownMenuLabel className="px-[9px] pt-1.5 pb-1 text-[11px] font-semibold tracking-[.06em] uppercase">
          Workspaces
        </DropdownMenuLabel>
        {state.workspaces.map((workspace) => (
          <DropdownMenuItem
            key={workspace.id}
            className={itemClass}
            onSelect={() => {
              selectWorkspace(workspace.id);
              onSelect(workspace.id);
            }}
          >
            <span className="min-w-0 flex-1 truncate font-medium">{workspace.name}</span>
            <span className="text-[11.5px] text-muted-foreground tabular-nums">
              {hostCount(hostsIn(state, hosts, workspace.id).length)}
            </span>
            {workspace.id === state.active
              ? <Check aria-hidden="true" className="size-3 text-primary" />
              : <span aria-hidden="true" className="w-3" />}
          </DropdownMenuItem>
        ))}
        <DropdownMenuSeparator className="mx-[3px] my-1" />
        <DropdownMenuItem className={itemClass} onSelect={() => manage(true)}>
          <Plus aria-hidden="true" className="size-3 text-muted-foreground" />
          New workspace…
        </DropdownMenuItem>
        <DropdownMenuItem className={itemClass} onSelect={() => manage(false)}>
          <Settings aria-hidden="true" className="size-[13px] text-muted-foreground" />
          Manage workspaces…
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** Create, rename and delete workspaces, and move hosts between them. The
 *  caller remounts it (via `open`) so each opening starts on `initialId`. */
export function WorkspaceManager({ hosts, open, initialId, focusNew, onOpenChange }: {
  hosts: WorkspaceHost[];
  open: boolean;
  initialId: string;
  focusNew: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open && <ManagerBody hosts={hosts} initialId={initialId} focusNew={focusNew} />}
    </Dialog>
  );
}

function ManagerBody({ hosts, initialId, focusNew }: {
  hosts: WorkspaceHost[];
  initialId: string;
  focusNew: boolean;
}) {
  const state = useWorkspaces();
  const [selectedId, setSelectedId] = useState(initialId);
  const [newName, setNewName] = useState("");
  const [moving, setMoving] = useState<string | null>(null);
  const [confirming, setConfirming] = useState(false);
  const newNameRef = useRef<HTMLInputElement | null>(null);
  const selected = state.workspaces.find((w) => w.id === selectedId) ?? state.workspaces[0];
  const isDefault = selected.id === DEFAULT_WORKSPACE_ID;
  const members = hostsIn(state, hosts, selected.id);

  const pick = (id: string) => {
    setSelectedId(id);
    setMoving(null);
    setConfirming(false);
  };
  const create = () => {
    const id = createWorkspace(newName);
    if (!id) return;
    setNewName("");
    pick(id);
  };

  return (
    <DialogContent
      className="flex h-[min(500px,calc(100vh-32px))] w-[min(720px,calc(100vw-32px))] max-w-none flex-col gap-0 overflow-hidden rounded-[14px] p-0 shadow-[var(--lift)] ring-0 sm:max-w-none"
      overlayClassName="bg-[oklch(24%_.03_255/.22)] supports-backdrop-filter:backdrop-blur-none"
      onOpenAutoFocus={(event) => {
        if (!focusNew) return;
        event.preventDefault();
        newNameRef.current?.focus();
      }}
    >
      <div className="flex shrink-0 items-center gap-2 border-b py-3 pr-12 pl-[18px]">
        <DialogTitle className="text-[14px] font-semibold">Manage workspaces</DialogTitle>
        <DialogDescription className="text-[12px] text-muted-foreground">
          New hosts join Default.
        </DialogDescription>
      </div>
      <div className="flex min-h-0 flex-1">
        <div className="flex w-[220px] shrink-0 flex-col border-r">
          <div className="grid min-h-0 flex-1 content-start gap-px overflow-auto px-1.5 py-2">
            {state.workspaces.map((workspace) => (
              <button
                key={workspace.id}
                type="button"
                aria-current={workspace.id === selected.id ? "true" : undefined}
                onClick={() => pick(workspace.id)}
                className={cn(
                  "flex h-8 w-full items-center gap-2 rounded-[8px] px-2.5 text-left text-[13px] hover:bg-accent",
                  workspace.id === selected.id && "bg-accent font-medium",
                )}
              >
                <span className="min-w-0 flex-1 truncate">{workspace.name}</span>
                <span className="text-[11.5px] text-muted-foreground tabular-nums">
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
              onKeyDown={(event) => { if (event.key === "Enter") create(); }}
              placeholder="New workspace name"
              aria-label="New workspace name"
              className="h-7 min-w-0 rounded-[8px] bg-muted px-[9px] text-[12.5px] outline-none placeholder:text-muted-foreground"
            />
            <Button size="sm" variant="secondary" onClick={create}>Create workspace</Button>
          </div>
        </div>
        <div className="flex min-w-0 flex-1 flex-col">
          <div className="grid min-h-0 flex-1 content-start gap-4 overflow-auto px-[18px] py-4">
            <div className="grid gap-1.5">
              <span className="text-[11.5px] font-medium text-muted-foreground">Name</span>
              {isDefault ? (
                <div className="flex h-[30px] items-center gap-2">
                  <span className="text-[13px] font-medium">Default</span>
                  <span className="inline-flex h-[18px] items-center rounded-[5px] bg-primary/12 px-1.5 text-[10.5px] font-medium text-primary">
                    built-in
                  </span>
                  <span className="text-[12px] text-muted-foreground">Can't be renamed or deleted.</span>
                </div>
              ) : (
                <NameInput key={selected.id} id={selected.id} name={selected.name} />
              )}
            </div>
            <div className="grid gap-1">
              <span className="text-[11.5px] font-medium text-muted-foreground">Hosts</span>
              {members.length === 0 && (
                <div className="rounded-[10px] border border-dashed p-3.5 text-[12.5px] text-muted-foreground">
                  No hosts yet. Open another workspace and move a host here.
                </div>
              )}
              {members.map((host) => (
                <div key={host.id} className="grid gap-0.5 rounded-[10px] border px-2.5 py-2">
                  <div className="flex h-[26px] items-center gap-2">
                    <span
                      aria-hidden="true"
                      className={cn(
                        "size-1.5 shrink-0 rounded-full",
                        host.ready ? "bg-status-running" : "bg-status-stopped",
                      )}
                    />
                    <span className="min-w-0 flex-1 truncate font-mono text-[12px] font-medium">{host.label}</span>
                    <span className="text-[12px] text-muted-foreground">{host.ready ? "ready" : "offline"}</span>
                    {state.workspaces.length > 1 && (
                      <button
                        type="button"
                        aria-label={`Move ${host.label} to…`}
                        aria-expanded={moving === host.id}
                        title="Move to workspace"
                        onClick={() => setMoving((current) => current === host.id ? null : host.id)}
                        className="inline-flex h-6 shrink-0 items-center gap-1 rounded-[7px] border px-2 text-[12px] hover:bg-accent"
                      >
                        <ArrowLeftRight aria-hidden="true" className="size-3" />
                        Move to…
                      </button>
                    )}
                  </div>
                  {moving === host.id && (
                    <div className="flex flex-wrap items-center gap-1 rounded-[8px] bg-muted px-[7px] py-1.5">
                      {state.workspaces.filter((w) => w.id !== selected.id).map((target) => (
                        <button
                          key={target.id}
                          type="button"
                          onClick={() => {
                            moveHost(host.id, target.id);
                            setMoving(null);
                          }}
                          className="h-6 rounded-[6px] border bg-card px-[9px] text-[12px] hover:border-ring"
                        >
                          {target.name}
                        </button>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
          {!isDefault && (
            <div className="flex shrink-0 items-center gap-2 border-t px-[18px] py-2.5">
              {confirming ? (
                <>
                  <span className="min-w-0 flex-1 text-[12.5px]">
                    Delete {selected.name}? Its hosts move to Default.
                  </span>
                  <Button size="sm" variant="secondary" onClick={() => setConfirming(false)}>Cancel</Button>
                  <Button
                    size="sm"
                    variant="destructive"
                    onClick={() => {
                      deleteWorkspace(selected.id);
                      pick(DEFAULT_WORKSPACE_ID);
                    }}
                  >
                    Delete
                  </Button>
                </>
              ) : (
                <Button
                  size="sm"
                  variant="secondary"
                  className="text-destructive"
                  onClick={() => setConfirming(true)}
                >
                  Delete workspace
                </Button>
              )}
            </div>
          )}
        </div>
      </div>
    </DialogContent>
  );
}

/** Saves each non-blank edit; a blank field reverts to the saved name on blur. */
function NameInput({ id, name }: { id: string; name: string }) {
  const [draft, setDraft] = useState(name);
  return (
    <Input
      aria-label="Workspace name"
      value={draft}
      onChange={(event) => {
        setDraft(event.target.value);
        renameWorkspace(id, event.target.value);
      }}
      onBlur={() => setDraft(name)}
      className="h-[30px] max-w-[320px] rounded-[8px] bg-card text-[13px]"
    />
  );
}
