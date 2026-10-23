import { SwitchRow } from "./formControls";
import { PluginsFields } from "./PluginsFields";
import { EnvironmentFields } from "./EnvironmentFields";
import { RuntimeFields } from "./RuntimeFields";
import type { CreateAgentForm } from "./useCreateAgentForm";

export function RuntimeSection({
  host,
  hosts,
  draft,
  updateDraft,
  formDisabled,
  bare,
  loadingManifest,
  effortPresets,
  modelPresets,
  target,
  targetIsCurrent,
  displayedPlugins,
  schemaV2,
  pluginDraft,
  setPluginDraft,
  addPlugin,
}: Pick<
  CreateAgentForm,
  | "host"
  | "hosts"
  | "draft"
  | "updateDraft"
  | "formDisabled"
  | "bare"
  | "loadingManifest"
  | "effortPresets"
  | "modelPresets"
  | "target"
  | "targetIsCurrent"
  | "displayedPlugins"
  | "schemaV2"
  | "pluginDraft"
  | "setPluginDraft"
  | "addPlugin"
>) {
  return (
    <section
      aria-labelledby="create-agent-runtime"
      className="space-y-3 rounded border p-3"
    >
      <div>
        <h3 id="create-agent-runtime" className="font-medium">
          Runtime
        </h3>
        <p className="text-xs text-muted-foreground">
          Runtime settings stay editable independently of image content.
        </p>
      </div>
      <RuntimeFields
        host={host}
        hosts={hosts}
        draft={draft}
        updateDraft={updateDraft}
        formDisabled={formDisabled}
        loadingManifest={loadingManifest}
        effortPresets={effortPresets}
        modelPresets={modelPresets}
        target={target}
        targetIsCurrent={targetIsCurrent}
      />
      <SwitchRow
        label="Create directory if missing"
        id="create-agent-create-cwd"
        checked={draft.createCwd}
        onCheckedChange={(value) => updateDraft("createCwd", value)}
        disabled={formDisabled || !draft.cwd.trim()}
        help="Make the cwd directory and its parents when it does not exist."
      />
      <SwitchRow
        label="Interactive"
        id="create-agent-interactive"
        checked={bare ? true : draft.interactive}
        onCheckedChange={(value) => updateDraft("interactive", value)}
        disabled={formDisabled || loadingManifest || bare}
        help={
          bare
            ? "A terminal-only image does not contain agent instructions or tools."
            : "Attach a terminal console."
        }
      />
      <EnvironmentFields
        draft={draft}
        updateDraft={updateDraft}
        formDisabled={formDisabled}
      />
      <PluginsFields
        draft={draft}
        updateDraft={updateDraft}
        formDisabled={formDisabled}
        displayedPlugins={displayedPlugins}
        schemaV2={schemaV2}
        pluginDraft={pluginDraft}
        setPluginDraft={setPluginDraft}
        addPlugin={addPlugin}
      />
    </section>
  );
}
