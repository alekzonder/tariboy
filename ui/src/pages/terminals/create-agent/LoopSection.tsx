import { Field, NumberField, PolicyField } from "./formControls";
import { Textarea } from "@/components/ui/textarea";
import { MessagesFields } from "./MessagesFields";
import type { CreateAgentForm } from "./useCreateAgentForm";

/** The Autopilot group: how the agent keeps working without you. */
export function LoopSection({
  draft,
  updateDraft,
  formDisabled,
}: Pick<CreateAgentForm, "draft" | "updateDraft" | "formDisabled">) {
  return (
    <>
      <div className="grid gap-3 sm:grid-cols-2">
        <NumberField
          label="interval seconds"
          id="create-agent-interval"
          value={draft.intervalS}
          onChange={(value) => updateDraft("intervalS", value)}
          disabled={formDisabled}
          min={0}
        />
        <NumberField
          label="soft timeout seconds"
          id="create-agent-timeout"
          value={draft.timeoutS}
          onChange={(value) => updateDraft("timeoutS", value)}
          disabled={formDisabled}
          min={0}
        />
        <NumberField
          label="hard timeout seconds"
          id="create-agent-hard-timeout"
          value={draft.hardTimeoutS}
          onChange={(value) => updateDraft("hardTimeoutS", value)}
          disabled={formDisabled}
          min={0}
        />
        <PolicyField
          label="timeout policy"
          id="create-agent-on-timeout"
          value={draft.onTimeout}
          onChange={(value) => updateDraft("onTimeout", value)}
          disabled={formDisabled}
        />
        <PolicyField
          label="error policy"
          id="create-agent-on-error"
          value={draft.onError}
          onChange={(value) => updateDraft("onError", value)}
          disabled={formDisabled}
        />
        <NumberField
          label="maximum idle iterations"
          id="create-agent-max-idle"
          value={draft.maxIdleIterations}
          onChange={(value) => updateDraft("maxIdleIterations", value)}
          disabled={formDisabled}
          min={0}
        />
        <NumberField
          label="AI inactivity timeout seconds"
          id="create-agent-ai-stall-timeout"
          value={draft.aiStallTimeoutS}
          onChange={(value) => updateDraft("aiStallTimeoutS", value)}
          disabled={formDisabled}
          min={1}
        />
        <MessagesFields draft={draft} updateDraft={updateDraft} formDisabled={formDisabled} />
      </div>
      <Field label="standing user prompt" id="create-agent-user-prompt">
        <Textarea
          id="create-agent-user-prompt"
          aria-label="standing user prompt"
          value={draft.userPrompt}
          onChange={(event) => updateDraft("userPrompt", event.target.value)}
          disabled={formDisabled}
        />
      </Field>
    </>
  );
}
