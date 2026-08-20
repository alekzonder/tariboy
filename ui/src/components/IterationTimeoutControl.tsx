import { useEffect, useState } from "react";
import { toast } from "sonner";
import { ApiError, extendIterationTimeout, type TimeoutExtension } from "@/lib/api";
import type { ActiveIteration, AgentStatus } from "@/lib/types";
import { Button } from "@/components/ui/button";

type Snapshot = Pick<ActiveIteration, "id" | "timeout_period_s" | "timeout_deadline" | "hard_timeout_deadline" | "effective_deadline" | "timeout_extensions">;

function roundedRemaining(ms: number): string {
  if (ms < 60_000) return "<1m";
  return `${Math.ceil(ms / 60_000)}m`;
}

function localDeadline(value: string): string {
  return new Date(value).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

// A small, self-contained live control: status supplies a server-clocked
// snapshot while a successful POST supplies the canonical replacement snapshot.
export function IterationTimeoutControl({ name, status, refresh }: {
  name: string;
  status: AgentStatus | null;
  refresh: () => Promise<void> | void;
}) {
  const active = status?.active_iteration;
  if (!active) return null;
  const key = `${active.id}:${active.timeout_deadline}:${active.hard_timeout_deadline}:${status.server_now}`;
  return <ActiveTimeoutControl key={key} name={name} active={active} serverNow={status.server_now} refresh={refresh} />;
}

function ActiveTimeoutControl({ name, active, serverNow, refresh }: {
  name: string;
  active: ActiveIteration;
  serverNow?: string;
  refresh: () => Promise<void> | void;
}) {
  // The status response already contains the authoritative sampled server
  // clock. Advance that snapshot monotonically instead of calling Date.now()
  // during render (which is impure and would also reintroduce browser skew).
  const [elapsed, setElapsed] = useState(0);
  const [pending, setPending] = useState(false);
  const [override, setOverride] = useState<Snapshot | null>(null);
  const snapshot = override?.id === active.id ? { ...active, ...override } : active;

  useEffect(() => {
    const timer = window.setInterval(() => setElapsed((value) => value + 1000), 1000);
    return () => window.clearInterval(timer);
  }, []);

  const deadline = snapshot.effective_deadline ?? snapshot.timeout_deadline;
  // A zero-period iteration deliberately has no soft deadline and cannot be
  // extended. The hard watchdog is not an operator timeout control.
  if (!snapshot.timeout_period_s || !snapshot.timeout_deadline || !deadline) {
    return <span className="whitespace-nowrap text-xs text-muted-foreground">No timeout</span>;
  }
  const sampledServerNow = Date.parse(serverNow ?? active.started_at);
  const remaining = Date.parse(deadline) -
    (Number.isFinite(sampledServerNow) ? sampledServerNow + elapsed : elapsed);
  if (remaining <= 0) {
    return <span className="whitespace-nowrap text-xs text-muted-foreground">timeout firing…</span>;
  }
  const period = `${Math.round(snapshot.timeout_period_s / 60)}m`;
  const extend = async () => {
    setPending(true);
    try {
      const result: TimeoutExtension = await extendIterationTimeout(name, snapshot.id);
      setOverride({
        id: result.id, timeout_period_s: snapshot.timeout_period_s,
        timeout_deadline: result.timeout_deadline, hard_timeout_deadline: result.hard_timeout_deadline,
        effective_deadline: result.timeout_deadline, timeout_extensions: result.timeout_extensions,
      });
      toast.success(`timeout extended by ${period}`);
      if (result.shim_sync === "pending") toast.warning("timeout saved; shim sync is pending");
      void refresh();
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        toast.warning("timeout changed; refreshed current status");
        await refresh();
      } else {
        toast.error(e instanceof ApiError ? e.message : String(e));
      }
    } finally {
      setPending(false);
    }
  };
  return <div className="flex shrink-0 items-center gap-1 whitespace-nowrap text-xs text-muted-foreground" aria-label="Iteration timeout">
    <span>Timeout {localDeadline(deadline)} (in {roundedRemaining(remaining)})</span>
    <Button size="sm" variant="secondary" disabled={pending} onClick={() => void extend()}>
      {pending ? "Extending…" : `+${period}`}
    </Button>
  </div>;
}
