import { useEffect, useRef } from "react";
import { DaemonProvider, HostSwitcher, ThemeToggle, Button } from "tariboy-ui";
import { Plus } from "lucide-react";

// HostSwitcher reads the registry through useDaemons(), so it needs a real
// DaemonProvider. Outside Tauri the registry is the browser one (localStorage
// keys from src/lib/daemons.ts), which the fixture seeds during render —
// DaemonProvider's own state initializer and mount fetch then read it exactly
// as they do in the app. Each cell is captured on its own page load, so the
// seeded registry never leaks between cells.
type Host = { id: string; label: string; baseURL: string; kind?: string; state?: string };

const HOSTS: Host[] = [
  { id: "d_build01", label: "build-01", baseURL: "https://10.0.4.11:7777", kind: "ssh", state: "ready" },
  { id: "d_build02", label: "build-02", baseURL: "https://10.0.4.12:7777", kind: "ssh", state: "ready" },
  { id: "d_macmini", label: "mac-mini", baseURL: "https://10.0.4.20:7777", kind: "https", state: "ready" },
];

function seedRegistry(hosts: Host[], activeId: string) {
  try {
    localStorage.setItem("tariboy_daemons", JSON.stringify(hosts));
    if (activeId) localStorage.setItem("tariboy_active_daemon", activeId);
    else localStorage.removeItem("tariboy_active_daemon");
    for (const host of hosts) {
      sessionStorage.setItem(`tariboy_daemon_token_${host.id}`, "preview-token");
    }
  } catch {
    /* storage is a convenience here, same as in the app */
  }
}

const Registry = ({
  hosts,
  activeId,
  children,
}: {
  hosts: Host[];
  activeId: string;
  children: React.ReactNode;
}) => {
  // Render phase: the provider below mounts after this, so it sees the seed.
  seedRegistry(hosts, activeId);
  return <DaemonProvider>{children}</DaemonProvider>;
};

// The workspace titlebar the switcher actually lives in.
const Titlebar = ({ children }: { children: React.ReactNode }) => (
  <div
    style={{
      width: 640,
      display: "flex",
      alignItems: "center",
      gap: 12,
      padding: "8px 12px",
      borderRadius: 10,
      border: "1px solid var(--border)",
      background: "var(--card)",
    }}
  >
    <span style={{ fontSize: 14, fontWeight: 600 }}>Tariboy</span>
    <span style={{ fontSize: 12, color: "var(--muted-foreground)" }}>builder · worker:v2</span>
    <div style={{ marginLeft: "auto", display: "flex", alignItems: "center", gap: 8 }}>
      {children}
      <ThemeToggle />
    </div>
  </div>
);

// Radix Select keeps its open state internal — no `open` prop reaches
// HostSwitcher — so the open cells press the real trigger.
function useOpenTrigger() {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const trigger = ref.current?.querySelector<HTMLElement>('[role="combobox"]');
    if (!trigger) return;
    trigger.dispatchEvent(
      new PointerEvent("pointerdown", { bubbles: true, button: 0, isPrimary: true }),
    );
    if (trigger.getAttribute("aria-expanded") === "true") return;
    trigger.focus();
    trigger.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true }));
  }, []);
  return ref;
}

export const LocalDaemon = () => (
  <Registry hosts={HOSTS} activeId="">
    <Titlebar>
      <HostSwitcher />
    </Titlebar>
  </Registry>
);

export const RemoteHostSelected = () => (
  <Registry hosts={HOSTS} activeId="d_build01">
    <Titlebar>
      <HostSwitcher />
    </Titlebar>
  </Registry>
);

export const HostListOpen = () => {
  const ref = useOpenTrigger();
  return (
    <Registry hosts={HOSTS} activeId="d_build01">
      <div ref={ref}>
        <Titlebar>
          <HostSwitcher />
        </Titlebar>
      </div>
    </Registry>
  );
};

export const InHostSettings = () => (
  <Registry hosts={[]} activeId="">
    <div
      style={{
        width: 640,
        display: "flex",
        alignItems: "center",
        gap: 12,
        padding: "12px 14px",
        borderRadius: 10,
        border: "1px solid var(--border)",
        background: "var(--card)",
      }}
    >
      <div style={{ display: "grid", gap: 2 }}>
        <span style={{ fontSize: 14, fontWeight: 600 }}>Hosts</span>
        <span style={{ fontSize: 12, color: "var(--muted-foreground)" }}>
          No daemons registered yet — agents run on this machine.
        </span>
      </div>
      <div style={{ marginLeft: "auto", display: "flex", alignItems: "center", gap: 8 }}>
        <HostSwitcher />
        <Button size="sm" variant="outline">
          <Plus className="size-4" />
          Add host
        </Button>
      </div>
    </div>
  </Registry>
);
