import { useEffect, useRef } from "react";
import { LoopToggle, Badge } from "tariboy-ui";

// The Autopilot tab header LoopToggle sits in: agent state on the left, the
// toggle pinned right.
const AutopilotHeader = ({
  state,
  detail,
  halted,
  children,
}: {
  state: string;
  detail: string;
  halted?: string;
  children: React.ReactNode;
}) => (
  <div
    style={{
      width: 620,
      display: "flex",
      alignItems: "flex-start",
      gap: 16,
      padding: "12px 14px",
      borderRadius: 10,
      border: "1px solid var(--border)",
      background: "var(--card)",
    }}
  >
    <div style={{ display: "grid", gap: 4 }}>
      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
        <span style={{ fontSize: 14, fontWeight: 600 }}>builder</span>
        <Badge variant="secondary">{state}</Badge>
      </div>
      <div style={{ fontSize: 13, color: "var(--muted-foreground)" }}>{detail}</div>
      {halted ? <div className="text-destructive" style={{ fontSize: 13 }}>{halted}</div> : null}
    </div>
    <div style={{ marginLeft: "auto" }}>{children}</div>
  </div>
);

// The enabled toggle is a ConfirmButton — its AlertDialog state is internal, so
// the confirm cell presses the real Pause trigger on mount.
function usePressPause() {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const buttons = Array.from(ref.current?.querySelectorAll("button") ?? []);
    buttons.find((button) => button.textContent?.trim() === "Pause")?.click();
  }, []);
  return ref;
}

export const Paused = () => (
  <AutopilotHeader state="idle" detail="48 iterations · last run 11:57">
    <LoopToggle name="builder" enabled={false} onChanged={() => {}} />
  </AutopilotHeader>
);

export const Running = () => (
  <AutopilotHeader state="running" detail="49 iterations · iteration 2024-05-15T11-57 in flight">
    <LoopToggle name="builder" enabled onChanged={() => {}} />
  </AutopilotHeader>
);

export const HaltedByError = () => (
  <AutopilotHeader
    state="error"
    detail="48 iterations · stopped 11:59"
    halted="harness exited 1 — image worker:v2 has no /work/tariboy checkout"
  >
    <LoopToggle name="builder" enabled={false} onChanged={() => {}} />
  </AutopilotHeader>
);

export const PauseConfirm = () => {
  const ref = usePressPause();
  return (
    <div ref={ref}>
      <AutopilotHeader state="running" detail="49 iterations · iteration 2024-05-15T11-57 in flight">
        <LoopToggle name="builder" enabled onChanged={() => {}} />
      </AutopilotHeader>
    </div>
  );
};
