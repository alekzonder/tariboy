import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { toast } from "sonner";
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
import { SendFilesButton } from "@/components/SendFilesButton";
import { TuiScreen } from "@/components/TuiScreen";
import { useFileDropTarget } from "@/hooks/useFileDropTarget";
import { useSendFiles } from "@/hooks/useSendFiles";
import { useTerminalSocket } from "@/hooks/useTerminalSocket";
import { agentDeleteOn, agentPostOn, ApiError } from "@/lib/api";
import { hostToParam, targetFor } from "@/lib/terminalsHost";
import type { AgentSummary } from "@/lib/types";

export default function AgentConsoleTab({ hostId, agent, refresh }: {
  hostId: string;
  agent: AgentSummary;
  refresh: () => void;
}) {
  const navigate = useNavigate();
  const target = targetFor(hostId);
  const interactive = agent.interactive !== false;
  const alive = agent.enabled ?? agent.state !== "stopped";
  const controller = useTerminalSocket(agent.name, interactive && alive, target);
  const [prompt, setPrompt] = useState("");
  const [execPending, setExecPending] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deletePending, setDeletePending] = useState(false);
  const absentUpload = useSendFiles({
    name: agent.name,
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
  const stop = () => act(() => agentPostOn(target, agent.name, "stop"));
  const kill = () => act(() => agentPostOn(target, agent.name, "kill"));
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
  const remove = async () => {
    if (deletePending) return;
    setDeletePending(true);
    try {
      await agentDeleteOn(target, agent.name, { force: true, purge: true });
      refresh();
      navigate("/");
    } catch (error) {
      toast.error(error instanceof ApiError ? error.message : String(error));
    } finally {
      setDeletePending(false);
    }
  };
  const configuration = `/agents/${hostToParam(hostId)}/${encodeURIComponent(agent.name)}/configuration`;

  return (
    <div className="flex h-full flex-col gap-2">
      <div className="flex shrink-0 flex-wrap items-center gap-2">
        {alive ? (
          <Button size="sm" variant="secondary" onClick={() => void stop()}>Stop</Button>
        ) : (
          <Button size="sm" onClick={() => void start()}>Start</Button>
        )}
        <Button size="sm" variant="outline" onClick={() => {
          if (window.confirm(`Kill the session for ${agent.name}?`)) void kill();
        }}>Kill session</Button>
        <AlertDialog
          open={deleteOpen}
          onOpenChange={(open) => {
            if (!deletePending) setDeleteOpen(open);
          }}
        >
          <AlertDialogTrigger asChild>
            <Button size="sm" variant="destructive">Delete</Button>
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Delete agent {agent.name}?</AlertDialogTitle>
              <AlertDialogDescription>
                This permanently deletes the agent and all of its durable data. This action cannot be undone.
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
        {agent.image !== "bare:latest" && (
          <div className="ml-auto flex min-w-0 flex-1 items-center justify-end gap-2">
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
      </div>
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
            name={agent.name}
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
