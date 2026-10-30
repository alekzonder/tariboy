import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useCreateAgentForm } from "./create-agent/useCreateAgentForm";
import { NewAgentForm } from "./create-agent/NewAgentForm";
import { CreateAgentFooter } from "./create-agent/CreateAgentAlerts";

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
  /** `agents` lists the names already used on the host, for the suggested name. */
  hosts: { id: string; label: string; revision?: string; agents?: string[] }[];
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
              : "Pick the folder the agent works in. Everything else has a default."}
          </DialogDescription>
        </DialogHeader>

        <NewAgentForm form={form} />

        <DialogFooter className="items-center">
          <CreateAgentFooter form={form} onCancel={() => onOpenChange(false)} />
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
