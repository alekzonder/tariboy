import type { ReactNode } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { withSurface } from "@/storybook/decorators";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { SendFilesButton } from "./SendFilesButton";
import { TuiScreen } from "./TuiScreen";

// Uploads address the surrounding ApiProvider's host and land in that host's
// shared files directory.

interface TermLike {
  write: (data: string) => void;
}

const SESSION = [
  "builder@build-01:/work/tariboy$ git status --short",
  " M ui/src/components/SendFilesButton.tsx",
  "builder@build-01:/work/tariboy$ ",
].join("\r\n");

// The websocket lives outside TuiScreen; this controller writes a captured
// session into the terminal instead (the same fake as in the TuiScreen stories).
const controller = {
  status: "open" as const,
  session: "present" as const,
  absent: false,
  name: "builder",
  send: () => {},
  sendResize: () => {},
  attachTerm: (term: TermLike | null) => term?.write(SESSION),
  reconnect: () => {},
};

const Grid = ({ children, className }: { children: ReactNode; className?: string }) => (
  <div className={`grid gap-2.5 ${className ?? ""}`}>{children}</div>
);

const meta = {
  component: SendFilesButton,
  decorators: [withSurface("panel")],
  parameters: { tariboy: { daemon: true } },
  args: { onUploaded: () => {} },
} satisfies Meta<typeof SendFilesButton>;
export default meta;

type Story = StoryObj<typeof meta>;

export const TerminalToolbar: Story = {
  decorators: [withSurface("panel", 620)],
  render: (args) => (
    <Grid>
      <div className="h-[170px]">
        <TuiScreen controller={controller} fill surface="workspace" persistDraft={false} />
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <SendFilesButton {...args} />
        <Button size="sm" variant="outline">
          Paste
        </Button>
        <span className="ml-auto text-xs text-muted-foreground">
          uploads land in /work/files on build-01
        </span>
      </div>
    </Grid>
  ),
};

export const NoLiveSession: Story = {
  decorators: [withSurface("panel", 520)],
  render: (args) => (
    <div className="grid gap-1.5 rounded-[10px] border border-border bg-card px-3.5 py-4">
      <div className="text-sm">Session not running or not interactive</div>
      <div className="text-xs text-muted-foreground">
        Start the agent and enable interactive mode (Settings → Runtime config).
      </div>
      <div className="mt-2 flex items-center gap-2">
        <Button size="sm">Start</Button>
        <SendFilesButton {...args} />
      </div>
    </div>
  ),
};

export const InTaskComment: Story = {
  decorators: [withSurface("panel", 560)],
  render: (args) => (
    <Grid>
      <div className="flex items-center justify-between gap-2">
        <Label htmlFor="task-comment">Comment on TB-142</Label>
        <SendFilesButton {...args} />
      </div>
      <Textarea
        id="task-comment"
        placeholder="Comment"
        defaultValue={"Repro attached.\n/work/files/TB-142-trace.log"}
        className="h-[92px]"
      />
    </Grid>
  ),
};

export const Disabled: Story = {
  decorators: [withSurface("panel", 560)],
  render: (args) => (
    <Grid>
      <div className="flex items-center justify-between gap-2">
        <Label htmlFor="task-comment-busy">Comment on TB-142</Label>
        <SendFilesButton {...args} disabled onUploadingChange={() => {}} />
      </div>
      <Textarea
        id="task-comment-busy"
        defaultValue={"Repro attached.\n/work/files/TB-142-trace.log"}
        disabled
        className="h-[92px]"
      />
      <span className="text-xs text-muted-foreground">
        Saving the comment — the button is disabled, the same visual it shows while an upload of its
        own is in flight.
      </span>
    </Grid>
  ),
};
