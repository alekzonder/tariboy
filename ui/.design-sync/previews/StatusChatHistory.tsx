import { useEffect, useRef } from "react";
import { StatusChatHistory } from "tariboy-ui";

// The Overview status card: cwd on the left, the newest status message on the
// right, and the history trigger at the end of that line.
const StatusCard = ({
  cwd,
  message,
  children,
}: {
  cwd: string;
  message: string;
  children: React.ReactNode;
}) => (
  <div
    style={{
      width: 680,
      display: "flex",
      alignItems: "center",
      gap: 16,
      padding: "8px 14px",
      borderRadius: 10,
      border: "1px solid var(--border)",
      background: "var(--card)",
    }}
  >
    <span style={{ fontFamily: "var(--font-mono)", fontSize: 12 }}>cwd: {cwd}</span>
    <div
      style={{
        marginLeft: "auto",
        display: "flex",
        alignItems: "center",
        gap: 8,
        fontSize: 14,
      }}
    >
      <span style={{ color: "var(--muted-foreground)" }}>{message}</span>
      {children}
    </div>
  </div>
);

// The sheet's open state is internal to StatusChatHistory, so the open cell
// presses the real trigger. The list stays empty on purpose: history is
// lazy-fetched from /api when the sheet opens and a preview has no daemon, so
// this is the component's genuine empty state.
function usePressHistory() {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    ref.current?.querySelector<HTMLButtonElement>('[aria-label="status history"]')?.click();
  }, []);
  return ref;
}

export const InStatusCard = () => (
  <StatusCard cwd="/work/tariboy" message="12:04 — waiting on builder to push a fix">
    <StatusChatHistory name="reviewer" />
  </StatusCard>
);

export const NoStatusMessage = () => (
  <StatusCard cwd="/work/docs" message="no message">
    <StatusChatHistory name="docs-bot" />
  </StatusCard>
);

export const HistoryOpen = () => {
  const ref = usePressHistory();
  return (
    <div ref={ref}>
      <StatusCard cwd="/work/tariboy" message="12:04 — waiting on builder to push a fix">
        <StatusChatHistory name="reviewer" />
      </StatusCard>
    </div>
  );
};
