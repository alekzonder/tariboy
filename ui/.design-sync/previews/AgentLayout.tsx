import type { ReactNode } from "react";
import { AgentLayout, MemoryRouter, Route, Routes } from "tariboy-ui";

// AgentLayout is the deprecated /agent/:name shell (App now redirects those URLs
// to AgentWorkspace). It reads its agent from useParams(), polls /api/agents for
// the sidebar and /api/agents/:name/status for the header, and renders the tab
// strip plus an <Outlet/> for the active tab.
//
// The component derives its whole identity from useParams(), so it must be
// mounted inside a MATCHED route — MemoryRouter alone matches no pattern and
// leaves the agent nameless. The eleven-tab strip is wider than the 900px
// capture viewport, so it runs past the right edge.
//
// Two things the preview has to supply, or the card teaches the wrong lesson:
//
// 1. THE TAB STRIP'S TINT. `.agent-header` colours itself from `--agent-hue`,
//    which AgentLayout feeds from the agent's stored colour. With no daemon
//    behind the capture server that colour never arrives and the hue falls back
//    to 0 — a pink strip that looks like an error state and is unrepresentative
//    of every real agent. `useCachedColor` falls back to localStorage, so the
//    cache is seeded at module scope (each cell is captured on its own page
//    load, so this is per-cell and cannot leak). #4f46e5 is builder's indigo,
//    the same value AgentColorSwatch's preview uses.
// 2. THE OUTLET. Without a child route the whole right-hand pane is blank and
//    the card reads as broken chrome. The Overview body below is the tab's real
//    shape — the shell renders it exactly as the app would.
//
// The sidebar rail and `status:` still come from /api/agents, which 404s here;
// those stay in their genuine empty state.
try {
  localStorage.setItem(
    "agent:color:builder",
    JSON.stringify({ color: "#4f46e5", ts: Date.now() }),
  );
} catch {
  /* private mode / blocked storage — the card still renders, just untinted */
}

const Frame = ({ children }: { children: ReactNode }) => (
  <div
    style={{
      height: 540,
      width: 840,
      overflow: "hidden",
      border: "1px solid var(--border)",
      borderRadius: "var(--radius)",
      background: "var(--background)",
    }}
  >
    {children}
  </div>
);

const Row = ({ label, value, mono }: { label: string; value: string; mono?: boolean }) => (
  <div className="flex items-center justify-between gap-3 border-b py-2 text-sm">
    <span className="text-muted-foreground">{label}</span>
    <span className={mono ? "font-mono" : undefined}>{value}</span>
  </div>
);

const Overview = () => (
  <div style={{ maxWidth: 520 }}>
    <Row label="image" value="worker:v2" mono />
    <Row label="host" value="This daemon (local)" />
    <Row label="goal" value="TB-142" mono />
    <Row label="iteration" value="42 · 1m 18s" />
    <Row label="spend (24h)" value="$4.10 of $20.00" />
  </div>
);

export const AgentShell = () => (
  <MemoryRouter initialEntries={["/agent/builder"]}>
    <Frame>
      <Routes>
        <Route path="/agent/:name/*" element={<AgentLayout />}>
          <Route index element={<Overview />} />
        </Route>
      </Routes>
    </Frame>
  </MemoryRouter>
);
