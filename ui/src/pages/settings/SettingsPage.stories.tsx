import { Route, Routes } from "react-router-dom";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Decorator } from "@storybook/react-vite";
import DaemonsPage from "@/pages/DaemonsPage";
import { withWidth } from "@/storybook/decorators";
import SettingsPage from "./SettingsPage";
import { AgentDefaultsSettings, MaintenanceSettings } from "./settingsSections";

// The settings shell: a rail of pages and an <Outlet/> for the page body. The
// Outlet needs the child routes of routes.tsx, so the story renders them under
// a catch-all route; the layout is mounted at /settings like in the app. There
// is no daemon behind Storybook, so the bodies show their own load failures.
const withFrame: Decorator = (Story) => (
  <div className="h-[620px] overflow-hidden rounded-md border border-border bg-background">
    <Story />
  </div>
);

const meta = {
  component: SettingsPage,
  decorators: [withFrame, withWidth(1024)],
  parameters: { layout: "fullscreen", tariboy: { daemon: true } },
} satisfies Meta<typeof SettingsPage>;
export default meta;

type Story = StoryObj;

const Settings = () => (
  <Routes>
    <Route path="/settings" element={<SettingsPage />}>
      <Route path="hosts" element={<DaemonsPage />} />
      <Route path="agent-defaults" element={<AgentDefaultsSettings />} />
      <Route path="maintenance" element={<MaintenanceSettings />} />
    </Route>
  </Routes>
);

const onPage = (page: string): Story => ({
  parameters: { tariboy: { daemon: true, route: { path: `/settings/${page}`, pattern: "*" } } },
  render: () => <Settings />,
});

export const Hosts: Story = onPage("hosts");

export const AgentDefaults: Story = onPage("agent-defaults");

export const Maintenance: Story = onPage("maintenance");
