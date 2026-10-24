import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useCreateAgentForm } from "./create-agent/useCreateAgentForm";
import { TargetSection } from "./create-agent/TargetSection";
import { IdentitySection } from "./create-agent/IdentitySection";
import { RuntimeSection } from "./create-agent/RuntimeSection";
import { LoopSection } from "./create-agent/LoopSection";
import { LifecycleSection } from "./create-agent/LifecycleSection";
import { CreateAgentFooter, SourceAlert, StartAlerts } from "./create-agent/CreateAgentAlerts";

export interface CloneAgentSource {
  hostId: string;
  agentName: string;
  hostLabel: string;
}

export interface CreateAgentDialogProps {
  open: boolean;
  hostId: string;
  imageRef?: string;
  cloneSource?: CloneAgentSource;
  hosts: { id: string; label: string; revision?: string }[];
  onOpenChange: (open: boolean) => void;
  onCreated: (hostId: string, name: string) => void;
}

export function CreateAgentDialog(props: CreateAgentDialogProps) {
  if (!props.open) return null;
  const sourceKey = props.cloneSource
    ? `${props.cloneSource.hostId}\u0000${props.cloneSource.agentName}`
    : "new";
  return (
    <CreateAgentDialogForm
      key={`${sourceKey}\u0000${props.hostId}\u0000${props.imageRef ?? ""}`}
      {...props}
    />
  );
}

function CreateAgentDialogForm(props: CreateAgentDialogProps) {
  const form = useCreateAgentForm(props);
  const { open, onOpenChange, cloneSource } = props;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{cloneSource ? "Clone agent" : "New agent"}</DialogTitle>
          <DialogDescription>
            {cloneSource
              ? `Copy configuration from ${cloneSource.agentName} on ${cloneSource.hostLabel}. Choose a unique name and review every field.`
              : "Choose a target and image, then configure identity, runtime, loop, and lifecycle settings."}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <SourceAlert form={form} />

          <TargetSection
            host={form.host}
            hosts={form.hosts}
            draft={form.draft}
            formDisabled={form.formDisabled}
            changeHost={form.changeHost}
            changeImage={form.changeImage}
            bare={form.bare}
            images={form.images}
            loadingImages={form.loadingImages}
          />

          <IdentitySection
            draft={form.draft}
            updateDraft={form.updateDraft}
            formDisabled={form.formDisabled}
          />

          <RuntimeSection
            host={form.host}
            hosts={form.hosts}
            draft={form.draft}
            updateDraft={form.updateDraft}
            formDisabled={form.formDisabled}
            bare={form.bare}
            loadingManifest={form.loadingManifest}
            effortPresets={form.effortPresets}
            modelPresets={form.modelPresets}
            target={form.target}
            targetIsCurrent={form.targetIsCurrent}
            displayedPlugins={form.displayedPlugins}
            schemaV2={form.schemaV2}
            pluginDraft={form.pluginDraft}
            setPluginDraft={form.setPluginDraft}
            addPlugin={form.addPlugin}
          />

          <LoopSection
            draft={form.draft}
            updateDraft={form.updateDraft}
            formDisabled={form.formDisabled}
          />

          <LifecycleSection
            draft={form.draft}
            updateDraft={form.updateDraft}
            formDisabled={form.formDisabled}
          />

          <StartAlerts form={form} />
        </div>

        <DialogFooter className="items-center">
          <CreateAgentFooter form={form} />
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
