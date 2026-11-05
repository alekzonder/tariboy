import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { Label } from "./label";
import { Switch } from "./switch";

const meta = { component: Switch } satisfies Meta<typeof Switch>;
export default meta;

type Story = StoryObj;

function Row({
  id,
  label,
  hint,
  children,
}: {
  id: string;
  label: string;
  hint: string;
  children: ReactNode;
}) {
  return (
    <div className="flex items-center justify-between gap-4">
      <div>
        <Label htmlFor={id}>{label}</Label>
        <p className="mt-0.5 mb-0 text-xs text-muted-foreground">{hint}</p>
      </div>
      {children}
    </div>
  );
}

export const SettingsRows: Story = {
  render: () => (
    <div className="grid w-[400px] gap-3.5">
      <Row id="preview-switch-interactive" label="Interactive" hint="Attach a terminal console.">
        <Switch id="preview-switch-interactive" defaultChecked />
      </Row>
      <Row
        id="preview-switch-goal"
        label="Goal"
        hint="Deliver the agent's current Native Task goal."
      >
        <Switch id="preview-switch-goal" />
      </Row>
      <Row
        id="preview-switch-start"
        label="Start now"
        hint="Off creates the agent in stopped state."
      >
        <Switch id="preview-switch-start" defaultChecked />
      </Row>
    </div>
  ),
};

export const CheckedStates: Story = {
  render: () => (
    <div className="flex items-center gap-5">
      <Label htmlFor="preview-switch-on" className="gap-2">
        <Switch id="preview-switch-on" defaultChecked />
        loop enabled
      </Label>
      <Label htmlFor="preview-switch-off" className="gap-2">
        <Switch id="preview-switch-off" />
        loop disabled
      </Label>
    </div>
  ),
};

export const Sizes: Story = {
  render: () => (
    <div className="flex items-center gap-5">
      <Label htmlFor="preview-switch-sm-on" className="gap-2">
        <Switch id="preview-switch-sm-on" size="sm" defaultChecked />
        sm · on
      </Label>
      <Label htmlFor="preview-switch-sm-off" className="gap-2">
        <Switch id="preview-switch-sm-off" size="sm" />
        sm · off
      </Label>
      <Label htmlFor="preview-switch-md-on" className="gap-2">
        <Switch id="preview-switch-md-on" defaultChecked />
        default · on
      </Label>
      <Label htmlFor="preview-switch-md-off" className="gap-2">
        <Switch id="preview-switch-md-off" />
        default · off
      </Label>
    </div>
  ),
};

export const DisabledAndInvalid: Story = {
  render: () => (
    <div className="grid w-[400px] gap-3.5">
      <Row
        id="preview-switch-disabled-on"
        label="Interactive"
        hint="Forced on for Terminal-only images."
      >
        <Switch id="preview-switch-disabled-on" defaultChecked disabled />
      </Row>
      <Row
        id="preview-switch-disabled-off"
        label="Auto update"
        hint="Unavailable while build-01 is disconnected."
      >
        <Switch id="preview-switch-disabled-off" disabled />
      </Row>
      <Row
        id="preview-switch-invalid"
        label="Budget enforcement"
        hint="Needs a budget rule before it can be turned off."
      >
        <Switch id="preview-switch-invalid" aria-invalid defaultChecked />
      </Row>
    </div>
  ),
};
