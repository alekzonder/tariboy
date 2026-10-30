import { lazy } from "react";
import { Navigate, Route, Routes, useLocation, useParams } from "react-router-dom";
import { useDaemons } from "@/components/daemonContext";
import TerminalsPage from "@/pages/terminals/TerminalsPage";
import { DEFAULT_AGENT_TAB } from "@/pages/agents/agentWorkspaceModel";
import { AppSettings } from "@/components/DesktopUpdates";

// Server sub-pages load on first open, inside the Suspense boundary of the
// layout that renders their Outlet.
const ImageOverview = lazy(() => import("@/pages/ImageOverview"));
const ImageTemplate = lazy(() => import("@/pages/images/ImageTemplate"));
const ImageSkills = lazy(() => import("@/pages/images/ImageSkills"));
const ImageFiles = lazy(() => import("@/pages/ImageFiles"));
const settingsSection = (name: keyof typeof import("@/pages/settings/settingsSections")) =>
  lazy(() => import("@/pages/settings/settingsSections").then((m) => ({ default: m[name] })));
const UsageSettings = settingsSection("UsageSettings");
const AgentDefaultsSettings = settingsSection("AgentDefaultsSettings");
const IntegrationsSettings = settingsSection("IntegrationsSettings");
const MaintenanceSettings = settingsSection("MaintenanceSettings");
const OperationPage = lazy(() => import("@/pages/settings/operations/OperationPage"));
const GroupsPage = lazy(() => import("@/pages/GroupsPage"));
const DaemonsPage = lazy(() => import("@/pages/DaemonsPage"));

export function AppRoutes() {
  return (
    <Routes>
      <Route path="/" element={<TerminalsPage />} />
      <Route path="/tasks" element={<TerminalsPage serverView="tasks" scoped />} />
      <Route path="/inbox" element={<TerminalsPage serverView="mail" scoped />} />
      <Route path="/activity" element={<TerminalsPage serverView="activity" scoped />} />
      <Route path="/app-settings" element={<AppSettings />} />
      <Route path="/agents/new" element={<CanonicalCreateRedirect />} />
      <Route path="/agents/:hostId/teams/:team" element={<TerminalsPage />} />
      <Route path="/agents/:hostId/:agent" element={<LegacyTerminalRedirect />} />
      <Route path="/agents/:hostId/:agent/:tab/*" element={<TerminalsPage />} />
      {/* TerminalsPage sends these to /inbox, /tasks and /activity. */}
      <Route path="/servers/:hostId/mail" element={<TerminalsPage />} />
      <Route path="/servers/:hostId/tasks" element={<TerminalsPage />} />
      <Route path="/servers/:hostId/activity" element={<TerminalsPage />} />

      <Route path="/library" element={<Navigate to="/library/images" replace />} />
      <Route path="/library/images" element={<TerminalsPage serverView="images" scoped />} />
      <Route
        path="/library/images/:name"
        element={<TerminalsPage serverView="image-detail" scoped />}
      >
        <Route index element={<ImageOverview />} />
      </Route>
      <Route
        path="/library/images/:name/:tag"
        element={<TerminalsPage serverView="image-detail" scoped />}
      >
        <Route index element={<ImageOverview />} />
        <Route path="template" element={<ImageTemplate />} />
        <Route path="skills" element={<ImageSkills />} />
        <Route path="files" element={<ImageFiles />} />
      </Route>
      <Route path="/library/workflows" element={<TerminalsPage serverView="workflows" scoped />} />
      <Route
        path="/library/workflows/:name/:tag"
        element={<TerminalsPage serverView="workflow-detail" scoped />}
      />
      <Route path="/library/sources" element={<TerminalsPage serverView="stores" scoped />} />
      <Route
        path="/library/sources/:name"
        element={<TerminalsPage serverView="store-detail" scoped />}
      />
      {/* TerminalsPage sends these to /library/images, /workflows and /sources. */}
      <Route path="/servers/:hostId/images/*" element={<TerminalsPage />} />
      <Route path="/servers/:hostId/workflows/*" element={<TerminalsPage />} />
      <Route path="/servers/:hostId/stores/*" element={<TerminalsPage />} />

      <Route path="/settings" element={<TerminalsPage serverView="settings" scoped />}>
        <Route path="hosts" element={<DaemonsPage />} />
        <Route path="teams" element={<GroupsPage />} />
        <Route path="usage" element={<UsageSettings />} />
        <Route path="agent-defaults" element={<AgentDefaultsSettings />} />
        <Route path="integrations/:plugin?" element={<IntegrationsSettings />} />
        <Route path="maintenance" element={<MaintenanceSettings />} />
        <Route path="maintenance/operations/:id" element={<OperationPage />} />
      </Route>
      {/* TerminalsPage sends these, and an unknown Settings page, to the
          /settings page that holds them now. */}
      <Route path="/settings/*" element={<TerminalsPage />} />
      <Route path="/servers/:hostId/settings/*" element={<TerminalsPage />} />

      <Route path="/images/*" element={<LegacyImagesRedirect />} />

      <Route path="/terminals" element={<CanonicalTerminalsRedirect />} />
      <Route path="/terminals/:hostId/:agent" element={<LegacyTerminalRedirect />} />
      <Route path="/agent/:name/*" element={<LegacyAgentRedirect />} />
      <Route path="/hosts" element={<Navigate to="/settings/hosts" replace />} />
      <Route path="/usage" element={<Navigate to="/settings/usage" replace />} />
      <Route path="/budgets" element={<Navigate to="/settings/usage" replace />} />
      <Route path="/plugins" element={<Navigate to="/settings/integrations" replace />} />
      <Route path="/ops" element={<Navigate to="/settings/maintenance" replace />} />
      <Route path="/daemons" element={<Navigate to="/settings/hosts" replace />} />
      <Route path="/groups" element={<Navigate to="/settings/teams" replace />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

function LegacyTerminalRedirect() {
  const { hostId = "local", agent = "" } = useParams();
  return (
    <Navigate to={`/agents/${hostId}/${encodeURIComponent(agent)}/${DEFAULT_AGENT_TAB}`} replace />
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
        ? "settings"
        : first === ""
          ? DEFAULT_AGENT_TAB
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

function LegacyImagesRedirect() {
  const location = useLocation();
  return <Navigate to={`/library${location.pathname}${location.search}`} replace />;
}
