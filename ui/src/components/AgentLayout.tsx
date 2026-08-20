import { useCallback, useEffect, useRef, useState } from "react";
import { NavLink, Outlet, useParams } from "react-router-dom";
import { usePolling } from "@/hooks/usePolling";
import { getAgentStatus, listAgents } from "@/lib/api";
import { AgentNameContext, AgentStatusContext } from "@/lib/agent";
import { AgentColorSwatch } from "@/components/AgentColorSwatch";
import { AliasEditor } from "@/components/AliasEditor";
import { useCachedColor } from "@/lib/colorCache";
import { cn, colorStyle, displayName, swatchStyle } from "@/lib/utils";

const TABS = [
  { to: "", label: "Overview", end: true },
  { to: "tasks", label: "Tasks" },
  { to: "channels", label: "Channels" },
  { to: "messages", label: "Messages" },
  { to: "logs", label: "Audit Log" },
  { to: "usage", label: "Usage" },
  { to: "scripts", label: "Scripts" },
  { to: "prompt", label: "Prompt" },
  { to: "context", label: "Context" },
  { to: "files", label: "Files" },
  { to: "settings", label: "Settings" },
];

/**
 * @deprecated Product routes use AgentWorkspace. Kept temporarily for focused
 * compatibility tests and embedders while App redirects every /agent/:name URL.
 */
