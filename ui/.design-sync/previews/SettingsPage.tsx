import type { ReactNode } from "react";
import {
  AgentDefaultsSettings,
  DaemonProvider,
  DaemonsPage,
  MaintenanceSettings,
  MemoryRouter,
  Route,
  Routes,
  SettingsPage,
} from "tariboy-ui";

// SettingsPage is the settings shell: a 208px rail with one flat list of pages
// (Hosts, Teams, Usage & budgets, Agent defaults, Integrations, Maintenance,
// and Application as a link) and an <Outlet/> for the page body.
//
// The Outlet is the whole point, so a preview must give it the CHILD ROUTES from
// routes.tsx, not just a router: MemoryRouter alone leaves the right-hand pane
// blank. Every NavLink is an absolute /settings/<page> path, so the layout is
// mounted at /settings like in the app.
//
// There is no daemon behind the capture server, so the page bodies show their
// own load failures under complete chrome.
const Shell = ({ children }: { children: ReactNode }) => (
  <div
    style={{
      height: 620,
      width: 1180,
      overflow: "hidden",
      border: "1px solid var(--border)",
      borderRadius: "var(--radius)",
      background: "var(--background)",
    }}
  >
    {children}
  </div>
);

const Settings = ({ path }: { path: string }) => (
  <MemoryRouter initialEntries={[`/settings/${path}`]}>
    <DaemonProvider>
      <Shell>
        <Routes>
          <Route path="/settings" element={<SettingsPage />}>
            <Route path="hosts" element={<DaemonsPage />} />
            <Route path="agent-defaults" element={<AgentDefaultsSettings />} />
            <Route path="maintenance" element={<MaintenanceSettings />} />
          </Route>
        </Routes>
      </Shell>
    </DaemonProvider>
  </MemoryRouter>
);

export const Hosts = () => <Settings path="hosts" />;

export const AgentDefaults = () => <Settings path="agent-defaults" />;

export const Maintenance = () => <Settings path="maintenance" />;
