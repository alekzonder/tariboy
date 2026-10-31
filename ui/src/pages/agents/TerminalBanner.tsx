import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { harnessLabel } from "@/lib/onboarding/firstRun";

/** What the terminal screen shows, as one line above it. */
export type BannerState =
  | { kind: "error"; reason: string }
  | { kind: "no-terminal" }
  | { kind: "off" }
  | { kind: "starting" }
  | { kind: "working"; taskKey: string }
  | { kind: "idle" };

const LINE = "flex flex-wrap items-center gap-2 px-1 text-sm text-muted-foreground";

/** One line above the terminal, always: what the screen shows and what to do.
 *  An error carries Retry; an agent that is off carries Start. */
export function TerminalBanner({
  state,
  harness,
  cwd,
  settings,
  onRetry,
  onStart,
}: {
  state: BannerState;
  harness: string;
  cwd: string;
  /** The agent's Settings tab. */
  settings: string;
  onRetry: () => void;
  onStart: () => void;
}) {
  // A plain terminal (empty harness) runs no coding tool.
  const tool = harness ? harnessLabel(harness) : "the terminal";
  switch (state.kind) {
    case "error":
      return (
        <div
          role="status"
          className="flex flex-wrap items-center gap-2 rounded-md border border-destructive/40 px-3 py-1.5 text-sm"
        >
          <span className="min-w-0 flex-1 break-words text-destructive">
            {harness ? tool : "The terminal"} did not start: {state.reason}
          </span>
          <Button size="sm" variant="outline" onClick={onRetry}>
            Retry
          </Button>
          <Link className="text-primary underline" to={settings}>
            Open settings
          </Link>
        </div>
      );
    case "no-terminal":
      return (
        <div role="status" className={LINE}>
          <span>
            No interactive terminal. Enable it in{" "}
            <Link className="text-primary underline" to={settings}>
              Settings → Autopilot
            </Link>
            .
          </span>
        </div>
      );
    case "off":
      return (
        <div role="status" className={LINE}>
          <span>Off.</span>
          <Button size="sm" onClick={onStart}>
            Start
          </Button>
        </div>
      );
    case "starting":
      return (
        <div role="status" className={LINE}>
          Starting {tool}
          {cwd ? ` in ${cwd}` : ""}…
        </div>
      );
    case "working":
      return (
        <div role="status" className={LINE}>
          {state.taskKey ? `Working on ${state.taskKey}` : "Working"} — this is its live screen.
        </div>
      );
    default:
      return (
        <div role="status" className={LINE}>
          Idle. The screen appears when a run starts. Type here to talk to the agent directly; this
          is not recorded in a task.
        </div>
      );
  }
}
