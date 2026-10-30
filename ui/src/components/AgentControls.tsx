import { useState } from "react";
import { Link } from "react-router-dom";
import { Copy, Code2, MoreHorizontal, Pin, Play, Settings2, Square, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { ApiError } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import type { PrimaryAction } from "@/pages/agents/agentWorkspaceModel";
import { ForceStopDialog } from "./ForceStopDialog";

function useAction(refresh?: () => void) {
  return async (label: string, fn: () => Promise<unknown>) => {
    try {
      await fn();
      toast.success(`${label} ok`);
      refresh?.();
    } catch (e) {
      const m = e instanceof ApiError ? e.message : String(e);
      toast.error(`${label} failed: ${m}`);
    }
  };
}

// RunStateActions is the lifecycle trio on its own. The Configuration page's
// run-state strip mounts THIS rather than AgentControls, so it shares these
// handlers instead of hand-rolling a second start/stop control — and without
// carrying Kill, Delete and the overflow menu onto that page.
export function RunStateActions({ name, refresh }: { name: string; refresh?: () => void }) {
  const api = useApi();
  const run = useAction(refresh);

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button size="sm" onClick={() => run("start", () => api.agentPost(name, "start"))}>
        Start
      </Button>
      <Button
        size="sm"
        variant="secondary"
        onClick={() => run("stop", () => api.agentPost(name, "stop"))}
      >
        Stop
      </Button>
      <Button
        size="sm"
        variant="secondary"
        onClick={() => run("restart", () => api.agentPost(name, "restart"))}
      >
        Restart
      </Button>
    </div>
  );
}

