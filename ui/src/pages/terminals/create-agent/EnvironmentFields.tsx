import { Field } from "./formControls";
import { Textarea } from "@/components/ui/textarea";
import type { CreateAgentForm } from "./useCreateAgentForm";

export function EnvironmentFields({
  draft,
  updateDraft,
  formDisabled,
}: Pick<CreateAgentForm, "draft" | "updateDraft" | "formDisabled">) {
  return (
    <Field label="environment JSON" id="create-agent-env">
      <Textarea
        id="create-agent-env"
        aria-label="environment JSON"
        value={draft.envText}
        onChange={(event) => updateDraft("envText", event.target.value)}
        disabled={formDisabled}
        className="min-h-28 font-mono text-xs"
      />
    </Field>
  );
}
