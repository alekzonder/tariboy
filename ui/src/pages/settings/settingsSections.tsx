import { useCallback, useEffect, useState, type ReactNode } from "react";
import { NavLink, useParams } from "react-router-dom";
import type { PluginContribution } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { cn } from "@/lib/utils";
import { ShellScriptEditor } from "@/components/ShellScriptEditor";
import { SupportBundle } from "@/components/SupportBundle";
import { CodexApprovalCard } from "@/components/CodexApprovalSettings";
import { MaintenanceSettingsCard } from "@/components/MaintenanceSettings";
import UsagePage from "@/pages/UsagePage";
import BudgetsPage from "@/pages/BudgetsPage";
import PluginsPage from "@/pages/PluginsPage";
import OpsPage from "@/pages/OpsPage";
import OperationsPage from "./operations/OperationsPage";
import PluginSettings from "./PluginSettings";

/** One Settings page: its title and its sections. */
function SettingsSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="space-y-6">
      <h1 className="text-xl font-semibold">{title}</h1>
      {children}
    </div>
  );
}

/** Settings → Usage & budgets: the AI usage of the host and its budgets. */
export function UsageSettings() {
  return (
    <SettingsSection title="Usage & budgets">
      <UsagePage />
      <BudgetsPage />
    </SettingsSection>
  );
}

/** Settings → Agent defaults: what every agent on the host starts with. */
export function AgentDefaultsSettings() {
  const api = useApi();
  const target = api.target;
  const load = useCallback(
    async () => (await api.daemon.getGlobalAgentShellScript()).script,
    [api],
  );
  const save = useCallback((script: string) => api.daemon.setGlobalAgentShellScript(script), [api]);
  return (
    <div className="mx-auto max-w-3xl">
      <SettingsSection title="Agent defaults">
        <ShellScriptEditor
          title="Global Agent Shell Script"
          description="Bash commands sourced before every agent iteration on this host."
          load={load}
          save={save}
        />
        <CodexApprovalCard key={target?.id ?? "active"} />
        <MaintenanceSettingsCard key={target?.id ?? "active"} />
      </SettingsSection>
    </div>
  );
}

/** Settings → Maintenance: backup and restore, the operations log, and the
 *  support bundle. */
export function MaintenanceSettings() {
  return (
    <SettingsSection title="Maintenance">
      <OpsPage />
      <OperationsPage />
      <div className="mx-auto max-w-5xl">
        <SupportBundle />
      </div>
    </SettingsSection>
  );
}

/** The host's plugins that bring their own settings page. */
function useSettingsPlugins() {
  const api = useApi();
  const [plugins, setPlugins] = useState<PluginContribution[]>([]);
  useEffect(() => {
    let current = true;
    void api.plugins
      .getPluginContributions()
      .then((result) => {
        if (current) setPlugins(result.plugins.filter((plugin) => plugin.settings));
      })
      .catch(() => {
        if (current) setPlugins([]);
      });
    return () => {
      current = false;
    };
  }, [api]);
  return plugins;
}

/** Settings → Integrations: the installed plugins, and a tab per plugin
 *  with settings such as Telegram. */
export function IntegrationsSettings() {
  const { plugin } = useParams();
  const plugins = useSettingsPlugins();
  const tab = ({ isActive }: { isActive: boolean }) =>
    cn(
      "flex h-8.5 shrink-0 items-center border-b-2 border-transparent px-2.5 text-body whitespace-nowrap text-muted-foreground hover:text-foreground",
      isActive && "border-primary font-medium text-foreground",
    );
  return (
    <SettingsSection title="Integrations">
      <nav aria-label="Integrations" className="flex flex-nowrap gap-0.5 overflow-x-auto border-b">
        <NavLink to="/settings/integrations" end className={tab}>
          Installed plugins
        </NavLink>
        {plugins.map((entry) => (
          <NavLink
            key={entry.name}
            to={`/settings/integrations/${encodeURIComponent(entry.name)}`}
            className={tab}
          >
            {entry.settings?.title ?? entry.name}
          </NavLink>
        ))}
      </nav>
      {plugin ? <PluginSettings key={plugin} name={plugin} /> : <PluginsPage />}
    </SettingsSection>
  );
}
