import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { GettingReady } from "@/components/onboarding/GettingReady";
import { Field } from "./formControls";
import { FolderField } from "./FolderField";
import { ImageSelect } from "./ImageSelect";
import { CodingToolField } from "./CodingToolField";
import { AdvancedSection } from "./AdvancedSection";
import { SourceAlert, StartAlerts } from "./CreateAgentAlerts";
import type { CreateAgentForm } from "./useCreateAgentForm";

const LOCAL_HOST_VALUE = "__local__";

function RunOnField({ form }: { form: CreateAgentForm }) {
  return (
    <Field label="Run on" id="create-agent-host">
      <Select
        value={form.host || LOCAL_HOST_VALUE}
        onValueChange={form.changeHost}
        disabled={form.formDisabled}
      >
        <SelectTrigger id="create-agent-host" aria-label="Run on" className="h-8">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {form.hosts.map((entry) => (
            <SelectItem key={entry.id || LOCAL_HOST_VALUE} value={entry.id || LOCAL_HOST_VALUE}>
              {entry.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Field>
  );
}

function ImageField({ form, checklistShown }: { form: CreateAgentForm; checklistShown: boolean }) {
  const { onboarding, imageChoices, loadingImages } = form;
  // Only `bare` is built: say what the daemon is still doing instead of a bare list.
  const onlyBare = imageChoices.every((choice) => choice.name === "bare" || choice.waiting);
  return (
    <div className="space-y-1">
      <div className="flex items-center gap-2">
        <Label htmlFor="create-agent-image">Image</Label>
        {form.bare && <Badge variant="secondary">Terminal only</Badge>}
      </div>
      {onboarding.status && onlyBare && !loadingImages && !checklistShown && (
        <GettingReady status={onboarding.status} retry={onboarding.retry} />
      )}
      <ImageSelect
        id="create-agent-image"
        choices={imageChoices}
        value={form.imageChoice}
        onChange={form.changeImage}
        disabled={form.formDisabled}
      />
      {loadingImages && <p className="text-xs text-muted-foreground">Loading built images…</p>}
    </div>
  );
}

/** The New agent fields: Run on (with several hosts), Folder, Image, Coding
 *  tool and Name, then Advanced. The dialog and the first-run card share it. */
export function NewAgentForm({
  form,
  checklistShown = false,
}: {
  form: CreateAgentForm;
  /** The frame shows the host's checklist already. */
  checklistShown?: boolean;
}) {
  const [advancedOpen, setAdvancedOpen] = useState(form.cloneSource !== undefined);
  return (
    <div className="space-y-4">
      <SourceAlert form={form} />
      {form.hosts.length > 1 && <RunOnField form={form} />}
      <FolderField form={form} />
      <ImageField form={form} checklistShown={checklistShown} />
      {!form.plainTerminal && <CodingToolField form={form} />}
      <Field label="Name" id="create-agent-name">
        <Input
          id="create-agent-name"
          aria-label="name"
          value={form.draft.name}
          onChange={(event) => form.changeName(event.target.value)}
          placeholder="empty = generated"
          disabled={form.formDisabled}
          className="h-8"
        />
      </Field>
      <AdvancedSection form={form} open={advancedOpen} onOpenChange={setAdvancedOpen} />
      <StartAlerts form={form} />
    </div>
  );
}