export function AgentLayout() {
  const { name = "" } = useParams();
  const { data, refresh } = usePolling(listAgents, 5000);
  const agents = data?.agents ?? [];
  const current = agents.find((a) => a.name === name);
  // Capture the app title index.html seeded (the single source of truth for the
  // app name) once on first render, so the reset value can never drift from it.
  const appTitleRef = useRef<string | null>(null);
  if (appTitleRef.current === null) appTitleRef.current = document.title;
  const appTitle = appTitleRef.current;
  // Keep the browser tab title in sync with the selected agent: alias-first via
  // displayName() once the polled snapshot carries it, else the bare name so the
  // title is right before the first /api/agents poll resolves. Reset to the
  // captured app title on unmount so leaving agent views (Home, other pages)
  // drops the name.
  const title = current ? displayName(current) : name || appTitle;
  useEffect(() => {
    document.title = title;
    return () => {
      document.title = appTitle;
    };
  }, [title, appTitle]);
  // Poll the agent status so the unified header (rendered above the tabs on
  // every tab) can show the live loop state; same call the Overview tab used.
  // Shared to the tabs via AgentStatusContext so Overview reads this snapshot
  // instead of opening a second identical 2s /status poll.
  const { data: status, refresh: refreshStatus } = usePolling(() => getAgentStatus(name), 2000);

  // Optimistic color for the selected agent: the header/swatch read from the
  // polled `/api/agents` snapshot, so a freshly-saved color wouldn't appear
  // until the next poll. Remember the just-saved hex and show it until the
  // polled snapshot catches up — pure derivation (no effects) so it self-clears
  // once the poll agrees or the user switches agents.
  const [pending, setPending] = useState<{ name: string; color: string } | null>(null);
  const optimisticColor =
    pending && pending.name === name && current?.color !== pending.color ? pending.color : undefined;
  const currentColor = optimisticColor ?? current?.color;
  // Paint from the localStorage cache immediately so the header tint + swatch
  // render the right color on load, before the first /api/agents poll resolves.
  const effectiveColor = useCachedColor(name, currentColor);

  // Called after the color POST persists: show it now, then re-poll so the
  // snapshot picks it up (the backend reflects the DB on the next poll).
  const handleColorSaved = useCallback(
    async (hex: string) => {
      setPending({ name, color: hex });
      await refresh();
    },
    [name, refresh],
  );

  // Group the agent list into blocks by their `group` field, preserving
  // first-seen order; ungrouped agents render in a trailing block with no
  // header so grouped agents never get lost among them, and an agent appears in
  // exactly one block. Mirrors the old UI's grouped sidebar sections.
  const groupOrder: string[] = [];
  const grouped = new Map<string, typeof agents>();
  const ungrouped: typeof agents = [];
  for (const a of agents) {
    if (a.group) {
      if (!grouped.has(a.group)) {
        groupOrder.push(a.group);
        grouped.set(a.group, []);
      }
      grouped.get(a.group)!.push(a);
    } else {
      ungrouped.push(a);
    }
  }
  const blocks: { key: string; label: string | null; agents: typeof agents }[] = [
    ...groupOrder.map((g) => ({ key: g, label: g, agents: grouped.get(g)! })),
    ...(ungrouped.length > 0 ? [{ key: "__ungrouped", label: null, agents: ungrouped }] : []),
  ];

  // One agent row. Shared across all blocks so grouped/ungrouped rows keep
  // identical per-row behaviour (active highlight, stopped styling, dot).
  const renderRow = (a: (typeof agents)[number]) => (
    <NavLink
      key={a.name}
      to={`/agent/${encodeURIComponent(a.name)}`}
      className={({ isActive }) =>
        cn(
          "flex items-center gap-2 px-3 py-1.5 text-sm hover:bg-accent",
          isActive && "bg-accent font-medium",
        )
      }
    >
      <span
        data-testid="agent-swatch"
        className="agent-swatch inline-block size-2.5 shrink-0 rounded-full"
        style={swatchStyle(a.name === name ? effectiveColor : a.color)}
      />
      <span className={cn("truncate", a.state !== "running" && "text-muted-foreground")}>
        {a.name}
      </span>
      {a.state !== "running" && <span className="text-xs">(stopped)</span>}
    </NavLink>
  );

  return (
    <AgentNameContext.Provider value={name}>
      <div className="flex h-full">
        <aside className="w-56 shrink-0 overflow-auto border-r">
          <div className="p-2 text-xs font-medium text-muted-foreground">AGENTS</div>
          {blocks.map((block) => (
            <div key={block.key}>
              {block.label && (
                <div
                  data-testid={`agent-group-${block.label}`}
                  className="px-3 pt-3 pb-1 text-xs font-semibold tracking-wide text-muted-foreground uppercase"
                >
                  {block.label}
                </div>
              )}
              {block.agents.map(renderRow)}
            </div>
          ))}
        </aside>
        <div className="flex flex-1 flex-col overflow-hidden">
          {/* Unified agent header: rendered above the tabs on every tab. Carries
              the alias editor and the live loop status. */}
          <header className="flex flex-wrap items-center gap-3 border-b px-4 py-2">
            <AliasEditor />
            <span className="text-sm text-muted-foreground">
              status: <span className="font-mono text-foreground">{status?.state ?? "…"}</span>
            </span>
          </header>
          {/* The tab strip doubles as the agent header: it carries the per-agent
              color tint (theme-aware, via `.agent-header` + `--agent-hue`) and the
              color swatch, so the tint + affordance stay visible on every tab. */}
          <nav
            className="agent-header flex items-center gap-1 border-b px-2"
            data-testid="agent-header"
            style={colorStyle(effectiveColor)}
          >
            {TABS.map((t) => (
              <NavLink
                key={t.to}
                to={t.to ? `/agent/${encodeURIComponent(name)}/${t.to}` : `/agent/${encodeURIComponent(name)}`}
                end={t.end}
                className={({ isActive }) =>
                  cn("px-3 py-2 text-sm", isActive ? "border-b-2 border-primary font-medium" : "text-muted-foreground")
                }
              >
                {t.label}
              </NavLink>
            ))}
            <div className="ml-auto flex items-center pr-1">
              <AgentColorSwatch name={name} color={effectiveColor} onSaved={handleColorSaved} />
            </div>
          </nav>
          <div className="flex-1 overflow-auto p-4">
            <AgentStatusContext.Provider value={{ status: status ?? null, refresh: refreshStatus }}>
              <Outlet />
            </AgentStatusContext.Provider>
          </div>
        </div>
      </div>
    </AgentNameContext.Provider>
  );
}
