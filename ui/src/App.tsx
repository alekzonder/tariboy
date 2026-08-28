import { NavLink, Navigate, Route, Routes, useLocation, useOutletContext, useParams } from "react-router-dom";
import { Toaster } from "@/components/ui/sonner";
import { ThemeToggle } from "@/components/ThemeToggle";
import { DaemonBanner } from "@/components/DaemonBanner";
import { DaemonProvider, useDaemons } from "@/components/DaemonProvider";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { PanelLeftClose, PanelLeftOpen } from "lucide-react";
import ImageOverview from "@/pages/ImageOverview";
import ImageTemplate from "@/pages/images/ImageTemplate";
import ImageSkills from "@/pages/images/ImageSkills";
import ImageFiles from "@/pages/ImageFiles";
import TerminalsPage from "@/pages/terminals/TerminalsPage";
import {
  AdvancedSettingsIndex,
  AppearanceSettings,
  CliSettings,
  GeneralSettings,
} from "@/pages/settings/SettingsPage";
import TaskReminderSettings from "@/pages/settings/TaskReminderSettings";
import PluginSettings from "@/pages/settings/PluginSettings";
import UsagePage from "@/pages/UsagePage";
import GroupsPage from "@/pages/GroupsPage";
import BudgetsPage from "@/pages/BudgetsPage";
import RulesPage from "@/pages/RulesPage";
import EvalsPage from "@/pages/EvalsPage";
import JudgeRunsPage from "@/pages/JudgeRunsPage";
import JudgeRunDetailPage from "@/pages/JudgeRunDetailPage";
import ImprovementDetailPage from "@/pages/ImprovementDetailPage";
import PluginsPage from "@/pages/PluginsPage";
import OpsPage from "@/pages/OpsPage";
import ChannelsPage from "@/pages/ChannelsPage";
import DaemonsPage from "@/pages/DaemonsPage";
import { SidebarStateProvider } from "@/pages/terminals/SidebarStateProvider";
import { useSharedSidebarState } from "@/pages/terminals/sidebarStateContext";
import { hostToParam } from "@/lib/terminalsHost";
import { CustomerQuestionNotifications } from "@/components/CustomerQuestionNotifications";
import type { ApiTarget } from "@/lib/api";

export default function App() {
  return (
    <DaemonProvider>
      <CustomerQuestionNotifications>
        <SidebarStateProvider>
          <div className="flex h-screen flex-col">
            <MainApp />
          </div>
        </SidebarStateProvider>
      </CustomerQuestionNotifications>
      <Toaster richColors />
    </DaemonProvider>
  );
}

