import { IterationAuditLog } from "tariboy-ui";

// IterationAuditLog streams one iteration's audit events: it fetches
// /logs?iteration=… plus the proxy transcript on mount, then refreshes on a
// timer and on the agent's SSE events. None of that can resolve in a preview,
// so what renders is the component's frame — the mono iteration chip, the audit
// export actions, and the bottom-pinned scroll pane in its "No events." state.
// That empty is a real product state (a queued iteration that has not logged
// yet), not a placeholder.
//
// The component is `h-full` and is always mounted into a flex column that has
// already claimed the page's leftover height, so the preview supplies one.

const Pane = ({ children }: { children: React.ReactNode }) => (
  <div style={{ height: 260, maxWidth: 720, display: "flex", flexDirection: "column" }}>
    {children}
  </div>
);

// The Overview / Audit-log pane while the current iteration is running. The
// chip also carries a trailing " · idle" when an iteration finished with
// `i-am-done --idle`; that differs by two words in the same chip, so it is not
// split into its own cell.
export const RunningIteration = () => (
  <Pane>
    <IterationAuditLog
      name="builder"
      iterationId="builder-20260911182241-48"
      iterationStatus="running"
    />
  </Pane>
);

// A fresh agent: no iteration selected, so the chip says so and the export
// actions are withheld (there is nothing to export yet).
export const NoIterations = () => (
  <Pane>
    <IterationAuditLog name="docs-bot" iterationId="" iterationStatus="" />
  </Pane>
);
