import { lazy } from "react";
import { Navigate, Route, Routes, useLocation, useParams } from "react-router-dom";
import { useDaemons } from "@/components/daemonContext";
import TerminalsPage from "@/pages/terminals/TerminalsPage";
import { hostToParam } from "@/lib/terminalsHost";
import { defaultAgentTab } from "@/lib/uiMode";
import { AppSettings } from "@/components/DesktopUpdates";

// Server sub-pages load on first open, inside the Suspense boundary of the
// layout that renders their Outlet.
const ImageOverview = lazy(() => import("@/pages/ImageOverview"));
const ImageTemplate = lazy(() => import("@/pages/images/ImageTemplate"));
const ImageSkills = lazy(() => import("@/pages/images/ImageSkills"));
const ImageFiles = lazy(() => import("@/pages/ImageFiles"));
const GeneralSettings = lazy(() =>
  import("@/pages/settings/SettingsPage").then((m) => ({ default: m.GeneralSettings })),
);
const CliSettings = lazy(() =>
  import("@/pages/settings/SettingsPage").then((m) => ({ default: m.CliSettings })),
);
const AppearanceSettings = lazy(() =>
  import("@/pages/settings/SettingsPage").then((m) => ({ default: m.AppearanceSettings })),
);
const AdvancedSettingsIndex = lazy(() =>
  import("@/pages/settings/SettingsPage").then((m) => ({ default: m.AdvancedSettingsIndex })),
);
const PluginSettings = lazy(() => import("@/pages/settings/PluginSettings"));
const UsagePage = lazy(() => import("@/pages/UsagePage"));
const GroupsPage = lazy(() => import("@/pages/GroupsPage"));
const BudgetsPage = lazy(() => import("@/pages/BudgetsPage"));
const PluginsPage = lazy(() => import("@/pages/PluginsPage"));
const OpsPage = lazy(() => import("@/pages/OpsPage"));
const DaemonsPage = lazy(() => import("@/pages/DaemonsPage"));

export function AppRoutes() {
  return (
    <Routes>
      <Route path="/" element={<TerminalsPage />} />
      <Route path="/app-settings" element={<AppSettings />} />
      <Route path="/agents/new" element={<CanonicalCreateRedirect />} />
      <Route path="/agents/:hostId/teams/:team" element={<TerminalsPage />} />
      <Route path="/agents/:hostId/:agent/:tab/*" element={<TerminalsPage />} />
      <Route path="/servers/:hostId/tasks" element={<TerminalsPage serverView="tasks" />} />
      <Route path="/servers/:hostId/activity" element={<TerminalsPage serverView="activity" />} />

      <Route path="/servers/:hostId/images" element={<TerminalsPage serverView="images" />} />
      <Route
        path="/servers/:hostId/images/:name"
        element={<TerminalsPage serverView="image-tags" />}
      />
      <Route
        path="/servers/:hostId/images/:name/:tag"
        element={<TerminalsPage serverView="image-detail" />}
      >
        <Route index element={<ImageOverview />} />
        <Route path="template" element={<ImageTemplate />} />
        <Route path="skills" element={<ImageSkills />} />
        <Route path="files" element={<ImageFiles />} />
      </Route>

      <Route path="/servers/:hostId/workflows" element={<TerminalsPage serverView="workflows" />} />
      <Route
        path="/servers/:hostId/workflows/:name/:tag"
        element={<TerminalsPage serverView="workflow-detail" />}
      />

      <Route path="/servers/:hostId/stores" element={<TerminalsPage serverView="stores" />} />
      <Route
        path="/servers/:hostId/stores/:name"
        element={<TerminalsPage serverView="store-detail" />}
      />

      <Route path="/servers/:hostId/settings" element={<TerminalsPage serverView="settings" />}>
        <Route index element={<GeneralSettings />} />
        <Route path="hosts" element={<DaemonsPage />} />
        <Route path="cli" element={<CliSettings />} />
        <Route path="appearance" element={<AppearanceSettings />} />
        <Route path="integrations/:plugin" element={<PluginSettingsRoute />} />
        <Route path="advanced" element={<AdvancedSettingsIndex />} />
        <Route path="advanced/usage" element={<UsagePage />} />
        <Route path="advanced/budgets" element={<BudgetsPage />} />
        <Route path="advanced/plugins" element={<PluginsPage />} />
        <Route path="advanced/ops" element={<OpsPage />} />
        <Route path="advanced/daemons" element={<DaemonsPage />} />
        <Route path="advanced/groups" element={<GroupsPage />} />
      </Route>

      <Route path="/tasks" element={<LegacyServerSurfaceRedirect section="tasks" />} />
      <Route path="/images/*" element={<LegacyServerSurfaceRedirect section="images" />} />
      <Route path="/settings/*" element={<LegacyServerSurfaceRedirect section="settings" />} />

      <Route path="/terminals" element={<CanonicalTerminalsRedirect />} />
      <Route path="/terminals/:hostId/:agent" element={<LegacyTerminalRedirect />} />
      <Route path="/agent/:name/*" element={<LegacyAgentRedirect />} />
      <Route path="/hosts" element={<Navigate to="/settings/hosts" replace />} />
      <Route path="/usage" element={<Navigate to="/settings/advanced/usage" replace />} />
      <Route path="/budgets" element={<Navigate to="/settings/advanced/budgets" replace />} />
      <Route path="/plugins" element={<Navigate to="/settings/advanced/plugins" replace />} />
      <Route path="/ops" element={<Navigate to="/settings/advanced/ops" replace />} />
      <Route path="/daemons" element={<Navigate to="/settings/advanced/daemons" replace />} />
      <Route path="/groups" element={<Navigate to="/settings/advanced/groups" replace />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

function PluginSettingsRoute() {
  const { plugin = "" } = useParams();
  return <PluginSettings name={plugin} />;
}

function LegacyTerminalRedirect() {
  const { hostId = "local", agent = "" } = useParams();
  return (
    <Navigate to={`/agents/${hostId}/${encodeURIComponent(agent)}/${defaultAgentTab()}`} replace />
  );
}

function CanonicalCreateRedirect() {
  const location = useLocation();
  const params = new URLSearchParams(location.search);
  params.set("new", "1");
  return <Navigate to={`/?${params.toString()}`} replace />;
}

function CanonicalTerminalsRedirect() {
  const location = useLocation();
  return <Navigate to={`/${location.search}`} replace />;
}

function LegacyAgentRedirect() {
  const { name = "", "*": tail = "" } = useParams();
  const location = useLocation();
  const { activeId } = useDaemons();
  const host = activeId || "local";
  const first = tail.split("/")[0];
  const tab =
    first === "logs" || first === "usage"
      ? "activity"
      : first === "settings"
        ? "configuration"
        : first === ""
          ? defaultAgentTab()
          : "advanced";
  const query =
    tab === "advanced" && first ? `?view=${encodeURIComponent(first)}` : location.search;
  return (
    <Navigate
      to={`/agents/${encodeURIComponent(host)}/${encodeURIComponent(name)}/${tab}${query}`}
      replace
    />
  );
}

function LegacyServerSurfaceRedirect({ section }: { section: "tasks" | "images" | "settings" }) {
  const location = useLocation();
  const { activeId } = useDaemons();
  const legacyBase = `/${section}`;
  const tail = location.pathname.slice(legacyBase.length);
  return (
    <Navigate
      to={`/servers/${encodeURIComponent(hostToParam(activeId))}/${section}${tail}${location.search}`}
      replace
    />
  );
}
