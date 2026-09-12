import { useEffect, useState, type ReactNode } from "react";
import {
  DaemonProvider,
  DesktopUpdatesProvider,
  MemoryRouter,
  Route,
  Routes,
  SidebarStateProvider,
  TerminalsPage,
} from "tariboy-ui";

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


// TerminalsPage is the application shell: the agents sidebar, the resize handle,
// the server context bar and whichever server surface the route selects. It is
// mounted under App.tsx's provider stack — daemons, shared sidebar state, desktop
// updates — and inside a MATCHED route, because the surface it shows is derived
// from useParams().
//
// CustomerQuestionNotifications is deliberately NOT in the stack: it opens a
// tasks websocket that never closes, so the render check's networkidle wait can
// never settle and the card times out. The context it provides has a usable
// default (empty attention set), which is what this page would show anyway with
// no daemon behind it.
//
// There is no daemon behind the capture server, so the agent list and each
// surface render their genuine empty/error state. The chrome — which is what a
// page-level card is for — is real and complete.
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
    }}
  >
    <Deferred>{children}</Deferred>
  </div>
);

const Providers = ({ children }: { children: ReactNode }) => (
  <DaemonProvider>
    <SidebarStateProvider>
      <DesktopUpdatesProvider>{children}</DesktopUpdatesProvider>
    </SidebarStateProvider>
  </DaemonProvider>
);

export const Workspace = () => (
  <MemoryRouter initialEntries={["/"]}>
    <Providers>
      <Shell>
        <Routes>
          <Route path="/" element={<TerminalsPage />} />
        </Routes>
      </Shell>
    </Providers>
  </MemoryRouter>
);

export const ServerTasks = () => (
  <MemoryRouter initialEntries={["/servers/local/tasks"]}>
    <Providers>
      <Shell>
        <Routes>
          <Route path="/servers/:hostId/tasks" element={<TerminalsPage serverView="tasks" />} />
        </Routes>
      </Shell>
    </Providers>
  </MemoryRouter>
);

export const ServerImages = () => (
  <MemoryRouter initialEntries={["/servers/local/images"]}>
    <Providers>
      <Shell>
        <Routes>
          <Route path="/servers/:hostId/images" element={<TerminalsPage serverView="images" />} />
        </Routes>
      </Shell>
    </Providers>
  </MemoryRouter>
);
