import { useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { SendFilesButton } from "@/components/SendFilesButton";
import { TuiScreen } from "@/components/TuiScreen";
import { useFileDropTarget } from "@/hooks/useFileDropTarget";
import { useSendFiles } from "@/hooks/useSendFiles";
import { useTerminalSocket } from "@/hooks/useTerminalSocket";
import { agentPostOn, ApiError } from "@/lib/api";
import { hostToParam, targetFor } from "@/lib/terminalsHost";
import type { AgentSummary } from "@/lib/types";

export default function AgentConsoleTab({ hostId, agent, refresh }: {
  hostId: string;
  agent: AgentSummary;
  refresh: () => void;
}) {
  const target = targetFor(hostId);
  const interactive = agent.interactive !== false;
  const alive = agent.enabled ?? agent.state !== "stopped";
  const controller = useTerminalSocket(agent.name, interactive && alive, target);
  const [prompt, setPrompt] = useState("");
  const [execPending, setExecPending] = useState(false);
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
  const exec = async () => {
    if (execPending) return;
    setExecPending(true);
    try {
      await agentPostOn(target, agent.name, "exec", prompt ? { prompt } : undefined);
      setPrompt("");
      refresh();
      if (interactive) controller.reconnect();
      toast.success("exec started");
    } catch (error) {
      toast.error(error instanceof ApiError ? error.message : String(error));
    } finally {
      setExecPending(false);
    }
  };
  const configuration = `/agents/${hostToParam(hostId)}/${encodeURIComponent(agent.name)}/configuration`;

  return (
    <div className="flex h-full flex-col gap-2">
      {/* Start/Stop, Kill and Delete live in the agent header now — they are
          lifecycle, not console, and the header is on every tab. What stays
          here is the one-shot exec prompt, which is console work. */}
      {agent.image !== "bare:latest" && (
        <div className="flex shrink-0 items-center justify-end gap-2">
          <Input
            value={prompt}
            onChange={(event) => setPrompt(event.target.value)}
            placeholder="one-shot exec prompt (optional)"
            className="h-8 min-w-48 max-w-xl flex-1"
            disabled={execPending}
          />
          <Button size="sm" disabled={execPending} onClick={() => void exec()}>Exec</Button>
        </div>
      )}
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
