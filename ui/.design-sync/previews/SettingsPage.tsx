import type { ReactNode } from "react";
import {
  AdvancedSettingsIndex,
  AppearanceSettings,
  CliSettings,
  DaemonProvider,
  DaemonsPage,
  GeneralSettings,
  MemoryRouter,
  Route,
  Routes,
  SettingsPage,
} from "tariboy-ui";

// SettingsPage is the settings shell: a 208px section rail (General, Hosts, CLI,
// Appearance, Advanced), an Integrations block for any plugin that contributes a
// settings panel, an Operator tools block that unfolds only inside /advanced —
// and an <Outlet/> for the section body.
//
// The Outlet is the whole point, so a preview must give it the CHILD ROUTES from
// App.tsx, not just a router: MemoryRouter alone leaves the right-hand pane
// blank. `basePath` is what every NavLink is built from, so it has to match the
// mounted path — here the canonical per-server one, /servers/local/settings.
// The Outlet also passes `target` down as outlet context (GeneralSettings reads
// it), which is why `target={null}` — the same-origin local daemon.
//
// There is no daemon behind the capture server, so the plugin-contribution fetch
// fails (no Integrations block) and the section bodies show their own load
// failures under complete chrome.
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

const BASE = "/servers/local/settings";

const Settings = ({ path }: { path: string }) => (
  <MemoryRouter initialEntries={[`${BASE}${path}`]}>
    <DaemonProvider>
      <Shell>
        <Routes>
          <Route
            path="/servers/:hostId/settings"
            element={<SettingsPage basePath={BASE} target={null} />}
          >
            <Route index element={<GeneralSettings />} />
            <Route path="hosts" element={<DaemonsPage />} />
            <Route path="cli" element={<CliSettings />} />
            <Route path="appearance" element={<AppearanceSettings />} />
            <Route path="advanced" element={<AdvancedSettingsIndex />} />
          </Route>
        </Routes>
      </Shell>
    </DaemonProvider>
  </MemoryRouter>
);

export const General = () => <Settings path="" />;

export const Hosts = () => <Settings path="/hosts" />;

export const Advanced = () => <Settings path="/advanced" />;
