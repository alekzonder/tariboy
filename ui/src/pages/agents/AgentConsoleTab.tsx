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
import { hostToParam } from "@/lib/terminalsHost";
import { TerminalBanner } from "./TerminalBanner";

export default function AgentConsoleTab({
  hostId,
  agent,
  refresh,
  execCount = 0,
  knownAbsent = false,
  onAbsent,
}: {
  hostId: string;
  agent: AgentRow;
  refresh: () => void;
  /** Bumped by the header after each successful Exec. */
  execCount?: number;
  knownAbsent?: boolean;
  onAbsent?: (absent: boolean) => void;
}) {
  const api = useApi();
  const interactive = agent.interactive !== false;
  const controller = useTerminalSocket(agent.name, interactive, undefined, knownAbsent);
  useEffect(() => onAbsent?.(controller.absent), [controller.absent, onAbsent]);
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

  return (
    <div className="flex h-full flex-col gap-2">
      {!interactive && <TerminalBanner settings={settings} />}
      {interactive && !controller.absent && (
        <FirstVisitHint hostId={hostId} agent={agent.name} tab="console" />
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
          <SendFilesButton onUploaded={(paths) => toast.success(`uploaded: ${paths.join(", ")}`)} />
        </div>
      ) : (
        <TuiScreen controller={controller} fill onStart={start} />
      )}
    </div>
  );
}
