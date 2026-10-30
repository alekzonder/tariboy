import { useState } from "react";
import { EditablePresetCombobox } from "@/components/EditablePresetCombobox";
import type { OnboardingHarness } from "@/lib/api";
import { DESKTOP_HARNESSES, harnessLabel } from "@/lib/onboarding/firstRun";
import { Field } from "./formControls";
import type { CreateAgentForm } from "./useCreateAgentForm";

/** The tools to offer: the host's detection, or every Desktop tool unmarked on
 *  a daemon without it. A value from a clone outside the list stays offered. */
function toolOptions(detected: OnboardingHarness[] | null, current: string) {
  const options: { type: string; found: boolean | null; install: string }[] = detected
    ? detected.map((tool) => ({ ...tool }))
    : DESKTOP_HARNESSES.map((type) => ({ type, found: null, install: "" }));
  if (current && !options.some((option) => option.type === current)) {
    options.push({ type: current, found: null, install: "" });
  }
  return options;
}

export function CodingToolField({
  form,
}: {
  form: Pick<
    CreateAgentForm,
    | "draft"
    | "updateDraft"
    | "changeHarness"
    | "formDisabled"
    | "loadingManifest"
    | "onboarding"
    | "modelPresets"
    | "effortPresets"
    | "cloneSource"
  >;
}) {
  const { draft, updateDraft, formDisabled, loadingManifest } = form;
  const [presetsOpen, setPresetsOpen] = useState(form.cloneSource !== undefined);
  const disabled = formDisabled || loadingManifest;
  const options = toolOptions(form.onboarding.status?.harnesses ?? null, draft.harness);
  const selected = options.find((option) => option.type === draft.harness);
  return (
    <fieldset className="space-y-1">
      <legend className="text-sm font-medium">Coding tool</legend>
      <div role="radiogroup" aria-label="Coding tool" className="flex flex-wrap gap-x-4 gap-y-1">
        {options.map((option) => (
          <label key={option.type} className="flex items-center gap-1.5 text-sm">
            <input
              type="radio"
              name="create-agent-harness"
              value={option.type}
              checked={draft.harness === option.type}
              onChange={() => form.changeHarness(option.type)}
              disabled={disabled}
              aria-label={harnessLabel(option.type)}
            />
            {harnessLabel(option.type)}
            <span className="text-xs text-muted-foreground">· {option.type}</span>
            {option.found === false && <span className="text-xs text-destructive">not found</span>}
          </label>
        ))}
      </div>
      {selected?.found === false && selected.install && (
        <p className="text-xs text-muted-foreground">To install it: {selected.install}</p>
      )}
      {!presetsOpen && (
        <p className="text-xs text-muted-foreground">
          Model: {draft.model || "tool default"} · Effort: {draft.effort || "tool default"}{" "}
          <button
            type="button"
            className="underline underline-offset-2"
            onClick={() => setPresetsOpen(true)}
          >
            change
          </button>
        </p>
      )}
      {presetsOpen && (
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Model" id="create-agent-model">
            <EditablePresetCombobox
              id="create-agent-model"
              ariaLabel="model"
              value={draft.model}
              groups={form.modelPresets}
              onChange={(value) => updateDraft("model", value)}
              placeholder="tool default"
              disabled={disabled}
            />
          </Field>
          <Field label="Effort" id="create-agent-effort">
            <EditablePresetCombobox
              id="create-agent-effort"
              ariaLabel="effort"
              value={draft.effort}
              groups={form.effortPresets}
              onChange={(value) => updateDraft("effort", value)}
              placeholder="tool default"
              disabled={disabled}
            />
          </Field>
        </div>
      )}
    </fieldset>
  );
}
