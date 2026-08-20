import { useEffect, useState } from "react";
import { toast } from "sonner";
import { useAgentName, useAgentStatus } from "@/lib/agent";
import { usePolling } from "@/hooks/usePolling";
import {
  getAgent, subscribeAgentEvents,
  setAgentModel, setAgentEffort, agentPost, ApiError, getActiveDaemon,
} from "@/lib/api";
import { useDaemons } from "@/components/DaemonProvider";
import { Card, CardContent } from "@/components/ui/card";
import { NotesEditor } from "@/components/NotesEditor";
import { StatusChatHistory } from "@/components/StatusChatHistory";
import { ComboField, EFFORT_PRESETS } from "@/components/ComboField";
import { LoopToggle, ConfirmButton } from "@/components/LoopControls";
import { TuiScreen } from "@/components/TuiScreen";
import { useTerminalSocket } from "@/hooks/useTerminalSocket";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { IterationAuditLog } from "@/components/IterationAuditLog";
import { InboxComposer } from "@/components/InboxComposer";
import { IterationTimeoutControl } from "@/components/IterationTimeoutControl";
import { MODEL_PRESETS_BY_HARNESS } from "@/lib/runtimePresets";

const err = (e: unknown) => toast.error(e instanceof ApiError ? e.message : String(e));

// AgentOverview is the agent landing tab — a 1:1 replica of the old "Agent" tab:
// a live control surface (alias, status+history, inline model/effort, loop/
// restart/kill, notes) whose body branches on `interactive`:
//   interactive     → the live tmux terminal (TuiScreen)
//   non-interactive → the live audit log + channels (send messages)
export default function AgentOverview() {
  const name = useAgentName();
  // Consume the daemon context so a HostSwitcher change re-runs the SSE effect.
  const { activeId } = useDaemons();
  const { data: view, refresh: refreshView } = usePolling(() => getAgent(name), 5000);
  // The status snapshot is polled once in AgentLayout and shared via context, so
  // Overview no longer opens its own /status poll (avoids the double 2s poll).
  const { status, refresh } = useAgentStatus();
  // Do not probe the tmux-only screen endpoint until the agent view confirms
  // interactive mode. Non-interactive agents correctly reject that endpoint.
  const interactive = view?.interactive ?? false;
  const tui = useTerminalSocket(name, interactive, getActiveDaemon());
  const [execPrompt, setExecPrompt] = useState("");

  // SSE: refetch status on any live event (the stream is a hint, not truth).
  // Keyed on activeId so a host switch re-targets the stream.
  useEffect(() => {
    if (!name) return;
    return subscribeAgentEvents(name, ["iteration", "audit"], () => void refresh());
  }, [name, refresh, activeId]);

  return (
    <div className="flex h-full flex-col gap-4">
      {/* Header — pinned to the top: cwd/status, controls, exec. The alias +
          loop-status line now lives in AgentLayout, above the tabs. */}
      <div className="shrink-0 space-y-4">
      <Card className="py-0">
        <CardContent className="flex items-center gap-4 py-2">
          <span className="min-w-0 flex-1 truncate font-mono text-xs">cwd: {view?.cwd || "…"}</span>
          <div className="flex min-w-0 flex-1 items-center justify-end gap-2 text-sm">
            <span className="min-w-0 flex-1 truncate text-right text-muted-foreground">
              {status?.status_message
                ? `${status.status_updated ? `${status.status_updated} — ` : ""}${status.status_message}`
                : "no message"}
            </span>
            <StatusChatHistory name={name} />
          </div>
        </CardContent>
      </Card>

      <div className="flex flex-wrap items-end gap-2">
        <LoopToggle name={name} enabled={status?.loop_enabled ?? false} onChanged={refresh} />
        <ConfirmButton
          label="Restart"
          description="The loop session is recreated with the current model and prompt. The harness context is lost."
          onConfirm={async () => { await agentPost(name, "restart"); toast.success("restart done"); refresh(); }}
        />
        <IterationTimeoutControl name={name} status={status} refresh={refresh} />
        <ConfirmButton
          label="Kill"
          variant="destructive"
          description="The current iteration is killed via its shim. The running harness context is lost."
          onConfirm={async () => { await agentPost(name, "kill"); toast.success("killed"); refresh(); }}
        />
        <Input
          value={execPrompt}
          onChange={(e) => setExecPrompt(e.target.value)}
          placeholder="one-shot exec prompt (optional)"
          className="h-8 min-w-0 flex-1"
        />
        <Button
          size="sm"
          onClick={() =>
            agentPost(name, "exec", execPrompt ? { prompt: execPrompt } : undefined)
              .then(() => { toast.success("exec started"); setExecPrompt(""); refresh(); }, err)
          }
        >
          Exec
        </Button>
        <ComboField
          key={`model-${view?.model ?? ""}`}
          label="model"
          value={view?.model ?? ""}
          presets={[...MODEL_PRESETS_BY_HARNESS.claude]}
          onCommit={(v) => setAgentModel(name, v).then(() => { toast.success(`model = ${v}`); refreshView(); }, err)}
        />
        <ComboField
          key={`effort-${view?.effort ?? ""}`}
          label="effort"
          value={view?.effort ?? ""}
          presets={EFFORT_PRESETS}
          onCommit={(v) => setAgentEffort(name, v).then(() => { toast.success(`effort = ${v}`); refreshView(); }, err)}
        />
      </div>
      </div>

      {/* Body — fills the remaining height: live terminal or audit log. */}
      <div
        className="min-h-0 flex-1"
        data-testid={interactive ? undefined : "agent-noninteractive-view"}
      >
        {interactive ? (
          <TuiScreen
            fill
            controller={tui}
            onStart={() => agentPost(name, "restart").then(() => {
              toast.success("started");
              // Re-dial now that the agent is starting. reconnect() clears the
              // old absent state and opens a fresh bounded startup grace, so
              // the engine and shim have time to publish the new session.
              tui.reconnect();
            }, err)}
          />
        ) : (
          <IterationAuditLog
            name={name}
            iterationId={status?.last_iteration_id ?? ""}
            iterationStatus={status?.last_iteration ?? ""}
          />
        )}
      </div>

      {/* Footer — pinned to the bottom: inbox composer (non-interactive) + notes. */}
      <div className="shrink-0 space-y-4">
        {!interactive && <InboxComposer name={name} />}
        <NotesEditor />
      </div>
    </div>
  );
}
