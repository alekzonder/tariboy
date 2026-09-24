import { useEffect, useRef } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { SendFilesButton } from "@/components/SendFilesButton";
import { TuiScreen } from "@/components/TuiScreen";
import { useFileDropTarget } from "@/hooks/useFileDropTarget";
import { useSendFiles } from "@/hooks/useSendFiles";
import { useTerminalSocket } from "@/hooks/useTerminalSocket";
import { agentPostOn, ApiError } from "@/lib/api";
import { hostToParam, targetFor } from "@/lib/terminalsHost";
import type { AgentSummary } from "@/lib/types";

export default function AgentConsoleTab({ hostId, agent, refresh, execCount = 0 }: {
  hostId: string;
  agent: AgentSummary;
  refresh: () => void;
  /** Bumped by the header after each successful Exec. */
  execCount?: number;
}) {
  const target = targetFor(hostId);
  const interactive = agent.interactive !== false;
  const alive = agent.enabled ?? agent.state !== "stopped";
  const controller = useTerminalSocket(agent.name, interactive && alive, target);
  const absentUpload = useSendFiles({
    daemon: target,
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

  const start = () => act(async () => {
    await agentPostOn(target, agent.name, "start");
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
  const configuration = `/agents/${hostToParam(hostId)}/${encodeURIComponent(agent.name)}/configuration`;

  return (
    <div className="flex h-full flex-col gap-2">
      {!interactive ? (
        <div
          data-testid="agent-console-absent-drop-target"
          data-file-drag-active={absentDrop.dragActive}
          onDragOver={absentDrop.onDragOver}
          onDragLeave={absentDrop.onDragLeave}
          onDrop={absentDrop.onDrop}
          className={`flex flex-1 flex-col items-center justify-center gap-2 rounded-md border text-sm text-muted-foreground${absentDrop.dragActive ? " ring-2 ring-primary" : ""}`}
        >
          <p>This agent has no interactive terminal.</p>
          <Link className="text-primary underline" to={configuration}>Open Configuration</Link>
          <SendFilesButton
            daemon={target}
            onUploaded={(paths) => toast.success(`uploaded: ${paths.join(", ")}`)}
          />
        </div>
      ) : !alive ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 rounded-md border text-sm text-muted-foreground">
          <p>Agent is stopped.</p>
          <Button size="sm" onClick={() => void start()}>Start</Button>
        </div>
      ) : (
        <TuiScreen controller={controller} fill daemon={target} onStart={start} />
      )}
    </div>
  );
}
