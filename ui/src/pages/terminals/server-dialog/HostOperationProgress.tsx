import { HOST_PROGRESS_STEPS, hostStepStates } from "@/lib/hostProgress";
import { HostDiagnostics } from "./HostDiagnostics";
import type { ServerForm } from "./useServerForm";

export function HostOperationProgress({
  operation,
  displayHost,
  output,
}: Pick<ServerForm, "operation" | "displayHost" | "output">) {
  const stepStates = hostStepStates(operation.currentStep, operation.status);
  const progressSteps =
    operation.kind === "connect"
      ? HOST_PROGRESS_STEPS.filter((step) => step.id === "connect" || step.id === "reconnect")
      : HOST_PROGRESS_STEPS;
  return (
    <section className="min-w-0 space-y-2 rounded-lg border p-3">
      <h3 className="text-sm font-medium">
        {operation.kind === "update"
          ? "Update Tariboy"
          : operation.kind === "connect"
            ? "Connect to existing Tariboy"
            : "Connect Tariboy"}
      </h3>
      <ol className="space-y-1.5 text-sm">
        {progressSteps.map((step) => {
          const state = stepStates[step.id];
          const label =
            operation.kind === "update"
              ? step.updateLabel
              : operation.kind === "connect" && step.id === "reconnect"
                ? "Check existing Tariboy"
                : step.provisionLabel;
          const marker =
            state === "complete" ? "✓" : state === "failed" ? "×" : state === "active" ? "●" : "○";
          return (
            <li
              key={step.id}
              data-testid="host-progress-step"
              data-step={step.id}
              aria-current={state === "active" ? "step" : undefined}
              className={
                state === "failed"
                  ? "flex min-w-0 items-center gap-2 font-medium text-destructive"
                  : state === "active"
                    ? "flex min-w-0 items-center gap-2 font-medium text-foreground"
                    : state === "complete"
                      ? "flex min-w-0 items-center gap-2 text-foreground"
                      : "flex min-w-0 items-center gap-2 text-muted-foreground"
              }
            >
              <span aria-hidden="true" className="w-4 shrink-0 text-center">
                {marker}
              </span>
              <span className="min-w-0">{label}</span>
              <span className="sr-only"> — {state}</span>
            </li>
          );
        })}
      </ol>

      {operation.status === "succeeded" && (
        <div className="rounded-md bg-emerald-500/10 p-2 text-sm text-emerald-700 dark:text-emerald-300">
          <p className="font-medium">
            {operation.kind === "update" ? "Update complete" : "Host connected"}
          </p>
          <p>Tariboy is ready on this host.</p>
        </div>
      )}

      {operation.status === "failed" && operation.error && (
        <div
          role="alert"
          className="min-w-0 rounded-md bg-destructive/10 p-2 text-sm text-destructive"
        >
          <p className="font-medium">
            {operation.kind === "update" ? "Update failed" : "Connection failed"}
          </p>
          <p className="break-words [overflow-wrap:anywhere]">{operation.error}</p>
        </div>
      )}

      <HostDiagnostics
        displayHost={displayHost}

        output={output}
      />
    </section>
  );
}
