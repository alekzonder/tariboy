import { useEffect, useRef } from "react";
import { toast } from "sonner";
import { FirstVisitHint } from "@/components/onboarding/FirstVisitHint";
import { SendFilesButton } from "@/components/SendFilesButton";
import { TuiScreen } from "@/components/TuiScreen";
import { useFileDropTarget } from "@/hooks/useFileDropTarget";
import { useSendFiles } from "@/hooks/useSendFiles";
import { useTerminalSocket } from "@/hooks/useTerminalSocket";
import { ApiError, type AgentRow } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { useAgentStatus } from "@/lib/agent";
import { deriveAgentState } from "@/lib/agentState";
import { hostToParam } from "@/lib/terminalsHost";
import { TerminalBanner, type BannerState } from "./TerminalBanner";

// The banner follows the derived agent state; "needs you" does not change
// what the screen shows, so it is left out here.
function bannerState({
  agent,
  status,
  interactive,
  alive,
  starting,
}: {
  agent: AgentRow;
  status: Parameters<typeof deriveAgentState>[0]["status"];
  interactive: boolean;
  alive: boolean;
  starting: boolean;
}): BannerState {
  const state = deriveAgentState({ agent, status, needs: { count: 0 } });
  if (state.kind === "error") return state;
  if (!interactive) return { kind: "no-terminal" };
  if (!alive) return { kind: "off" };
  if (starting) return { kind: "starting" };
  if (state.kind === "working") return { kind: "working", taskKey: agent.current_goal_task_key };
  return { kind: "idle" };
}

export default function AgentConsoleTab({
  hostId,
  agent,
  refresh,
  execCount = 0,
}: {
  hostId: string;
  agent: AgentRow;
  refresh: () => void;
  /** Bumped by the header after each successful Exec. */
  execCount?: number;
}) {
  const api = useApi();
  const interactive = agent.interactive !== false;
  const alive = agent.enabled ?? agent.state !== "stopped";
  const controller = useTerminalSocket(agent.name, interactive && alive);
  // The workspace polls the status more often than the agent list refreshes.
  const { status } = useAgentStatus();
  const absentUpload = useSendFiles({
    onUploaded: (paths) => toast.success(`uploaded: ${paths.join(", ")}`),
  });
  const absentDrop = useFileDropTarget(absentUpload.sendFiles);

  const act = async (fn: () => Promise<unknown>) => {
    try {
      await fn();
      refresh();
    } catch (error) {
      toast.error(error instanceof ApiError ? error.message : String(error));
    }
  };

  const start = () =>
    act(async () => {
      await api.agents.startAgent(agent.name);
      controller.reconnect();
    });
  // Exec lives in the header; an interactive terminal reconnects after it.
  // Only a change counts: remounting the tab must not redial.
  const { reconnect } = controller;
  const seenExec = useRef(execCount);
  useEffect(() => {
    if (seenExec.current === execCount) return;
    seenExec.current = execCount;
    if (interactive) reconnect();
  }, [execCount, interactive, reconnect]);
  const settings = `/agents/${hostToParam(hostId)}/${encodeURIComponent(agent.name)}/settings`;
  const banner = bannerState({
    agent,
    status,
    interactive,
    alive,
    starting: controller.status === "connecting" || controller.absent,
  });

  return (
    <div className="flex h-full flex-col gap-2">
      <TerminalBanner
        state={banner}
        harness={agent.image.split(":")[0] === "bare" ? "" : agent.harness}
        cwd={agent.cwd}
        settings={settings}
        onRetry={start}
        onStart={start}
      />
      {interactive && alive && <FirstVisitHint hostId={hostId} agent={agent.name} tab="console" />}
      {!interactive ? (
        <div
          data-testid="agent-console-absent-drop-target"
          data-file-drag-active={absentDrop.dragActive}
          onDragOver={absentDrop.onDragOver}
          onDragLeave={absentDrop.onDragLeave}
          onDrop={absentDrop.onDrop}
          className={`flex flex-1 flex-col items-center justify-center gap-2 rounded-md border text-sm text-muted-foreground${absentDrop.dragActive ? " ring-2 ring-primary" : ""}`}
        >
          <SendFilesButton onUploaded={(paths) => toast.success(`uploaded: ${paths.join(", ")}`)} />
        </div>
      ) : !alive ? (
        // The banner says the agent is off; the box only keeps the layout.
        <div className="flex-1 rounded-md border" />
      ) : (
        <TuiScreen controller={controller} fill onStart={start} />
      )}
    </div>
  );
}
