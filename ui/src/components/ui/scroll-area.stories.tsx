import type { Meta, StoryObj } from "@storybook/react-vite";
import { Badge } from "./badge";
import { ScrollArea } from "./scroll-area";

const meta = { component: ScrollArea } satisfies Meta<typeof ScrollArea>;
export default meta;

type Story = StoryObj;

const LOG_LINES = [
  "09:47:02 builder   claimed TB-142 from queue tasks",
  "09:47:03 builder   pulling worker:v2 on This daemon (local)",
  "09:47:11 builder   harness=claude effort=high",
  "09:47:40 reviewer  requested changes on TB-142",
  "09:48:02 builder   applied review feedback (3 files)",
  "09:48:31 packager  built bare:latest sha256:9ac71e3f52",
  "09:48:33 packager  pushed bare:latest to build-01",
  "09:49:05 docs-bot  regenerated the CLI reference",
  "09:49:18 builder   exit 0 in 1m 18s · $0.14",
  "09:49:19 builder   iteration 42 complete",
];

const IMAGES = [
  { ref: "worker:v2", built: "2026-09-11 09:47", state: "Active" },
  { ref: "worker:v1", built: "2026-09-04 17:02", state: "Idle" },
  { ref: "reviewer:v3", built: "2026-08-29 11:20", state: "Idle" },
  { ref: "bare:latest", built: "2026-08-21 08:55", state: "Active" },
  { ref: "docs-bot:v7", built: "2026-08-14 13:41", state: "Idle" },
  { ref: "packager:v2", built: "2026-08-02 19:10", state: "Idle" },
];

const MESSAGES = [
  ["builder", "TB-142 is claimed; starting on the retry backoff."],
  ["reviewer", "Please keep the jitter bounded at 30s."],
  ["builder", "Done — capped at 30s and covered by a unit test."],
  ["packager", "bare:latest is built and pushed to build-01."],
  ["docs-bot", "CLI reference regenerated for the new flag."],
  ["reviewer", "Approved. Closing TB-142."],
];

export const PluginLogs: Story = {
  render: () => (
    <ScrollArea className="h-[180px] w-[440px] rounded border bg-muted/30 p-2">
      <pre className="m-0 font-mono text-xs/[18px] whitespace-pre-wrap">{LOG_LINES.join("\n")}</pre>
    </ScrollArea>
  ),
};

export const BuiltImageList: Story = {
  render: () => (
    <ScrollArea className="h-[180px] w-[380px] rounded border">
      <div className="grid">
        {IMAGES.map((image) => (
          <div
            key={image.ref}
            className="flex items-center justify-between gap-3 border-b border-border px-3 py-2.5"
          >
            <div className="grid min-w-0 gap-0.5">
              <span className="font-mono text-[13px]">{image.ref}</span>
              <span className="text-xs text-muted-foreground">Built {image.built}</span>
            </div>
            <Badge variant={image.state === "Active" ? "default" : "secondary"}>
              {image.state}
            </Badge>
          </div>
        ))}
      </div>
    </ScrollArea>
  ),
};

export const ChannelTranscript: Story = {
  render: () => (
    <ScrollArea className="h-[180px] w-[400px] rounded border bg-muted/30 p-2">
      <div className="grid gap-2.5 text-[13px]">
        {MESSAGES.map(([who, text], i) => (
          <div key={i} className="grid gap-0.5">
            <span className="font-mono text-xs text-muted-foreground">{who}</span>
            <span>{text}</span>
          </div>
        ))}
      </div>
    </ScrollArea>
  ),
};
