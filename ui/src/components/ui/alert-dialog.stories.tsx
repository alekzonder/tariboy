import type { ReactNode } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Trash2 } from "lucide-react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "./alert-dialog";
import { Badge } from "./badge";
import { Button } from "./button";

const meta = { component: AlertDialog } satisfies Meta<typeof AlertDialog>;
export default meta;

type Story = StoryObj;

// The page behind the overlay: the agent console header row (AgentConsoleTab)
// and the images table row (BuiltImages) are where AlertDialog is actually
// raised from, so the dimmed backdrop reads as a real screen.
const AgentToolbar = ({ children }: { children?: ReactNode }) => (
  <div className="grid w-150 gap-3">
    <div className="flex items-center gap-2">
      <span className="text-body font-medium">builder</span>
      <Badge variant="secondary">running</Badge>
      <span className="text-ui text-muted-foreground">worker:v2 · This daemon (local)</span>
      <span className="ml-auto flex gap-2">{children}</span>
    </div>
    <div className="h-37.5 rounded-card border border-border bg-muted" />
  </div>
);

const ImagesTable = ({ children }: { children?: ReactNode }) => (
  <div className="grid w-150 gap-2">
    <div className="text-ui text-muted-foreground">Built images · This daemon (local)</div>
    {["worker:v2", "worker:v1", "bare:latest"].map((ref, i) => (
      <div
        key={ref}
        className="flex items-center gap-2 rounded-card border border-border px-2.5 py-2 text-body"
      >
        <span className="font-mono">{ref}</span>
        <span className="ml-auto flex gap-2">{i === 0 ? children : null}</span>
      </div>
    ))}
  </div>
);

export const DeleteAgent: Story = {
  render: () => (
    <AgentToolbar>
      <AlertDialog defaultOpen>
        <AlertDialogTrigger asChild>
          <Button size="sm" variant="destructive">
            Delete
          </Button>
        </AlertDialogTrigger>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete agent builder?</AlertDialogTitle>
            <AlertDialogDescription>
              This permanently deletes the agent and all of its durable data. This action cannot be
              undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction variant="destructive">Delete agent</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </AgentToolbar>
  ),
};

export const WithMedia: Story = {
  render: () => (
    <ImagesTable>
      <AlertDialog defaultOpen>
        <AlertDialogTrigger asChild>
          <Button size="sm" variant="destructive" aria-label="Remove worker:v2">
            Remove
          </Button>
        </AlertDialogTrigger>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <Trash2 />
            </AlertDialogMedia>
            <AlertDialogTitle>Remove image worker:v2?</AlertDialogTitle>
            <AlertDialogDescription>
              Deletes this immutable runnable image. Original build files are not managed by
              Tariboy.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction variant="destructive">Remove image</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </ImagesTable>
  ),
};

export const SizeSm: Story = {
  render: () => (
    <AgentToolbar>
      <AlertDialog defaultOpen>
        <AlertDialogTrigger asChild>
          <Button size="sm" variant="outline">
            Stop
          </Button>
        </AlertDialogTrigger>
        <AlertDialogContent size="sm">
          <AlertDialogHeader>
            <AlertDialogTitle>Stop builder?</AlertDialogTitle>
            <AlertDialogDescription>
              The session ends and the autopilot loop pauses. Durable data is kept.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction>Stop agent</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </AgentToolbar>
  ),
};

export const Pending: Story = {
  render: () => (
    <AgentToolbar>
      <AlertDialog defaultOpen>
        <AlertDialogTrigger asChild>
          <Button size="sm" variant="destructive">
            Delete
          </Button>
        </AlertDialogTrigger>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete agent docs-bot?</AlertDialogTitle>
            <AlertDialogDescription>
              This permanently deletes the agent and all of its durable data. This action cannot be
              undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled>Cancel</AlertDialogCancel>
            <AlertDialogAction variant="destructive" disabled>
              Deleting…
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </AgentToolbar>
  ),
};
