import { Field } from "./formControls";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import type { CreateAgentForm } from "./useCreateAgentForm";

export function IdentitySection({
  draft,
  updateDraft,
  formDisabled,
}: Pick<CreateAgentForm, "draft" | "updateDraft" | "formDisabled">) {
  return (
    <>
      <div className="grid gap-3 sm:grid-cols-3">
        <Field label="alias" id="create-agent-alias">
          <Input
            id="create-agent-alias"
            aria-label="alias"
            value={draft.alias}
            onChange={(event) => updateDraft("alias", event.target.value)}
            disabled={formDisabled}
            className="h-8"
          />
        </Field>
        <Field label="team" id="create-agent-group">
          <Input
            id="create-agent-group"
            aria-label="team"
            value={draft.group}
            onChange={(event) => updateDraft("group", event.target.value)}
            disabled={formDisabled}
            className="h-8"
          />
        </Field>
        <Field label="color" id="create-agent-color">
          <Input
            id="create-agent-color"
            aria-label="color"
            value={draft.color}
            onChange={(event) => updateDraft("color", event.target.value)}
            placeholder="#rrggbb"
            disabled={formDisabled}
            className="h-8"
          />
        </Field>
      </div>
      <Field label="notes" id="create-agent-notes">
        <Textarea
          id="create-agent-notes"
          aria-label="notes"
          value={draft.notes}
          onChange={(event) => updateDraft("notes", event.target.value)}
          disabled={formDisabled}
        />
      </Field>
    </>
  );
}
