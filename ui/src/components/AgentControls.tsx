import { useState } from "react";
import { Link } from "react-router-dom";
import { MoreVertical, Settings2, Trash2 } from "lucide-react";
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

/**
 * The action cluster on the right of the agent header: the run-state toggle,
 * Kill, and an overflow menu for everything that is not a per-second decision.
 * It rides in the header rather than in a tab, so the lifecycle of the agent is
 * reachable from every tab — which is why the Terminal tab no longer carries a
 * second copy of these buttons.
 *
 * Exec (a one-shot iteration with an optional prompt) sits in front of the
 * toggle and only while the agent is enabled.
 *
 * Every call goes through the client of the nearest ApiProvider: the workspace
 * is addressed by route, and must never fall back to the active daemon.
 */
export function AgentControls({
  name,
  alive,
  image,
  disabled = false,
  configurationPath,
  refresh,
  onExec,
  onDeleted,
}: {
  name: string;
  /** The agent is up — the toggle offers Stop; otherwise it offers Start. */
  alive: boolean;
  /** `bare:latest` has no iteration loop, so it gets no Exec. */
  image?: string;
  disabled?: boolean;
  /** Route of this agent's Configuration tab, for the menu's Settings item. */
  configurationPath: string;
  refresh?: () => void;
  /** Called after an Exec request succeeds, e.g. to reconnect a terminal. */
  onExec?: () => void;
  /** Called after the agent is deleted, so the caller can leave the route. */
  onDeleted?: () => void;
}) {
  const api = useApi();
  const run = useAction(refresh);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deletePending, setDeletePending] = useState(false);
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
      toast.success("exec started");
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

  return (
    <div data-testid="agent-controls" className="flex shrink-0 items-center gap-1.5">
      {alive && image !== "bare:latest" && (
        <Button size="sm" variant="secondary" disabled={disabled} onClick={() => setExecOpen(true)}>
          Exec
        </Button>
      )}
      {alive ? (
        <Button
          size="sm"
          variant="secondary"
          disabled={disabled}
          onClick={() => run("stop", () => api.agentPost(name, "stop"))}
        >
          Stop
        </Button>
      ) : (
        <Button
          size="sm"
          disabled={disabled}
          onClick={() => run("start", () => api.agentPost(name, "start"))}
        >
          Start
        </Button>
      )}
      <Button
        size="sm"
        variant="secondary"
        disabled={disabled}
        className="text-destructive"
        onClick={() => {
          if (window.confirm(`Kill the session for ${name}?`)) {
            void run("kill", () => api.agentPost(name, "kill"));
          }
        }}
      >
        Kill
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="Agent settings"
            className="size-7 rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"
          >
            <MoreVertical className="size-3.5" />
          </Button>
        </DropdownMenuTrigger>
        {/* The handoff's menu: 12px radius, --lift, no ring — one raised
            surface, same family as the content island. */}
        <DropdownMenuContent
          align="end"
          className="min-w-41 rounded-card p-1.25 shadow-[var(--lift)] ring-0"
        >
          <DropdownMenuItem asChild className="h-7 rounded-control px-2.25 text-body">
            <Link to={configurationPath}>
              <Settings2 aria-hidden="true" />
              Settings
            </Link>
          </DropdownMenuItem>
          <DropdownMenuSeparator className="mx-0.75 my-1" />
          <DropdownMenuItem
            className="h-7 rounded-control px-2.25 text-body text-destructive focus:bg-destructive/10 focus:text-destructive"
            onSelect={() => setDeleteOpen(true)}
          >
            <Trash2 aria-hidden="true" />
            Delete agent
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <Dialog
        open={execOpen}
        onOpenChange={(open) => {
          if (!execPending) setExecOpen(open);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Exec one-shot iteration</DialogTitle>
            <DialogDescription>
              Runs one iteration now; the optional text applies only to it.
            </DialogDescription>
          </DialogHeader>
          <Textarea
            value={prompt}
            onChange={(event) => setPrompt(event.target.value)}
            placeholder="one-shot prompt (optional)"
            disabled={execPending}
          />
          <DialogFooter>
            <Button variant="outline" disabled={execPending} onClick={() => setExecOpen(false)}>
              Cancel
            </Button>
            <Button disabled={execPending || disabled} onClick={() => void exec()}>
              Exec
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={deleteOpen}
        onOpenChange={(open) => {
          if (!deletePending) setDeleteOpen(open);
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
            <AlertDialogCancel disabled={deletePending}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={deletePending}
              onClick={(event) => {
                event.preventDefault();
                void remove();
              }}
            >
              {deletePending ? "Deleting…" : "Delete agent"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