function ExecDialog({
  open,
  pending,
  disabled,
  prompt,
  setPrompt,
  setOpen,
  onExec,
}: {
  open: boolean;
  pending: boolean;
  disabled: boolean;
  prompt: string;
  setPrompt: (prompt: string) => void;
  setOpen: (open: boolean) => void;
  onExec: () => void;
}) {
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!pending) setOpen(next);
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Run once</DialogTitle>
          <DialogDescription>
            Starts one run now; the optional text applies only to it.
          </DialogDescription>
        </DialogHeader>
        <Textarea
          value={prompt}
          onChange={(event) => setPrompt(event.target.value)}
          placeholder="one-shot prompt (optional)"
          disabled={pending}
        />
        <DialogFooter>
          <Button variant="outline" disabled={pending} onClick={() => setOpen(false)}>
            Cancel
          </Button>
          <Button disabled={pending || disabled} onClick={onExec}>
            Run once
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function DeleteAgentDialog({
  name,
  open,
  pending,
  setOpen,
  onDelete,
}: {
  name: string;
  open: boolean;
  pending: boolean;
  setOpen: (open: boolean) => void;
  onDelete: () => void;
}) {
  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (!pending) setOpen(next);
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete agent {name}?</AlertDialogTitle>
          <AlertDialogDescription>
            This permanently deletes the agent and all of its durable data. This action cannot be
            undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>Cancel</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={pending}
            onClick={(event) => {
              event.preventDefault();
              onDelete();
            }}
          >
            {pending ? "Deleting…" : "Delete agent"}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

const MENU_ITEM = "h-7 rounded-control px-2.25 text-body";

const PRIMARY_LABEL: Record<PrimaryAction, string> = {
  answer: "Answer",
  give: "Give task",
  start: "Start",
  restart: "Restart",
};

type AgentControlsProps = {
  name: string;
  /** The agent is up — the toggle offers Stop; otherwise it offers Start. */
  alive: boolean;
  /** `bare:latest` has no iteration loop, so it gets no Run once…. */
  image?: string;
  disabled?: boolean;
  /** The one primary button the derived state asks for. */
  primary: PrimaryAction;
  /** Where Answer and Give task go: the Work tab, with the task to open. */
  workPath: string;
  /** Route of this agent's Settings tab, for the menu's Settings item. */
  settingsPath: string;
  refresh?: () => void;
  /** Called after a Run once request succeeds, e.g. to reconnect a terminal. */
  onExec?: () => void;
  /** Called after the agent is deleted, so the caller can leave the route. */
  onDeleted?: () => void;
  onClone?: () => void;
  pinned?: boolean;
  onTogglePin?: () => void;
  /** Present only where the folder can open in VS Code. */
  onOpenFolder?: () => void;
};

function OverflowMenu({
  props,
  canRunOnce,
  onRunOnce,
  onForceStop,
  onDelete,
}: {
  props: AgentControlsProps;
  canRunOnce: boolean;
  onRunOnce: () => void;
  onForceStop: () => void;
  onDelete: () => void;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="More agent actions"
          className="size-7 rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"
        >
          <MoreHorizontal className="size-3.5" />
        </Button>
      </DropdownMenuTrigger>
      {/* The handoff's menu: 12px radius, --lift, no ring — one raised
          surface, same family as the content island. */}
      <DropdownMenuContent
        align="end"
        className="min-w-41 rounded-card p-1.25 shadow-[var(--lift)] ring-0"
      >
        {canRunOnce && (
          <DropdownMenuItem className={MENU_ITEM} disabled={props.disabled} onSelect={onRunOnce}>
            <Play aria-hidden="true" />
            Run once…
          </DropdownMenuItem>
        )}
        <DropdownMenuItem className={MENU_ITEM} disabled={props.disabled} onSelect={onForceStop}>
          <Square aria-hidden="true" />
          Force-stop current run
        </DropdownMenuItem>
        {props.onOpenFolder && (
          <DropdownMenuItem
            data-testid="open-agent-cwd-vscode"
            className={MENU_ITEM}
            disabled={props.disabled}
            onSelect={props.onOpenFolder}
          >
            <Code2 aria-hidden="true" />
            Open folder in VS Code
          </DropdownMenuItem>
        )}
        {props.onClone && (
          <DropdownMenuItem
            className={MENU_ITEM}
            disabled={props.disabled}
            onSelect={props.onClone}
          >
            <Copy aria-hidden="true" />
            Clone
          </DropdownMenuItem>
        )}
        {props.onTogglePin && (
          <DropdownMenuItem className={MENU_ITEM} onSelect={props.onTogglePin}>
            <Pin aria-hidden="true" />
            {props.pinned ? "Unpin" : "Pin"}
          </DropdownMenuItem>
        )}
        <DropdownMenuItem asChild className={MENU_ITEM}>
          <Link to={props.settingsPath}>
            <Settings2 aria-hidden="true" />
            Settings
          </Link>
        </DropdownMenuItem>
        <DropdownMenuSeparator className="mx-0.75 my-1" />
        <DropdownMenuItem
          className={`${MENU_ITEM} text-destructive focus:bg-destructive/10 focus:text-destructive`}
          onSelect={onDelete}
        >
          <Trash2 aria-hidden="true" />
          Delete agent
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/**
 * The action cluster on the right of the agent header: the one primary action
 * of the derived state, the on/off toggle, and a `⋯` menu for everything that
 * is not a per-second decision (Run once…, Force-stop, VS Code, Clone, Pin,
 * Settings, Delete). It rides in the header rather than in a tab, so the
 * lifecycle of the agent is reachable from every tab.
 *
 * Every call goes through the client of the nearest ApiProvider: the workspace
 * is addressed by route, and must never fall back to the active daemon.
 */
export function AgentControls(props: AgentControlsProps) {
  const { name, alive, disabled = false, primary, refresh, onExec, onDeleted } = props;
  const api = useApi();
  const run = useAction(refresh);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deletePending, setDeletePending] = useState(false);
  const [forceStopOpen, setForceStopOpen] = useState(false);
  const [execOpen, setExecOpen] = useState(false);
  const [execPending, setExecPending] = useState(false);
  const [prompt, setPrompt] = useState("");

  const exec = async () => {
    if (execPending) return;
    setExecPending(true);
    try {
      await api.agentPost(name, "exec", prompt ? { prompt } : undefined);
      setPrompt("");
      setExecOpen(false);
      refresh?.();
      onExec?.();
      toast.success("run started");
    } catch (error) {
      toast.error(error instanceof ApiError ? error.message : String(error));
    } finally {
      setExecPending(false);
    }
  };

  const remove = async () => {
    if (deletePending) return;
    setDeletePending(true);
    try {
      await api.agents.agentDelete(name, { force: true, purge: true });
      refresh?.();
      onDeleted?.();
    } catch (error) {
      toast.error(error instanceof ApiError ? error.message : String(error));
    } finally {
      setDeletePending(false);
    }
  };

  const post = (action: "start" | "stop" | "restart") => () =>
    void run(action, () => api.agentPost(name, action));

  return (
    <div data-testid="agent-controls" className="flex shrink-0 items-center gap-1.5">
      {primary === "answer" || primary === "give" ? (
        <Button asChild size="sm">
          <Link to={props.workPath} aria-disabled={disabled || undefined}>
            {PRIMARY_LABEL[primary]}
          </Link>
        </Button>
      ) : (
        <Button size="sm" disabled={disabled} onClick={post(primary)}>
          {PRIMARY_LABEL[primary]}
        </Button>
      )}
      {primary !== "start" &&
        (alive ? (
          <Button size="sm" variant="secondary" disabled={disabled} onClick={post("stop")}>
            Stop
          </Button>
        ) : (
          <Button size="sm" variant="secondary" disabled={disabled} onClick={post("start")}>
            Start
          </Button>
        ))}
      <OverflowMenu
        props={props}
        canRunOnce={alive && props.image !== "bare:latest"}
        onRunOnce={() => setExecOpen(true)}
        onForceStop={() => setForceStopOpen(true)}
        onDelete={() => setDeleteOpen(true)}
      />

      <ExecDialog
        open={execOpen}
        pending={execPending}
        disabled={disabled}
        prompt={prompt}
        setPrompt={setPrompt}
        setOpen={setExecOpen}
        onExec={() => void exec()}
      />
      <ForceStopDialog
        name={name}
        open={forceStopOpen}
        setOpen={setForceStopOpen}
        onForceStop={() => void run("force-stop", () => api.agentPost(name, "kill"))}
      />
      <DeleteAgentDialog
        name={name}
        open={deleteOpen}
        pending={deletePending}
        setOpen={setDeleteOpen}
        onDelete={() => void remove()}
      />
    </div>
  );
}
