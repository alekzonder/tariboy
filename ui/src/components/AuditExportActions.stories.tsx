import type { Meta, StoryObj } from "@storybook/react-vite";
import { Input } from "@/components/ui/input";
import { AuditExportActions } from "./AuditExportActions";

// AuditExportActions is the Copy / Export pair at the right end of an audit log
// header. Copy writes the rendered Markdown to the clipboard; Export is gated by
// an AlertDialog warning that the ZIP carries prompts, commands and model
// responses. With `iteration` it scopes to one iteration; without it the whole
// agent audit is exported.
const meta = {
  component: AuditExportActions,
  parameters: { tariboy: { daemon: true } },
} satisfies Meta<typeof AuditExportActions>;
export default meta;

type Story = StoryObj;

const mono = "font-mono text-ui text-muted-foreground";

export const IterationHeader: Story = {
  render: () => (
    <div className="flex w-130 flex-col gap-1">
      <div className="flex items-center justify-between gap-2">
        <div className={mono}>iteration 4f2a91 · ok</div>
        <AuditExportActions name="builder" iteration="4f2a91c7-2b60-4f1e-8f0a-91d2c7e51a33" />
      </div>
      <div className={`rounded-control border border-border bg-muted p-2 ${mono}`}>
        18:22:41 iteration loop · 18:22:49 Bash go test ./... · 18:23:07 iteration ok
      </div>
    </div>
  ),
};

export const FullAuditToolbar: Story = {
  render: () => (
    <div className="flex w-140 items-center gap-2">
      <div className="flex h-8 items-center rounded-control border border-input px-2.5 text-body whitespace-nowrap text-muted-foreground">
        all types
      </div>
      <Input
        aria-label="Search text"
        placeholder="search all fields…"
        defaultValue="updater"
        className="h-8"
      />
      <AuditExportActions name="builder" />
    </div>
  ),
};

export const AgentScoped: Story = {
  render: () => (
    <div className="flex w-85 items-center gap-2.5">
      <span className="text-body">
        Full audit for <span className="font-mono text-ui">docs-bot</span>
      </span>
      <span className="ml-auto">
        <AuditExportActions name="docs-bot" />
      </span>
    </div>
  ),
};
