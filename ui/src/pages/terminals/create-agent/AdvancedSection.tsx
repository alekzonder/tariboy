import type { ReactNode } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { Field, SwitchRow } from "./formControls";
import { IdentitySection } from "./IdentitySection";
import { PluginsFields } from "./PluginsFields";
import { EnvironmentFields } from "./EnvironmentFields";
import { LoopSection } from "./LoopSection";
import { GoalFields } from "./GoalFields";
import type { CreateAgentForm } from "./useCreateAgentForm";

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section aria-label={title} className="space-y-3">
      <h4 className="text-sm font-medium">{title}</h4>
      {children}
    </section>
  );
}

function VersionField({ form }: { form: CreateAgentForm }) {
  const choice = form.imageChoice;
  const tag = form.draft.image.split(":")[1] ?? "";
  return (
    <Field label="version" id="create-agent-image-version">
      <select
        id="create-agent-image-version"
        aria-label="version"
        value={tag}
        onChange={(event) => form.changeImage(`${choice?.name}:${event.target.value}`)}
        disabled={form.formDisabled || !choice || choice.tags.length < 2}
        className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm"
      >
        {(choice?.tags.length ? choice.tags : [tag]).map((value) => (
          <option key={value} value={value}>
            {value}
          </option>
        ))}
      </select>
    </Field>
  );
}

/** Every create setting that has a working default, grouped. */
export function AdvancedSection({
  form,
  open,
  onOpenChange,
}: {
  form: CreateAgentForm;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { draft, updateDraft, formDisabled, bare, loadingManifest } = form;
  const Chevron = open ? ChevronDown : ChevronRight;
  return (
    <div className="space-y-4">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => onOpenChange(!open)}
        className="flex items-center gap-1 text-sm font-medium"
      >
        <Chevron className="size-4" aria-hidden />
        Advanced
      </button>
      {open && (
        <div className="space-y-5 border-l pl-4">
          <Group title="Identity">
            <IdentitySection draft={draft} updateDraft={updateDraft} formDisabled={formDisabled} />
          </Group>
          <Group title="Image">
            <VersionField form={form} />
            <PluginsFields {...form} />
          </Group>
          <Group title="Folder">
            <SwitchRow
              label="Use a managed folder"
              id="create-agent-managed-folder"
              checked={draft.managedFolder}
              onCheckedChange={(value) => updateDraft("managedFolder", value)}
              disabled={formDisabled}
              help="Tariboy makes an empty folder for the agent."
            />
            <EnvironmentFields
              draft={draft}
              updateDraft={updateDraft}
              formDisabled={formDisabled}
            />
          </Group>
          <Group title="Coding tool">
            <SwitchRow
              label="Headless (no terminal)"
              id="create-agent-interactive"
              checked={bare ? false : !draft.interactive}
              onCheckedChange={(value) => updateDraft("interactive", !value)}
              disabled={formDisabled || loadingManifest || bare}
              help={
                bare
                  ? "A plain terminal always has a terminal."
                  : "The agent runs without a terminal you can type in."
              }
            />
          </Group>
          <Group title="Autopilot">
            <LoopSection draft={draft} updateDraft={updateDraft} formDisabled={formDisabled} />
          </Group>
          <Group title="Tasks">
            <GoalFields draft={draft} updateDraft={updateDraft} formDisabled={formDisabled} />
          </Group>
          <SwitchRow
            label="Create without starting"
            id="create-agent-no-start"
            checked={!draft.startNow}
            onCheckedChange={(value) => updateDraft("startNow", !value)}
            disabled={formDisabled}
            help="The agent stays stopped until you start it."
          />
        </div>
      )}
    </div>
  );
}