function MainApp() {
  const location = useLocation();
  const sidebar = useSharedSidebarState();
  const sidebarRoute =
    location.pathname === "/"
    || location.pathname === "/workspace"
    || location.pathname.startsWith("/agents/")
    || location.pathname.startsWith("/servers/");
  return (
    <>
      <header
        className="flex h-12 shrink-0 items-center border-b pr-4"
        data-tauri-drag-region="deep"
        data-testid="app-titlebar"
      >
        <div className="flex min-w-0 items-center">
          <div
            aria-hidden="true"
            className="h-12 w-[72px] shrink-0"
          />
          {sidebarRoute && (
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              className="mr-1 shrink-0"
              aria-label={sidebar.hidden ? "Show agents" : "Hide agents"}
              title={sidebar.hidden ? "Show agents" : "Hide agents"}
              onClick={() => sidebar.setHidden(!sidebar.hidden)}
            >
              {sidebar.hidden
                ? <PanelLeftOpen className="size-4" />
                : <PanelLeftClose className="size-4" />}
            </Button>
          )}
          <nav aria-label="Primary" className="flex gap-1">
            <NavLink
              to="/workspace"
              className={({ isActive }) =>
                cn(
                  "rounded px-3 py-1.5 text-sm hover:bg-accent",
                  isActive && "bg-accent font-medium",
                )
              }
            >
              Workspace
            </NavLink>
          </nav>
        </div>
        <div
          aria-hidden="true"
          className="h-full min-w-4 flex-1"
        />
        <div className="flex items-center gap-2">
          <ThemeToggle />
        </div>
      </header>
      <DaemonBanner />
      <main className="min-h-0 flex-1 overflow-hidden">
        <Routes>
          <Route path="/" element={<TerminalsPage />} />
          <Route path="/workspace" element={<TerminalsPage />} />
          <Route path="/agents/new" element={<CanonicalCreateRedirect />} />
          <Route path="/agents/:hostId/teams/:team" element={<TerminalsPage />} />
          <Route path="/agents/:hostId/:agent/:tab/*" element={<TerminalsPage />} />
          <Route path="/servers/:hostId/tasks" element={<TerminalsPage serverView="tasks" />} />

          <Route path="/servers/:hostId/images" element={<TerminalsPage serverView="images" />} />
          <Route path="/servers/:hostId/images/:name/:tag" element={<TerminalsPage serverView="image-detail" />}>
            <Route index element={<ImageOverview />} />
            <Route path="template" element={<ImageTemplate />} />
            <Route path="skills" element={<ImageSkills />} />
            <Route path="files" element={<ImageFiles />} />
          </Route>

          <Route path="/servers/:hostId/settings" element={<TerminalsPage serverView="settings" />}>
            <Route index element={<GeneralSettings />} />
            <Route path="task-reminders" element={<TaskReminderSettingsRoute />} />
            <Route path="hosts" element={<DaemonsPage />} />
            <Route path="cli" element={<CliSettings />} />
            <Route path="appearance" element={<AppearanceSettings />} />
            <Route path="integrations/:plugin" element={<PluginSettingsRoute />} />
            <Route path="advanced" element={<AdvancedSettingsIndex />} />
            <Route path="advanced/usage" element={<UsagePage />} />
            <Route path="advanced/budgets" element={<BudgetsPage />} />
            <Route path="advanced/rules" element={<RulesPage />} />
            <Route path="advanced/evals" element={<EvalsPage />} />
            <Route path="advanced/judges" element={<JudgeRunsPage />} />
            <Route path="advanced/judges/:id" element={<JudgeRunDetailPage />} />
            <Route path="advanced/improvements/:id" element={<ImprovementDetailPage />} />
            <Route path="advanced/plugins" element={<PluginsPage />} />
            <Route path="advanced/ops" element={<OpsPage />} />
            <Route path="advanced/daemons" element={<DaemonsPage />} />
            <Route path="advanced/groups" element={<GroupsPage />} />
            <Route path="advanced/channels" element={<ChannelsPage />} />
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
          <Route path="/rules" element={<Navigate to="/settings/advanced/rules" replace />} />
          <Route path="/evals" element={<Navigate to="/settings/advanced/evals" replace />} />
          <Route path="/judges/*" element={<LegacyJudgeRedirect />} />
          <Route path="/plugins" element={<Navigate to="/settings/advanced/plugins" replace />} />
          <Route path="/ops" element={<Navigate to="/settings/advanced/ops" replace />} />
          <Route path="/daemons" element={<Navigate to="/settings/advanced/daemons" replace />} />
          <Route path="/groups" element={<Navigate to="/settings/advanced/groups" replace />} />
          <Route path="/channels" element={<Navigate to="/settings/advanced/channels" replace />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
    </>
  );
}

function TaskReminderSettingsRoute() {
  const target = useOutletContext<ApiTarget>();
  return <TaskReminderSettings target={target} />;
}

function PluginSettingsRoute() {
  const target = useOutletContext<ApiTarget>();
  const { plugin = "" } = useParams();
  return <PluginSettings name={plugin} target={target} />;
}

function LegacyTerminalRedirect() {
  const { hostId = "local", agent = "" } = useParams();
  return <Navigate to={`/agents/${hostId}/${encodeURIComponent(agent)}/console`} replace />;
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

function LegacyJudgeRedirect() {
  const { "*": id = "" } = useParams();
  return (
    <Navigate
      to={id ? `/settings/advanced/judges/${encodeURIComponent(id)}` : "/settings/advanced/judges"}
      replace
    />
  );
}

function LegacyAgentRedirect() {
  const { name = "", "*": tail = "" } = useParams();
  const location = useLocation();
  const { activeId } = useDaemons();
  const host = activeId || "local";
  const first = tail.split("/")[0];
  const tab =
    first === "logs" || first === "usage" ? "activity"
    : first === "settings" ? "configuration"
    : first === "" ? "console"
    : "advanced";
  const query = tab === "advanced" && first
    ? `?view=${encodeURIComponent(first)}`
    : location.search;
  return (
    <Navigate
      to={`/agents/${encodeURIComponent(host)}/${encodeURIComponent(name)}/${tab}${query}`}
      replace
    />
  );
}

function LegacyServerSurfaceRedirect({ section }: {
  section: "tasks" | "images" | "settings";
}) {
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
