import { useState } from "react";
import { AlertTriangle, Check, Loader2, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { OnboardingStatus } from "@/lib/api";
import { checklistRows, harnessLabel, type ChecklistRow } from "@/lib/onboarding/firstRun";
import { errorText } from "@/lib/errors";

const ICONS = {
  done: <Check className="size-4 text-emerald-600" aria-hidden />,
  working: <Loader2 className="size-4 animate-spin text-muted-foreground" aria-hidden />,
  failed: <X className="size-4 text-destructive" aria-hidden />,
  warn: <AlertTriangle className="size-4 text-amber-600" aria-hidden />,
};

/** The "Getting ready" rows of one host: what the daemon set up by itself,
 *  with Details and Retry on a failed row. */
export function GettingReady({
  status,
  retry,
}: {
  status: OnboardingStatus;
  retry: () => Promise<void>;
}) {
  return (
    <ul aria-label="Getting ready" className="space-y-1.5">
      {checklistRows(status).map((row) => (
        <ChecklistItem key={row.key} row={row} retry={retry} />
      ))}
    </ul>
  );
}

function ChecklistItem({ row, retry }: { row: ChecklistRow; retry: () => Promise<void> }) {
  const [busy, setBusy] = useState(false);
  const [retryError, setRetryError] = useState("");
  const runRetry = async () => {
    setBusy(true);
    setRetryError("");
    try {
      await retry();
    } catch (error) {
      setRetryError(errorText(error));
    } finally {
      setBusy(false);
    }
  };
  return (
    <li data-state={row.state} className="text-sm">
      <div className="flex flex-wrap items-center gap-2">
        {ICONS[row.state]}
        <span>{row.text}</span>
        {row.state === "failed" && (
          <Button size="xs" variant="outline" disabled={busy} onClick={() => void runRetry()}>
            Retry
          </Button>
        )}
      </div>
      {row.error && (
        <details className="ml-6 text-xs text-muted-foreground">
          <summary className="cursor-pointer">Details</summary>
          <pre className="whitespace-pre-wrap break-words">{row.error}</pre>
        </details>
      )}
      {row.install && row.install.length > 0 && (
        <details className="ml-6 text-xs text-muted-foreground" open={row.state === "warn"}>
          <summary className="cursor-pointer">
            {row.state === "warn" ? "How to install" : "Install another…"}
          </summary>
          <ul className="space-y-0.5">
            {row.install.map((tool) => (
              <li key={tool.type}>
                {harnessLabel(tool.type)}: {tool.install}
              </li>
            ))}
          </ul>
        </details>
      )}
      {retryError && (
        <p role="alert" className="ml-6 text-xs text-destructive">
          {retryError}
        </p>
      )}
    </li>
  );
}
