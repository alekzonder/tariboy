import { SwitchRow } from "./formControls";
import type { CreateAgentForm } from "./useCreateAgentForm";

export function LifecycleSection({
  draft,
  updateDraft,
  formDisabled,
}: Pick<
  CreateAgentForm,
  | "draft"
  | "updateDraft"
  | "formDisabled"
>) {
  return (
    <section
      aria-labelledby="create-agent-lifecycle"
      className="space-y-3 rounded border p-3"
    >
      <h3 id="create-agent-lifecycle" className="font-medium">
        Lifecycle
      </h3>
      <SwitchRow
        label="Start now"
        id="create-agent-start"
        checked={draft.startNow}
        onCheckedChange={(value) => updateDraft("startNow", value)}
        disabled={formDisabled}
        help="Off creates the agent in stopped state."
      />
    </section>
  );
}
