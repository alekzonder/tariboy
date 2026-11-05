import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { Button } from "./button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "./sheet";

const meta = { component: Sheet } satisfies Meta<typeof Sheet>;
export default meta;

type Story = StoryObj;

// The agent detail screen the sheet slides over.
function AgentScreen({ children }: { children?: ReactNode }) {
  return (
    <div className="grid w-[700px] gap-3">
      <div className="flex items-center gap-2">
        <span className="text-sm font-medium">reviewer</span>
        <span className="text-xs text-muted-foreground">worker:v2 · This daemon (local)</span>
        <span className="ml-auto">{children}</span>
      </div>
      <div className="h-[220px] rounded-lg border border-border bg-muted" />
    </div>
  );
}

const EVENTS = [
  { ts: "2024-05-15 11:58:04", message: "Picked up TB-142 from the queue." },
  { ts: "2024-05-15 11:59:41", message: "Reviewed 3 files, left 2 comments." },
  { ts: "2024-05-15 12:04:12", message: "Waiting on builder to push a fix." },
  { ts: "2024-05-15 12:11:30", message: "Re-reviewed TB-142 — approving." },
];

const HOSTS = [
  { label: "This daemon (local)", note: "4 agents" },
  { label: "build-01", note: "2 agents" },
  { label: "build-02", note: "offline" },
];

function StatusList() {
  return (
    <div className="overflow-hidden px-4">
      <h3 className="border-b border-border py-2 text-xs font-semibold text-muted-foreground">
        Iteration: 2024-05-15T11-57
      </h3>
      {EVENTS.map((event) => (
        <div key={event.ts} className="border-b border-border py-2 text-[13px]">
          <div className="text-xs text-muted-foreground">{event.ts}</div>
          <div>{event.message}</div>
        </div>
      ))}
    </div>
  );
}

function ActionFooter({ cancel, confirm }: { cancel: string; confirm: string }) {
  return (
    <SheetFooter>
      <div className="flex justify-end gap-2">
        <Button variant="outline">{cancel}</Button>
        <Button>{confirm}</Button>
      </div>
    </SheetFooter>
  );
}

export const StatusHistory: Story = {
  render: () => (
    <AgentScreen>
      <Sheet defaultOpen>
        <SheetTrigger asChild>
          <Button size="sm" variant="ghost" aria-label="status history">
            history
          </Button>
        </SheetTrigger>
        <SheetContent>
          <SheetHeader>
            <SheetTitle>Status history — reviewer</SheetTitle>
            <SheetDescription>Status updates, newest first, grouped by iteration.</SheetDescription>
          </SheetHeader>
          <StatusList />
          <SheetFooter>
            <Button variant="outline">Export as JSON</Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>
    </AgentScreen>
  ),
};

export const SideLeft: Story = {
  render: () => (
    <AgentScreen>
      <Sheet defaultOpen>
        <SheetTrigger asChild>
          <Button size="sm" variant="outline">
            Hosts
          </Button>
        </SheetTrigger>
        <SheetContent side="left">
          <SheetHeader>
            <SheetTitle>Hosts</SheetTitle>
            <SheetDescription>Daemons this workspace can reach.</SheetDescription>
          </SheetHeader>
          <div className="grid gap-1.5 px-4">
            {HOSTS.map((host) => (
              <div
                key={host.label}
                className="flex items-center gap-2 rounded-md border border-border px-2 py-1.5 text-[13px]"
              >
                <span>{host.label}</span>
                <span className="ml-auto text-xs text-muted-foreground">{host.note}</span>
              </div>
            ))}
          </div>
        </SheetContent>
      </Sheet>
    </AgentScreen>
  ),
};

export const SideTop: Story = {
  render: () => (
    <AgentScreen>
      <Sheet defaultOpen>
        <SheetTrigger asChild>
          <Button size="sm" variant="outline">
            Release notes
          </Button>
        </SheetTrigger>
        <SheetContent side="top">
          <SheetHeader>
            <SheetTitle>Tariboy 0.58.1 is installed</SheetTitle>
            <SheetDescription>
              Every host on this workspace is now on 0.58.1. Agents kept their sessions.
            </SheetDescription>
          </SheetHeader>
          <ActionFooter cancel="Dismiss" confirm="Read the changelog" />
        </SheetContent>
      </Sheet>
    </AgentScreen>
  ),
};

export const SideBottom: Story = {
  render: () => (
    <AgentScreen>
      <Sheet defaultOpen>
        <SheetTrigger asChild>
          <Button size="sm" variant="outline">
            Send files
          </Button>
        </SheetTrigger>
        <SheetContent side="bottom">
          <SheetHeader>
            <SheetTitle>Send files to packager</SheetTitle>
            <SheetDescription>Files land in /work/inbox on This daemon (local).</SheetDescription>
          </SheetHeader>
          <div className="grid gap-1.5 px-4">
            {["release-notes.md", "worker-manifest.json"].map((name) => (
              <div
                key={name}
                className="rounded-md border border-border px-2 py-1.5 font-mono text-[13px]"
              >
                {name}
              </div>
            ))}
          </div>
          <ActionFooter cancel="Cancel" confirm="Upload 2 files" />
        </SheetContent>
      </Sheet>
    </AgentScreen>
  ),
};
