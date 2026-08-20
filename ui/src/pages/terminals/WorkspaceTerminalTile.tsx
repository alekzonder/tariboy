import { useState } from "react";
import { Link } from "react-router-dom";
import { RefreshCw, X } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { TuiScreen } from "@/components/TuiScreen";
import { useTerminalSocket } from "@/hooks/useTerminalSocket";
import { agentPostOn, ApiError } from "@/lib/api";
import { hostToParam, targetFor } from "@/lib/terminalsHost";
import type { AgentSummary } from "@/lib/types";
import { cn } from "@/lib/utils";
import { terminalKey, type TerminalIdentity } from "./workspaceState";

export interface WorkspaceTerminalTileProps {
  identity: TerminalIdentity;
  hostLabel?: string;
  agent?: AgentSummary;
  selected: boolean;
  onFocus: () => void;
  onRetry: () => void;
  onReplace: () => void;
  onClose: () => void;
  onOpenConfiguration?: () => void;
  showHeader?: boolean;
}

export function WorkspaceTerminalTile({
  identity,
  hostLabel,
  agent,
  selected,
  onFocus,
  onRetry,
  onReplace,
  onClose,
  onOpenConfiguration,
  showHeader = true,
}: WorkspaceTerminalTileProps) {
  const target = targetFor(identity.hostId);
  const interactive = agent?.interactive !== false;
  const alive = agent ? (agent.enabled ?? agent.state !== "stopped") : false;
  const controller = useTerminalSocket(
    identity.agentName,
    Boolean(agent && interactive && alive),
    target,
  );
  const [starting, setStarting] = useState(false);

  const start = async () => {
    setStarting(true);
    try {
      await agentPostOn(target, identity.agentName, "start");
      await onRetry();
      controller.reconnect();
    } catch (error) {
      toast.error(error instanceof ApiError ? error.message : String(error));
    } finally {
      setStarting(false);
    }
  };

  const configuration =
    `/agents/${hostToParam(identity.hostId)}/${encodeURIComponent(identity.agentName)}/configuration`;
  const connectionLabel = !agent
    ? "unavailable"
    : !interactive
      ? "non-interactive"
      : !alive
        ? "stopped"
        : controller.status;

  return (
    <section
      className={cn(
        "flex h-full min-h-0 flex-col overflow-hidden bg-background",
        showHeader ? "rounded-md border" : "border-0",
        selected && showHeader && "ring-2 ring-primary",
      )}
      aria-label={`${identity.agentName} terminal on ${hostLabel || identity.hostId || "Local"}`}
      data-selected={selected || undefined}
      onPointerDown={onFocus}
    >
      {showHeader && (
      <header className="flex h-8 shrink-0 items-center gap-2 border-b bg-muted/50 px-2 text-xs">
        <span className="min-w-0 truncate font-medium" title={identity.agentName}>
          {identity.agentName}
        </span>
        <span className="min-w-0 truncate text-muted-foreground">
          {hostLabel || identity.hostId || "Local"}
        </span>
        <span className="ml-auto shrink-0 text-muted-foreground">{connectionLabel}</span>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={`Close ${identity.agentName} terminal`}
          title="Detach terminal from workspace"
          onPointerDown={(event) => event.stopPropagation()}
          onClick={onClose}
        >
          <X className="size-3.5" />
        </Button>
      </header>
      )}

      {!agent ? (
        <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-3 p-4 text-center text-sm text-muted-foreground">
          <p>Host or agent unavailable. This workspace position was kept.</p>
          <div className="flex flex-wrap justify-center gap-2">
            <Button size="sm" variant="outline" aria-label={`Retry ${identity.agentName}`} onClick={onRetry}>
              <RefreshCw className="size-3.5" />
              Retry
            </Button>
            <Button size="sm" variant="outline" aria-label={`Replace ${identity.agentName}`} onClick={onReplace}>
              Replace
            </Button>
          </div>
        </div>
      ) : !interactive ? (
        <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-2 p-4 text-sm text-muted-foreground">
          <p>This agent has no interactive terminal.</p>
          <Link
            className="text-primary underline"
            to={configuration}
            onClick={(event) => {
              if (!onOpenConfiguration) return;
              event.preventDefault();
              onOpenConfiguration();
            }}
          >
            Open Configuration
          </Link>
        </div>
      ) : !alive ? (
        <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-2 p-4 text-sm text-muted-foreground">
          <p>Agent is stopped.</p>
          <Button
            size="sm"
            aria-label={`Start ${identity.agentName}`}
            disabled={starting}
            onClick={() => void start()}
          >
            {starting ? "Starting…" : "Start"}
          </Button>
        </div>
      ) : (
        <div className="flex min-h-0 flex-1 flex-col">
          <TuiScreen
            key={terminalKey(identity)}
            controller={controller}
            fill
            daemon={target}
            onStart={start}
            persistDraft={false}
            surface="workspace"
          />
        </div>
      )}
    </section>
  );
}
