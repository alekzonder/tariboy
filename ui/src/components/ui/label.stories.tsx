import type { ReactNode } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Lock } from "lucide-react";
import { Input } from "./input";
import { Label } from "./label";
import { Switch } from "./switch";
import { Textarea } from "./textarea";

const meta = { component: Label } satisfies Meta<typeof Label>;
export default meta;

type Story = StoryObj;

const help = "text-xs text-muted-foreground";

const Field = ({ id, label, children }: { id: string; label: string; children: ReactNode }) => (
  <div className="grid gap-1.5">
    <Label htmlFor={id}>{label}</Label>
    {children}
  </div>
);

export const FieldLabel: Story = {
  render: () => (
    <div className="grid w-[320px] gap-1.5">
      <Label htmlFor="preview-label-name">name</Label>
      <Input id="preview-label-name" placeholder="empty = generated" />
    </div>
  ),
};

export const FormLabels: Story = {
  render: () => (
    <div className="grid w-[360px] gap-3">
      <Field id="preview-label-image" label="image *">
        <Input id="preview-label-image" defaultValue="worker:v2" />
      </Field>
      <Field id="preview-label-cwd" label="cwd">
        <Input id="preview-label-cwd" placeholder="empty = managed workdir" />
      </Field>
      <Field id="preview-label-notes" label="notes">
        <Textarea id="preview-label-notes" placeholder="one-shot exec prompt (optional)" />
      </Field>
    </div>
  ),
};

export const WithIconAndHelp: Story = {
  render: () => (
    <div className="grid w-[360px] gap-1.5">
      <Label htmlFor="preview-label-token">
        <Lock />
        daemon token
      </Label>
      <Input id="preview-label-token" type="password" defaultValue="tb_live_9ac71e3f" />
      <span className={help}>Stored in the OS keychain, never in the project.</span>
    </div>
  ),
};

export const SwitchRow: Story = {
  render: () => (
    <div className="flex w-[360px] items-center justify-between gap-3">
      <div>
        <Label htmlFor="preview-label-interactive">Interactive</Label>
        <p className={`${help} mt-0.5 mb-0`}>Attach a terminal console.</p>
      </div>
      <Switch id="preview-label-interactive" defaultChecked />
    </div>
  ),
};

export const Disabled: Story = {
  render: () => (
    <div className="group grid w-[320px] gap-1.5" data-disabled="true">
      <Label htmlFor="preview-label-host">Host</Label>
      <Input id="preview-label-host" defaultValue="This daemon (local)" disabled />
      <span className={help}>The host cannot be changed after creation.</span>
    </div>
  ),
};
