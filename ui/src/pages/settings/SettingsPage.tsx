import { NavLink, Outlet, useLocation } from "react-router-dom";
import type { ApiTarget } from "@/lib/api";
import { cn } from "@/lib/utils";
import { SupportBundle } from "@/components/SupportBundle";

const SECTIONS = [
  { suffix: "", label: "General", end: true },
  { suffix: "/task-reminders", label: "Task reminders" },
  { suffix: "/hosts", label: "Hosts" },
  { suffix: "/cli", label: "CLI" },
  { suffix: "/appearance", label: "Appearance" },
  { suffix: "/advanced", label: "Advanced" },
];

const ADVANCED = [
  ["usage", "Usage"],
  ["budgets", "Budgets"],
  ["rules", "Rules"],
  ["evals", "Evals"],
  ["judges", "Judges"],
  ["plugins", "Plugins"],
  ["ops", "Ops"],
  ["daemons", "Daemons"],
  ["groups", "Groups"],
  ["channels", "Raw channels"],
] as const;

export default function SettingsPage({ basePath = "/settings", target = null }: {
  basePath?: string;
  target?: ApiTarget;
}) {
  const location = useLocation();
  const advanced = location.pathname.startsWith(`${basePath}/advanced`);
  return (
    <div className="flex h-full min-h-0">
      <aside className="w-52 shrink-0 overflow-y-auto border-r p-3">
        <h1 className="mb-3 text-lg font-semibold">Settings</h1>
        <nav aria-label="Settings" className="space-y-1">
          {SECTIONS.map((item) => (
            <NavLink
              key={item.suffix}
              to={`${basePath}${item.suffix}`}
              end={item.end}
              className={({ isActive }) =>
                cn("block rounded px-3 py-2 text-sm hover:bg-accent", isActive && "bg-accent font-medium")
              }
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
        {advanced && (
          <nav aria-label="Advanced settings" className="mt-4 border-t pt-3">
            <div className="mb-1 px-3 text-xs font-semibold uppercase text-muted-foreground">Operator tools</div>
            {ADVANCED.map(([path, label]) => (
              <NavLink
                key={path}
                to={`${basePath}/advanced/${path}`}
                className={({ isActive }) =>
                  cn("block rounded px-3 py-1.5 text-sm hover:bg-accent", isActive && "bg-accent font-medium")
                }
              >
                {label}
              </NavLink>
            ))}
          </nav>
        )}
      </aside>
      <section className="min-w-0 flex-1 overflow-auto p-5">
        <Outlet context={target} />
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
  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <div className="space-y-3">
        <h2 className="text-xl font-semibold">General</h2>
        <p className="text-sm text-muted-foreground">
          Tariboy keeps agent sessions, images, and host connections in one workspace.
        </p>
      </div>
      <SupportBundle />
    </div>
  );
}

export function CliSettings() {
  return <Copy title="CLI">CLI integration and command-line launch preferences will appear here.</Copy>;
}

export function AppearanceSettings() {
  return <Copy title="Appearance">Use the theme switcher in the application header to change appearance.</Copy>;
}

export function AdvancedSettingsIndex() {
  return (
    <Copy title="Advanced">
      Inspect organization-wide usage, budgets, policy, evaluations, plugins, groups, and raw event channels.
      These tools are intentionally separated from the everyday agent workflow.
    </Copy>
  );
}
