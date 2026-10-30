import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { harnessLabel } from "@/lib/onboarding/firstRun";

/** One line above the terminal while it is not usable yet: the coding tool
 * starting, or why it did not start with a Retry. Nothing once it is open. */
export function TerminalStatusLine({
  harness,
  cwd,
  starting,
  reason,
  configuration,
  onRetry,
}: {
  harness: string;
  cwd: string;
  /** The agent runs, but the terminal is still connecting or has no session. */
  starting: boolean;
  /** The agent's halt or error reason. */
  reason?: string;
  configuration: string;
  onRetry: () => void;
}) {
  // A plain terminal (empty harness) runs no coding tool.
  const tool = harness ? harnessLabel(harness) : "the terminal";
  if (reason) {
    return (
      <div
        role="status"
        className="flex flex-wrap items-center gap-2 rounded-md border border-destructive/40 px-3 py-1.5 text-sm"
      >
        <span className="min-w-0 flex-1 break-words text-destructive">
          {harness ? tool : "The terminal"} did not start: {reason}
        </span>
        <Button size="sm" variant="outline" onClick={onRetry}>
          Retry
        </Button>
        <Link className="text-primary underline" to={configuration}>
          Open settings
        </Link>
      </div>
    );
  }
  if (!starting) return null;
  return (
    <div role="status" className="px-1 text-sm text-muted-foreground">
      Starting {tool}
      {cwd ? ` in ${cwd}` : ""}…
    </div>
  );
}
