import { Field } from "./formControls";
import { HARNESSES } from "@/components/agentFormValues";
import { EditablePresetCombobox } from "@/components/EditablePresetCombobox";
import { PathAutocomplete } from "@/components/PathAutocomplete";
import { ApiProvider } from "@/components/ApiProvider";
import { unresolvedDaemon } from "@/lib/daemons";
import type { CreateAgentForm } from "./useCreateAgentForm";

export function RuntimeFields({
  host,
  hosts,
  draft,
  updateDraft,
  formDisabled,
  loadingManifest,
  effortPresets,
  modelPresets,
  target,
  targetIsCurrent,
}: Pick<
  CreateAgentForm,
  | "host"
  | "hosts"
  | "draft"
  | "updateDraft"
  | "formDisabled"
  | "loadingManifest"
  | "effortPresets"
  | "modelPresets"
  | "target"
  | "targetIsCurrent"
>) {
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <Field label="Harness" id="create-agent-harness">
        <select
          id="create-agent-harness"
          aria-label="harness"
          value={draft.harness}
          onChange={(event) =>
            updateDraft("harness", event.target.value)
          }
          disabled={formDisabled || loadingManifest}
          className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm"
        >
          <option value="">image default</option>
          {HARNESSES.map((value) => (
            <option key={value} value={value}>
              {value}
            </option>
          ))}
        </select>
      </Field>
      <Field label="Effort" id="create-agent-effort">
        <EditablePresetCombobox
          id="create-agent-effort"
          ariaLabel="effort"
          value={draft.effort}
          groups={effortPresets}
          onChange={(value) => updateDraft("effort", value)}
          placeholder="image default"
          disabled={formDisabled || loadingManifest}
        />
      </Field>
      <div className="sm:col-span-2">
        <Field label="Model" id="create-agent-model">
          <EditablePresetCombobox
            id="create-agent-model"
            ariaLabel="model"
            value={draft.model}
            groups={modelPresets}
            onChange={(value) => updateDraft("model", value)}
            placeholder="image default"
            disabled={formDisabled || loadingManifest}
          />
        </Field>
      </div>
      <div className="sm:col-span-2">
        <Field label="cwd" id="create-agent-cwd">
          {/* The cwd is browsed on the host the agent is created on. */}
          <ApiProvider
            target={
              host === ""
                ? null
                : targetIsCurrent && target
                  ? target
                  : unresolvedDaemon(
                      host,
                      hosts.find((entry) => entry.id === host)?.label,
                    )
            }
          >
            <PathAutocomplete
              id="create-agent-cwd"
              value={draft.cwd}
              onChange={(value) => updateDraft("cwd", value)}
              placeholder="empty = managed workdir"
              aria-label="cwd"
            />
          </ApiProvider>
        </Field>
      </div>
    </div>
  );
}
