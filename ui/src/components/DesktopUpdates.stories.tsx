import type { Meta, StoryObj } from "@storybook/react-vite";
import { AppSettings, DesktopUpdatesProvider } from "./DesktopUpdates";

// AppSettings is the /app-settings route: the Desktop updates card plus the way
// back to the workspace. It reads DesktopUpdatesProvider's context, and the
// provider derives `desktop` from __TAURI_INTERNALS__, so in a browser the card
// renders its real non-desktop shape: current version "—", the auto-download
// switch disabled at its default-on position, and the "updates are
// Desktop-only" status line in place of the check/download button.
const meta = { component: AppSettings } satisfies Meta<typeof AppSettings>;
export default meta;

type Story = StoryObj<typeof meta>;

export const BrowserFallback: Story = {
  render: () => (
    <DesktopUpdatesProvider>
      <div className="h-115 w-205 overflow-hidden">
        <AppSettings />
      </div>
    </DesktopUpdatesProvider>
  ),
};
