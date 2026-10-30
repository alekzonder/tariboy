import { GettingReady } from "@/components/onboarding/GettingReady";
import { TOUR_ANCHORS } from "@/lib/onboarding/tour";
import type { CreateAgentDialogProps } from "./CreateAgentDialog";
import { NewAgentForm } from "./create-agent/NewAgentForm";
import { CreateAgentFooter } from "./create-agent/CreateAgentAlerts";
import { useCreateAgentForm } from "./create-agent/useCreateAgentForm";

const keepOpen = () => {};

/** The home of a workspace with no agent yet: what the daemon set up by
 *  itself, and the New agent form inline. */
export function FirstRunHome({
  hostId,
  hosts,
  onCreated,
}: Pick<CreateAgentDialogProps, "hostId" | "hosts" | "onCreated">) {
  const form = useCreateAgentForm({ open: true, hostId, hosts, onCreated, onOpenChange: keepOpen });
  const { status, retry } = form.onboarding;
  return (
    <div className="mx-auto max-w-2xl space-y-6 overflow-y-auto p-6">
      <div className="space-y-1">
        <h1 className="text-2xl font-semibold">Welcome to Tariboy</h1>
        <p className="text-muted-foreground">Run an AI coding agent in one of your repositories.</p>
      </div>
      {status && (
        <section aria-labelledby="first-run-ready" className="space-y-2">
          <h2 id="first-run-ready" className="font-medium">
            Getting ready
          </h2>
          <GettingReady status={status} retry={retry} />
        </section>
      )}
      <section aria-labelledby="first-run-card" className="space-y-4 rounded-lg border p-4">
        <h2 id="first-run-card" className="font-medium">
          Run your first agent
        </h2>
        <NewAgentForm form={form} checklistShown />
        <div className="flex items-center justify-end gap-2">
          <CreateAgentFooter form={form} tourAnchor={TOUR_ANCHORS.newAgent} />
        </div>
      </section>
      <p className="text-sm text-muted-foreground">
        Later: connect another computer over SSH in Settings.
      </p>
    </div>
  );
}
