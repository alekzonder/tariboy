import type { Meta, StoryObj } from "@storybook/react-vite";
import { Badge } from "./badge";
import { Separator } from "./separator";

const meta = { component: Separator } satisfies Meta<typeof Separator>;
export default meta;

type Story = StoryObj;

const PANEL = "grid w-[400px] gap-2.5 rounded-[10px] border border-border p-3 text-[13px]";
const MUTED = "text-xs text-muted-foreground";

export const SectionDivider: Story = {
  render: () => (
    <div className={PANEL}>
      <div>
        <div className="font-medium">builder</div>
        <div className={MUTED}>worker:v2 · This daemon (local)</div>
      </div>
      <Separator />
      <div>
        <div className="font-medium">Current goal</div>
        <div className={MUTED}>TB-142 — split the packager image cache</div>
      </div>
      <Separator />
      <div>
        <div className="font-medium">Budget</div>
        <div className={MUTED}>$4.10 of $20.00 used in the last 24h</div>
      </div>
    </div>
  ),
};

export const VerticalInlineMeta: Story = {
  render: () => (
    <div className="flex w-fit items-center gap-2.5 rounded-[10px] border border-border px-3 py-2 text-[13px]">
      <span className="font-medium">reviewer</span>
      <Separator orientation="vertical" className="h-4" />
      <span className={MUTED}>worker:v1</span>
      <Separator orientation="vertical" className="h-4" />
      <span className={MUTED}>build-01</span>
      <Separator orientation="vertical" className="h-4" />
      <Badge variant="secondary">stopped</Badge>
    </div>
  ),
};

export const ListGroups: Story = {
  render: () => (
    <div className={`${PANEL} gap-0 overflow-hidden p-0`}>
      <div className={`px-3 py-2 ${MUTED}`}>Hosts</div>
      <Separator />
      <div className="px-3 py-2 font-medium">This daemon (local)</div>
      <div className="px-3 py-2 font-medium">build-01</div>
      <Separator />
      <div className={`px-3 py-2 ${MUTED}`}>Images</div>
      <Separator />
      <div className="px-3 py-2 font-medium">worker:v2</div>
      <div className="px-3 py-2 font-medium">bare:latest</div>
    </div>
  ),
};

export const DialogFooterRule: Story = {
  render: () => (
    <div className={`${PANEL} gap-3`}>
      <div>
        <div className="font-medium">Remove worker:v1</div>
        <div className={MUTED}>Two agents still reference this image and will fail to restart.</div>
      </div>
      <Separator />
      <div className={`flex justify-end gap-2 ${MUTED}`}>
        <span>Esc to cancel</span>
        <Separator orientation="vertical" className="h-3.5" />
        <span>Enter to remove</span>
      </div>
    </div>
  ),
};
