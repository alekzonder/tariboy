import type { ReactNode } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Input } from "./input";
import { Label } from "./label";

const meta = { component: Input } satisfies Meta<typeof Input>;
export default meta;

type Story = StoryObj;

const Field = ({ id, label, children }: { id: string; label: string; children: ReactNode }) => (
  <div className="grid gap-1.5">
    <Label htmlFor={id}>{label}</Label>
    {children}
  </div>
);

export const LabelledField: Story = {
  render: () => (
    <div className="grid w-80 gap-1.5">
      <Label htmlFor="preview-agent-name">name</Label>
      <Input id="preview-agent-name" placeholder="empty = generated" />
      <span className="text-ui text-muted-foreground">
        Leave empty and the daemon names the agent.
      </span>
    </div>
  ),
};

export const IdentityForm: Story = {
  render: () => (
    <div className="grid w-115 grid-cols-2 gap-3">
      <Field id="preview-identity-name" label="name">
        <Input id="preview-identity-name" defaultValue="builder" />
      </Field>
      <Field id="preview-identity-alias" label="alias">
        <Input id="preview-identity-alias" defaultValue="bld" />
      </Field>
      <Field id="preview-identity-group" label="group">
        <Input id="preview-identity-group" defaultValue="release-train" />
      </Field>
      <Field id="preview-identity-color" label="color">
        <Input id="preview-identity-color" placeholder="#rrggbb" />
      </Field>
    </div>
  ),
};

export const States: Story = {
  render: () => (
    <div className="grid w-85 gap-3">
      <Field id="preview-state-value" label="image">
        <Input id="preview-state-value" defaultValue="worker:v2" />
      </Field>
      <Field id="preview-state-placeholder" label="cwd">
        <Input id="preview-state-placeholder" placeholder="empty = managed workdir" />
      </Field>
      <Field id="preview-state-disabled" label="host">
        <Input id="preview-state-disabled" defaultValue="This daemon (local)" disabled />
      </Field>
      <Field id="preview-state-invalid" label="limit USD">
        <Input id="preview-state-invalid" defaultValue="-20" aria-invalid />
        <span className="text-ui text-destructive">Budget limit must be a positive amount.</span>
      </Field>
    </div>
  ),
};

export const Types: Story = {
  render: () => (
    <div className="grid w-85 gap-3">
      <Field id="preview-type-search" label="Search agents">
        <Input id="preview-type-search" type="search" placeholder="Search agents…" />
      </Field>
      <Field id="preview-type-number" label="maximum queued messages">
        <Input id="preview-type-number" type="number" min={1} defaultValue={25} />
      </Field>
      <Field id="preview-type-mono" label="resolved image ref">
        <Input
          id="preview-type-mono"
          defaultValue="ghcr.io/tariboy/worker:v2"
          className="font-mono"
          readOnly
        />
      </Field>
    </div>
  ),
};
