import { IterationTimeoutControl } from "tariboy-ui";

// Deadlines are relative to the capture's fixed clock (2024-05-15T12:00:00Z);
// server_now is the daemon clock the component counts down from, exactly as
// /api/agents/<name>/status ships it.
const NOW = "2024-05-15T12:00:00Z";

const status = (active: Record<string, unknown> | undefined) =>
  ({
    name: "builder",
    state: "running",
    loop_enabled: true,
    iterations: 48,
    last_iteration: "2024-05-15T11:57:00Z",
    last_iteration_id: "2024-05-15T11-57",
    status_message: "Reviewing TB-142 — approving.",
    status_updated: "2024-05-15T11:59:41Z",
    server_now: NOW,
    active_iteration: active,
  }) as never;

const IterationRow = ({
  name = "builder",
  note = "worker:v2",
  children,
}: {
  name?: string;
  note?: string;
  children: React.ReactNode;
}) => (
  <div
    style={{
      width: 600,
      display: "flex",
      alignItems: "center",
      gap: 16,
      padding: "10px 14px",
      borderRadius: 10,
      border: "1px solid var(--border)",
      background: "var(--card)",
    }}
  >
    <div style={{ display: "grid", gap: 2, minWidth: 0 }}>
      <span style={{ fontSize: 14, fontWeight: 600 }}>{name}</span>
      <span
        style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--muted-foreground)" }}
      >
        iteration 2024-05-15T11-57 · {note}
      </span>
    </div>
    <div style={{ marginLeft: "auto" }}>{children}</div>
  </div>
);

export const Running = () => (
  <IterationRow>
    <IterationTimeoutControl
      name="builder"
      status={status({
        id: "2024-05-15T11-57",
        started_at: "2024-05-15T11:57:00Z",
        timeout_period_s: 1800,
        timeout_deadline: "2024-05-15T12:18:00Z",
        hard_timeout_deadline: "2024-05-15T13:30:00Z",
        effective_deadline: "2024-05-15T12:18:00Z",
        timeout_extensions: 0,
      })}
      refresh={() => {}}
    />
  </IterationRow>
);

export const Extended = () => (
  <IterationRow note="worker:v2 · extended 2×, hard stop 13:30">
    <IterationTimeoutControl
      name="builder"
      status={status({
        id: "2024-05-15T11-57",
        started_at: "2024-05-15T11:57:00Z",
        timeout_period_s: 900,
        timeout_deadline: "2024-05-15T12:12:00Z",
        hard_timeout_deadline: "2024-05-15T13:30:00Z",
        effective_deadline: "2024-05-15T12:12:00Z",
        timeout_extensions: 2,
      })}
      refresh={() => {}}
    />
  </IterationRow>
);

export const AboutToFire = () => (
  <IterationRow>
    <IterationTimeoutControl
      name="builder"
      status={status({
        id: "2024-05-15T11-57",
        started_at: "2024-05-15T11:57:00Z",
        timeout_period_s: 1800,
        timeout_deadline: "2024-05-15T12:00:40Z",
        hard_timeout_deadline: "2024-05-15T13:30:00Z",
        effective_deadline: "2024-05-15T12:00:40Z",
        timeout_extensions: 1,
      })}
      refresh={() => {}}
    />
  </IterationRow>
);

export const NoTimeout = () => (
  <IterationRow name="packager" note="bare:latest">
    <IterationTimeoutControl
      name="packager"
      status={status({
        id: "2024-05-15T11-57",
        started_at: "2024-05-15T11:57:00Z",
        timeout_period_s: 0,
        hard_timeout_deadline: "2024-05-15T13:30:00Z",
        timeout_extensions: 0,
      })}
      refresh={() => {}}
    />
  </IterationRow>
);

export const TimeoutFiring = () => (
  <IterationRow>
    <IterationTimeoutControl
      name="builder"
      status={status({
        id: "2024-05-15T11-57",
        started_at: "2024-05-15T11:57:00Z",
        timeout_period_s: 1800,
        timeout_deadline: "2024-05-15T11:59:30Z",
        hard_timeout_deadline: "2024-05-15T13:30:00Z",
        effective_deadline: "2024-05-15T11:59:30Z",
        timeout_extensions: 0,
      })}
      refresh={() => {}}
    />
  </IterationRow>
);
