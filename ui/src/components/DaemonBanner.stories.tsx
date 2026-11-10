import type { Meta, StoryObj } from "@storybook/react-vite";
import { DaemonBanner } from "./DaemonBanner";

// DaemonBanner picks its source of truth from the environment: in the desktop
// app Rust pushes daemon lifecycle transitions, in a browser the only signal is
// whether GET /api/status answers. Storybook is a browser with no daemon, so the
// BrowserBanner branch runs and renders the error bar it really ships. The
// desktop branch needs __TAURI_INTERNALS__ and cannot be reached here.
const meta = {
  component: DaemonBanner,
  parameters: { tariboy: { daemon: true } },
} satisfies Meta<typeof DaemonBanner>;
export default meta;

type Story = StoryObj<typeof meta>;

export const Unreachable: Story = {
  render: () => (
    <div className="w-180">
      <DaemonBanner />
    </div>
  ),
};

// The bar as the app stacks it: directly under the title bar, above the routed
// view. The frame is story scaffolding; the bar is the shipped component.
export const UnderAppChrome: Story = {
  render: () => (
    <div className="w-180 overflow-hidden rounded-card border border-border bg-background">
      <div className="flex h-12 items-center border-b border-border px-4 text-body text-muted-foreground">
        Tariboy
      </div>
      <DaemonBanner />
      <div className="p-6 text-body text-muted-foreground">workspace</div>
    </div>
  ),
};
