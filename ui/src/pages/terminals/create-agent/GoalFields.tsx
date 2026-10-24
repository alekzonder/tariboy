import { NumberField, SwitchRow } from "./formControls";
import type { CreateAgentForm } from "./useCreateAgentForm";

export function GoalFields({
  draft,
  updateDraft,
  formDisabled,
}: Pick<CreateAgentForm, "draft" | "updateDraft" | "formDisabled">) {
  return (
    <>
      <SwitchRow
        label="Goal"
        id="create-agent-goal"
        checked={draft.goalEnabled}
        onCheckedChange={(value) => updateDraft("goalEnabled", value)}
        disabled={formDisabled}
        help="Select and deliver the agent's current Native Task goal."
      />
      <NumberField
        label="wait customer timeout seconds"
        id="create-agent-goal-wait-customer-timeout"
        value={draft.goalWaitCustomerTimeoutS}
        onChange={(value) => updateDraft("goalWaitCustomerTimeoutS", value)}
        disabled={formDisabled}
        min={1}
      />
      <NumberField
        label="goal delivery cooldown seconds"
        id="create-agent-goal-delivery-cooldown"
        value={draft.goalDeliveryCooldownS}
        onChange={(value) => updateDraft("goalDeliveryCooldownS", value)}
        disabled={formDisabled}
        min={1}
      />
    </>
  );
}
