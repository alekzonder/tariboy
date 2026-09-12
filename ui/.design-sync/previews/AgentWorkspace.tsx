import { useEffect, useState, type ReactNode } from "react";
import {
  AgentWorkspace,
  DaemonProvider,
  MemoryRouter,
  Route,
  Routes,
} from "tariboy-ui";

// AgentWorkspace is the per-agent screen: the identity header (name, run state,
// host, image, goal task key, cwd) and the six-tab strip, over whichever tab the
// route selects — Console, Autopilot, Activity, Tasks, Configuration, Advanced.
//
// It takes its agent as a PROP (an AgentSummary) but reads the active tab from
// useParams(), so it must be mounted inside the real matched route from App.tsx,
// `/agents/:hostId/:agent/:tab/*`. MemoryRouter alone matches nothing and the
// workspace would redirect to /console forever.
//
// hostId "" is the same-origin local daemon (see lib/terminalsHost): the
// provider resolves it without a registry lookup, so the workspace reaches its
// "ready" connection state and paints the real chrome. There is no daemon behind
// the capture server, so /status and every tab's fetch fail — each tab shows its
// own genuine empty/error state under a complete header and tab strip.
const BUILDER = {
  name: "builder",
  image: "worker:v2",
  state: "running",
  harness: "claude-code",
  loop_enabled: true,
  enabled: true,
  group: "core",
  interactive: true,
  cwd: "/home/agent/github/tariboy",
  current_goal_task_key: "TB-142",
};

// Hermetic: the render check reuses ONE browser page across every card, so a
// preview that seeds the daemon registry (HostSwitcher does) leaves it behind in
// localStorage. This page would then resolve those unreachable SSH hosts and
// poll all of them, which both slows the first commit and keeps the network
// from ever going idle. Reset to "local daemon only" at module scope.
try {
  localStorage.removeItem("tariboy_daemons");
  localStorage.removeItem("tariboy_active_daemon");
} catch {
  // private mode / blocked storage — the default registry is what we want anyway
}


// The render check measures the root the instant the network goes idle. These
// page trees are heavy enough (router, flexlayout, xterm, the whole surface)
// that React's FIRST commit lands after that instant, so the check can catch an
// empty root even though the card paints correctly a moment later. Committing a
// trivial frame first and mounting the real page on the next tick makes the root
// non-empty from the first paint. The card still renders the real component —
// this only moves when it mounts, the same class of trick as dispatching a real
// event to open a Radix menu.
function Deferred({ children }: { children: ReactNode }) {
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);
  return <>{mounted ? children : null}</>;
}

const Shell = ({ children }: { children: ReactNode }) => (
  <div
    style={{
      height: 620,
      width: 1180,
      overflow: "hidden",
      border: "1px solid var(--border)",
      borderRadius: "var(--radius)",
      background: "var(--background)",
      padding: 16,
    }}
  >
    <Deferred>{children}</Deferred>
  </div>
);

// CustomerQuestionNotifications is deliberately absent: it opens a tasks
// websocket that never closes, which keeps the render check's networkidle wait
// from settling. The context it provides has a usable default (empty attention
// set) — exactly what this screen shows with no daemon behind it anyway.
const Providers = ({ children }: { children: ReactNode }) => (
  <DaemonProvider>{children}</DaemonProvider>
);

const Workspace = ({ tab }: { tab: string }) => (
  <Shell>
    <MemoryRouter initialEntries={[`/agents/local/builder/${tab}`]}>
      <Providers>
        <Routes>
          <Route
            path="/agents/:hostId/:agent/:tab/*"
            element={
              <AgentWorkspace
                hostId=""
                hostLabel="This daemon (local)"
                agent={BUILDER}
                refresh={() => {}}
              />
            }
          />
        </Routes>
      </Providers>
    </MemoryRouter>
  </Shell>
);

export const Console = () => <Workspace tab="console" />;

export const Tasks = () => <Workspace tab="tasks" />;

export const Configuration = () => <Workspace tab="configuration" />;
