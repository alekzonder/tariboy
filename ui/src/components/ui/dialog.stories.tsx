import type { ReactNode } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Badge } from "./badge";
import { Button } from "./button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "./dialog";
import { Input } from "./input";
import { Label } from "./label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./select";
import { Switch } from "./switch";
import { Textarea } from "./textarea";

const meta = { component: Dialog } satisfies Meta<typeof Dialog>;
export default meta;

type Story = StoryObj;

// What sits under the overlay: the terminals workspace the dialogs are raised
// from. Keeps the dimmed/blurred backdrop honest instead of dimming blank white.
const Workspace = () => (
  <div className="flex w-180 gap-4">
    <div className="grid w-45 content-start gap-1.5">
      <div className="text-ui text-muted-foreground">This daemon (local)</div>
      {["builder", "reviewer", "packager", "docs-bot"].map((name) => (
        <div key={name} className="rounded-control px-2 py-1 text-body">
          {name}
        </div>
      ))}
    </div>
    <div className="h-55 flex-1 rounded-card border border-border bg-muted" />
  </div>
);

const Opened = ({ children, ...props }: { children: ReactNode; showCloseButton?: boolean }) => (
  <>
    <Workspace />
    <Dialog defaultOpen>
      <DialogContent {...props}>{children}</DialogContent>
    </Dialog>
  </>
);

const Field = ({ id, label, children }: { id: string; label: string; children: ReactNode }) => (
  <div className="grid gap-1.5">
    <Label htmlFor={id}>{label}</Label>
    {children}
  </div>
);

const NewAgentForm = () => (
  <div className="grid gap-4">
    <div className="grid grid-cols-2 gap-3">
      <Field id="new-agent-host" label="Host">
        <Select defaultValue="local">
          <SelectTrigger id="new-agent-host">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="local">This daemon (local)</SelectItem>
            <SelectItem value="build-01">build-01</SelectItem>
          </SelectContent>
        </Select>
      </Field>
      <Field id="new-agent-image" label="Image">
        <Select defaultValue="worker:v2">
          <SelectTrigger id="new-agent-image">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="worker:v2">worker:v2</SelectItem>
            <SelectItem value="worker:v1">worker:v1</SelectItem>
            <SelectItem value="bare:latest">bare:latest</SelectItem>
          </SelectContent>
        </Select>
      </Field>
    </div>
    <Field id="new-agent-name" label="Name">
      <Input id="new-agent-name" defaultValue="packager" />
    </Field>
    <Field id="new-agent-goal" label="Goal">
      <Textarea
        id="new-agent-goal"
        rows={3}
        defaultValue="Package each merged change for release and keep the changelog current."
      />
    </Field>
    <div className="flex items-center gap-2">
      <Switch id="new-agent-autopilot" defaultChecked />
      <Label htmlFor="new-agent-autopilot">Start Autopilot after create</Label>
    </div>
  </div>
);

export const NewAgent: Story = {
  render: () => (
    <>
      <Workspace />
      <Dialog defaultOpen>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>New agent</DialogTitle>
            <DialogDescription>
              Choose a target and image, then configure identity, runtime, Autopilot, and lifecycle
              settings.
            </DialogDescription>
          </DialogHeader>
          <NewAgentForm />
          <DialogFooter>
            <Button variant="outline">Cancel</Button>
            <Button>Create agent</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  ),
};

const transferHosts = [
  { host: "build-01", note: "ready" },
  { host: "build-02", note: "already has worker:v2" },
];

export const Confirm: Story = {
  render: () => (
    <Opened>
      <DialogHeader>
        <DialogTitle>Transfer worker:v2</DialogTitle>
        <DialogDescription>
          Export the image from This daemon (local) and apply it on the selected hosts.
        </DialogDescription>
      </DialogHeader>
      <div className="grid gap-2">
        {transferHosts.map((row) => (
          <div
            key={row.host}
            className="flex items-center gap-2 rounded-control border border-border px-2 py-1.5 text-body"
          >
            <span>{row.host}</span>
            <span className="ml-auto text-ui text-muted-foreground">{row.note}</span>
          </div>
        ))}
      </div>
      <DialogFooter>
        <Button variant="outline">Cancel</Button>
        <Button>Start transfer</Button>
      </DialogFooter>
    </Opened>
  ),
};

export const FooterCloseAction: Story = {
  render: () => (
    <Opened>
      <DialogHeader>
        <DialogTitle>Host build-01 updated</DialogTitle>
        <DialogDescription>
          The daemon on build-01 is now on 0.58.1 and reconnected. No agents were restarted.
        </DialogDescription>
      </DialogHeader>
      <DialogFooter showCloseButton />
    </Opened>
  ),
};

export const NoCloseButton: Story = {
  render: () => (
    <Opened showCloseButton={false}>
      <DialogHeader>
        <DialogTitle>TB-142</DialogTitle>
        <DialogDescription>
          Autopilot loop stalls when the image manifest is missing a harness.
        </DialogDescription>
      </DialogHeader>
      <div className="flex items-center gap-2">
        <Badge variant="secondary">in review</Badge>
        <span className="text-ui text-muted-foreground">assigned to reviewer · worker:v2</span>
      </div>
      <DialogFooter>
        <Button variant="outline">Reassign</Button>
        <Button>Mark done</Button>
      </DialogFooter>
    </Opened>
  ),
};
