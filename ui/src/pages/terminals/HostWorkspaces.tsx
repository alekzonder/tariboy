import { useRef, useState } from "react";
import { Check, ChevronDown, Layers, Plus, Settings } from "lucide-react";
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
import {
  DEFAULT_WORKSPACE_ID,
  deleteWorkspace,
  hostsIn,
  renameWorkspace,
  selectWorkspace,
  useWorkspaces,
} from "@/lib/workspaces";
import { WorkspaceDeleteBar, WorkspaceHostRow, WorkspaceList } from "./WorkspaceManagerParts";

export interface WorkspaceHost {
  id: string;
  label: string;
  ready: boolean;
}

const hostCount = (n: number) => `${n} host${n === 1 ? "" : "s"}`;

/** Titlebar control: the current workspace and a menu to switch or manage. */
export function WorkspaceSwitcher({
  hosts,
  onSelect,
  onManage,
}: {
  hosts: WorkspaceHost[];
  onSelect: (id: string) => void;
  /** Opens the manager on the current workspace; `focusNew` for "New workspace…". */
  onManage: (focusNew: boolean) => void;
}) {
  const state = useWorkspaces();
  const current = state.workspaces.find((w) => w.id === state.active) ?? state.workspaces[0];
  const itemClass = "h-7.5 gap-2 rounded-control px-2.25 text-body";
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
          className="flex h-6.5 min-w-0 shrink-0 items-center gap-1.5 rounded-control px-2 text-body hover:bg-accent"
        >
          {/* A phone's titlebar has room for the icon only. */}
          <Layers aria-hidden="true" className="size-4 sm:hidden" />
          <span className="max-w-40 truncate font-medium max-sm:hidden">{current.name}</span>
          <ChevronDown aria-hidden="true" className="size-2.5 shrink-0 opacity-45" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        sideOffset={5}
        className="w-65 rounded-card p-1.25 shadow-[var(--lift)] ring-0"
        onCloseAutoFocus={(event) => {
          if (managing.current) event.preventDefault();
          managing.current = false;
        }}
      >
        <DropdownMenuLabel className="px-2.25 pt-1.5 pb-1 text-caption font-semibold tracking-[.06em] uppercase">
          Workspaces
        </DropdownMenuLabel>
        {state.workspaces.map((workspace) => (
          <DropdownMenuItem
            key={workspace.id}
            role="menuitemradio"
            aria-checked={workspace.id === state.active}
            className={itemClass}
            onSelect={() => {
              selectWorkspace(workspace.id);
              onSelect(workspace.id);
            }}
          >
            <span className="min-w-0 flex-1 truncate font-medium">{workspace.name}</span>
            <span className="text-meta text-muted-foreground tabular-nums">
              {hostCount(hostsIn(state, hosts, workspace.id).length)}
            </span>
            {workspace.id === state.active ? (
              <Check aria-hidden="true" className="size-3 text-primary" />
            ) : (
              <span aria-hidden="true" className="w-3" />
            )}
          </DropdownMenuItem>
        ))}
        <DropdownMenuSeparator className="mx-0.75 my-1" />
        <DropdownMenuItem className={itemClass} onSelect={() => manage(true)}>
          <Plus aria-hidden="true" className="size-3 text-muted-foreground" />
          New workspace…
        </DropdownMenuItem>
        <DropdownMenuItem className={itemClass} onSelect={() => manage(false)}>
          <Settings aria-hidden="true" className="size-3.25 text-muted-foreground" />
          Manage workspaces…
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** Create, rename and delete workspaces, and move hosts between them. The
 *  caller remounts it (via `open`) so each opening starts on `initialId`. */
export function WorkspaceManager({
  hosts,
  open,
  initialId,
  focusNew,
  onOpenChange,
}: {
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

function ManagerBody({
  hosts,
  initialId,
  focusNew,
}: {
  hosts: WorkspaceHost[];
  initialId: string;
  focusNew: boolean;
}) {
  const state = useWorkspaces();
  const [selectedId, setSelectedId] = useState(initialId);
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

  return (
    <DialogContent
      className="flex h-[min(500px,calc(100vh-32px))] w-[min(720px,calc(100vw-32px))] max-w-none flex-col gap-0 overflow-hidden rounded-xl p-0 shadow-[var(--lift)] ring-0 sm:max-w-none"
      onOpenAutoFocus={(event) => {
        if (!focusNew) return;
        event.preventDefault();
        newNameRef.current?.focus();
      }}
    >
      <div className="flex shrink-0 items-center gap-2 border-b py-3 pr-12 pl-4.5">
        <DialogTitle className="text-body-lg font-semibold">Manage workspaces</DialogTitle>
        <DialogDescription className="text-ui text-muted-foreground">
          New hosts join Default.
        </DialogDescription>
      </div>
      <div className="flex min-h-0 flex-1 max-sm:flex-col">
        <WorkspaceList
          state={state}
          hosts={hosts}
          selectedId={selected.id}
          newNameRef={newNameRef}
          onPick={pick}
        />
        <div className="flex min-w-0 flex-1 flex-col">
          <div className="grid min-h-0 flex-1 content-start gap-4 overflow-auto px-4.5 py-4">
            <div className="grid gap-1.5">
              <span className="text-meta font-medium text-muted-foreground">Name</span>
              {isDefault ? (
                <div className="flex h-7.5 items-center gap-2">
                  <span className="text-body font-medium">Default</span>
                  <span className="inline-flex h-4.5 items-center rounded-chip bg-primary/12 px-1.5 text-tiny font-medium text-primary">
                    built-in
                  </span>
                  <span className="text-ui text-muted-foreground">
                    Can't be renamed or deleted.
                  </span>
                </div>
              ) : (
                <NameInput key={selected.id} id={selected.id} name={selected.name} />
              )}
            </div>
            <div className="grid gap-1">
              <span className="text-meta font-medium text-muted-foreground">Hosts</span>
              {members.length === 0 && (
                <div className="rounded-lg border border-dashed p-3.5 text-body-sm text-muted-foreground">
                  No hosts yet. Open another workspace and move a host here.
                </div>
              )}
              {members.map((host) => (
                <WorkspaceHostRow
                  key={host.id}
                  host={host}
                  state={state}
                  selectedId={selected.id}
                  moving={moving === host.id}
                  onToggleMove={() =>
                    setMoving((current) => (current === host.id ? null : host.id))
                  }
                  onMoved={() => setMoving(null)}
                />
              ))}
            </div>
          </div>
          {!isDefault && (
            <WorkspaceDeleteBar
              name={selected.name}
              confirming={confirming}
              onConfirming={setConfirming}
              onDelete={() => {
                deleteWorkspace(selected.id);
                pick(DEFAULT_WORKSPACE_ID);
              }}
            />
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
      className="h-7.5 max-w-80 rounded-md bg-card text-body"
    />
  );
}
