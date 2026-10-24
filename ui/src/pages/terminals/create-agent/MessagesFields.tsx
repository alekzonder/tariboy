import { NumberField } from "./formControls";
import type { CreateAgentForm } from "./useCreateAgentForm";

export function MessagesFields({
  draft,
  updateDraft,
  formDisabled,
}: Pick<CreateAgentForm, "draft" | "updateDraft" | "formDisabled">) {
  return (
    <>
      <NumberField
        label="message batch size"
        id="create-agent-message-batch"
        value={draft.messagesBatch}
        onChange={(value) => updateDraft("messagesBatch", value)}
        disabled={formDisabled}
        min={1}
      />
      <NumberField
        label="maximum queued messages"
        id="create-agent-message-queue"
        value={draft.messagesMaxQueue}
        onChange={(value) => updateDraft("messagesMaxQueue", value)}
        disabled={formDisabled}
        min={1}
      />
    </>
  );
}
