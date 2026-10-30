import { Button } from "@/components/ui/button";
import type { CreateAgentForm } from "./useCreateAgentForm";

/** The clone source could not be read: say why and offer to read it again. */
export function SourceAlert({ form }: { form: CreateAgentForm }) {
  const { sourceState, sourceError, retrySource } = form;
  return (
    <>
      {sourceState === "error" && (
        <div role="alert" className="space-y-2 rounded border border-destructive p-3">
          <p className="text-sm text-destructive">{sourceError}</p>
          <Button size="sm" variant="outline" onClick={retrySource}>
            Retry source
          </Button>
        </div>
      )}
    </>
  );
}

/** A rejected create, or a created agent that did not start. */
export function StartAlerts({ form }: { form: CreateAgentForm }) {
  const { submitError, blocker, startError, busy, targetIsCurrent, targetIsUsable, retryStart } =
    form;
  return (
    <>
      {submitError && !blocker && (
        <p role="alert" className="text-sm text-destructive">
          {submitError}
        </p>
      )}
      {startError && (
        <div role="alert" className="space-y-2 rounded border border-destructive p-3">
          <p className="text-sm text-destructive">{startError}</p>
          <Button
            disabled={busy || !targetIsCurrent || !targetIsUsable}
            size="sm"
            onClick={() => void retryStart()}
          >
            Retry start
          </Button>
        </div>
      )}
    </>
  );
}

/** What still blocks the create, and the create button itself. */
export function CreateAgentFooter({
  form,
  tourAnchor,
  onCancel,
}: {
  form: CreateAgentForm;
  /** The tour's anchor on the first-run card's Start button. */
  tourAnchor?: string;
  /** Shown in the dialog; the inline card has nothing to cancel. */
  onCancel?: () => void;
}) {
  const { created, blocker, retryImage, busy, targetIsUsable, submit } = form;
  return (
    <>
      {!created && blocker && (
        <div className="mr-auto flex items-center gap-2">
          <p
            id="create-agent-blocker"
            role={blocker.error ? "alert" : "status"}
            className={blocker.error ? "text-sm text-destructive" : "text-sm text-muted-foreground"}
          >
            {blocker.message}
          </p>
          {blocker.retryImage && (
            <Button size="sm" variant="outline" onClick={retryImage}>
              Retry image
            </Button>
          )}
        </div>
      )}
      {onCancel && (
        <Button variant="outline" onClick={onCancel}>
          Cancel
        </Button>
      )}
      {!created && (
        <Button
          disabled={busy || blocker !== null || !targetIsUsable}
          aria-describedby={blocker ? "create-agent-blocker" : undefined}
          data-tour={tourAnchor}
          onClick={() => void submit()}
        >
          {busy ? "Creating…" : form.draft.startNow ? "Start agent" : "Create agent"}
        </Button>
      )}
    </>
  );
}
