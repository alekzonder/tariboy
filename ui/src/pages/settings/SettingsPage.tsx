import { useCallback, useEffect, useState } from "react";
import { NavLink, Outlet, useLocation } from "react-router-dom";
import type { PluginContribution } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { cn } from "@/lib/utils";
import { LazyBoundary } from "@/components/LazyBoundary";
import { useActiveLinkIntoView } from "@/hooks/useActiveLinkIntoView";
import { ShellScriptEditor } from "@/components/ShellScriptEditor";
import { SupportBundle } from "@/components/SupportBundle";
import { CodexApprovalCard } from "@/components/CodexApprovalSettings";
import { MaintenanceSettingsCard } from "@/components/MaintenanceSettings";

const SECTIONS = [
  { suffix: "", label: "General", end: true },
  { suffix: "/hosts", label: "Hosts" },
  { suffix: "/cli", label: "CLI" },
  { suffix: "/appearance", label: "Appearance" },
  { suffix: "/advanced", label: "Advanced" },
];

const ADVANCED = [
  ["usage", "Usage"],
  ["budgets", "Budgets"],
  ["plugins", "Plugins"],
  ["ops", "Ops"],
  ["daemons", "Daemons"],
  ["groups", "Groups"],
  ["channels", "Raw channels"],
] as const;

export default function SettingsPage({ basePath = "/settings" }: { basePath?: string }) {
  const api = useApi();
  const location = useLocation();
  const advanced = location.pathname.startsWith(`${basePath}/advanced`);
  const strip = useActiveLinkIntoView<HTMLElement>();
  const [integrations, setIntegrations] = useState<PluginContribution[]>([]);
  useEffect(() => {
    let current = true;
    void api.plugins
      .getPluginContributions()
      .then((result) => {
        if (current) setIntegrations(result.plugins.filter((plugin) => plugin.settings));
      })
      .catch(() => {
        if (current) setIntegrations([]);
      });
    return () => {
      current = false;
    };
  }, [api]);
  // Below lg the side column is one sideways-scrolling strip above the page.
  const group =
    "max-lg:mt-0 max-lg:flex max-lg:shrink-0 max-lg:items-center max-lg:gap-1 max-lg:border-t-0 max-lg:border-l max-lg:pt-0 max-lg:pl-2";
  const link = ({ isActive }: { isActive: boolean }) =>
    cn(
      "block rounded px-3 py-1.5 text-sm whitespace-nowrap hover:bg-accent max-lg:shrink-0",
      isActive && "bg-accent font-medium",
    );
  return (
    <div className="flex h-full min-h-0 max-lg:flex-col">
      <aside
        ref={strip}
        className="w-52 shrink-0 overflow-y-auto border-r p-3 max-lg:scrollbar-none max-lg:flex max-lg:w-auto max-lg:items-center max-lg:gap-2 max-lg:overflow-x-auto max-lg:border-r-0 max-lg:border-b max-lg:py-1.5"
      >
        <h1 className="mb-3 text-lg font-semibold max-lg:sr-only">Settings</h1>
        <nav
          aria-label="Settings"
          className="space-y-1 max-lg:flex max-lg:shrink-0 max-lg:gap-1 max-lg:space-y-0"
        >
          {SECTIONS.map((item) => (
            <NavLink
              key={item.suffix}
              to={`${basePath}${item.suffix}`}
              end={item.end}
              className={({ isActive }) => cn(link({ isActive }), "lg:py-2")}
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
        {integrations.length > 0 && (
          <nav aria-label="Integration settings" className={cn("mt-4 border-t pt-3", group)}>
            <div className="mb-1 px-3 text-xs font-semibold uppercase text-muted-foreground max-lg:hidden">
              Integrations
            </div>
            {integrations.map((plugin) => (
              <NavLink
                key={plugin.name}
                to={`${basePath}/integrations/${encodeURIComponent(plugin.name)}`}
                className={link}
              >
                {plugin.settings?.title ?? plugin.name}
              </NavLink>
            ))}
          </nav>
        )}
        {advanced && (
          <nav aria-label="Advanced settings" className={cn("mt-4 border-t pt-3", group)}>
            <div className="mb-1 px-3 text-xs font-semibold uppercase text-muted-foreground max-lg:hidden">
              Operator tools
            </div>
            {ADVANCED.map(([path, label]) => (
              <NavLink key={path} to={`${basePath}/advanced/${path}`} className={link}>
                {label}
              </NavLink>
            ))}
          </nav>
        )}
      </aside>
      <section className="min-w-0 flex-1 overflow-auto p-5 max-sm:p-3">
        <LazyBoundary resetKey={location.pathname}>
          <Outlet />
        </LazyBoundary>
      </section>
    </div>
  );
}

function Copy({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="mx-auto max-w-3xl space-y-3">
      <h2 className="text-xl font-semibold">{title}</h2>
      <p className="text-sm text-muted-foreground">{children}</p>
    </div>
  );
}

export function GeneralSettings() {
  const api = useApi();
  const target = api.target;
  const load = useCallback(
    async () => (await api.daemon.getGlobalAgentShellScript()).script,
    [api],
  );
  const save = useCallback((script: string) => api.daemon.setGlobalAgentShellScript(script), [api]);
  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <div className="space-y-3">
        <h2 className="text-xl font-semibold">General</h2>
        <p className="text-sm text-muted-foreground">
          Tariboy keeps agent sessions, images, and host connections in one workspace.
        </p>
      </div>
      <ShellScriptEditor
        title="Global Agent Shell Script"
        description="Bash commands sourced before every agent iteration on this host."
        load={load}
        save={save}
      />
      <CodexApprovalCard key={target?.id ?? "active"} />
      <MaintenanceSettingsCard key={target?.id ?? "active"} />
      <SupportBundle />
    </div>
  );
}

export function CliSettings() {
  return (
    <Copy title="CLI">CLI integration and command-line launch preferences will appear here.</Copy>
  );
}

export function AppearanceSettings() {
  return (
    <Copy title="Appearance">
      Use the theme switcher in the application header to change appearance.
    </Copy>
  );
}

export function AdvancedSettingsIndex() {
  return (
    <Copy title="Advanced">
      Inspect organization-wide usage, budgets, policy, plugins, groups, and raw event channels.
      These tools are intentionally separated from the everyday agent workflow.
    </Copy>
  );
}
