import {
  Button,
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "tariboy-ui";

// The agent detail screen the sheet slides over.
const AgentScreen = ({ children }: { children?: React.ReactNode }) => (
  <div style={{ display: "grid", gap: 12, width: 700 }}>
    <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
      <span style={{ fontSize: 14, fontWeight: 500 }}>reviewer</span>
      <span style={{ fontSize: 12, color: "var(--muted-foreground)" }}>
        worker:v2 · This daemon (local)
      </span>
      <span style={{ marginLeft: "auto" }}>{children}</span>
    </div>
    <div
      style={{
        height: 220,
        borderRadius: 8,
        border: "1px solid var(--border)",
        background: "var(--muted)",
      }}
    />
  </div>
);

const EVENTS = [
  { ts: "2024-05-15 11:58:04", message: "Picked up TB-142 from the queue." },
  { ts: "2024-05-15 11:59:41", message: "Reviewed 3 files, left 2 comments." },
  { ts: "2024-05-15 12:04:12", message: "Waiting on builder to push a fix." },
  { ts: "2024-05-15 12:11:30", message: "Re-reviewed TB-142 — approving." },
];

const StatusList = () => (
  <div style={{ padding: "0 16px", overflow: "hidden" }}>
    <h3
      style={{
        fontSize: 12,
        fontWeight: 600,
        color: "var(--muted-foreground)",
        padding: "8px 0",
        borderBottom: "1px solid var(--border)",
      }}
    >
      Iteration: 2024-05-15T11-57
    </h3>
    {EVENTS.map((event) => (
      <div
        key={event.ts}
        style={{ padding: "8px 0", borderBottom: "1px solid var(--border)", fontSize: 13 }}
      >
        <div style={{ fontSize: 12, color: "var(--muted-foreground)" }}>{event.ts}</div>
        <div>{event.message}</div>
      </div>
    ))}
  </div>
);

export const StatusHistory = () => (
  <AgentScreen>
    <Sheet defaultOpen>
      <SheetTrigger asChild>
        <Button size="sm" variant="ghost" aria-label="status history">
          history
        </Button>
      </SheetTrigger>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>Status history — reviewer</SheetTitle>
          <SheetDescription>
            Status updates, newest first, grouped by iteration.
          </SheetDescription>
        </SheetHeader>
        <StatusList />
        <SheetFooter>
          <Button variant="outline">Export as JSON</Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  </AgentScreen>
);

export const SideLeft = () => (
  <AgentScreen>
    <Sheet defaultOpen>
      <SheetTrigger asChild>
        <Button size="sm" variant="outline">
          Hosts
        </Button>
      </SheetTrigger>
      <SheetContent side="left">
        <SheetHeader>
          <SheetTitle>Hosts</SheetTitle>
          <SheetDescription>Daemons this workspace can reach.</SheetDescription>
        </SheetHeader>
        <div style={{ padding: "0 16px", display: "grid", gap: 6 }}>
          {[
            { label: "This daemon (local)", note: "4 agents" },
            { label: "build-01", note: "2 agents" },
            { label: "build-02", note: "offline" },
          ].map((host) => (
            <div
              key={host.label}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 8,
                fontSize: 13,
                padding: "6px 8px",
                borderRadius: 6,
                border: "1px solid var(--border)",
              }}
            >
              <span>{host.label}</span>
              <span
                style={{ marginLeft: "auto", fontSize: 12, color: "var(--muted-foreground)" }}
              >
                {host.note}
              </span>
            </div>
          ))}
        </div>
      </SheetContent>
    </Sheet>
  </AgentScreen>
);

export const SideTop = () => (
  <AgentScreen>
    <Sheet defaultOpen>
      <SheetTrigger asChild>
        <Button size="sm" variant="outline">
          Release notes
        </Button>
      </SheetTrigger>
      <SheetContent side="top">
        <SheetHeader>
          <SheetTitle>Tariboy 0.58.1 is installed</SheetTitle>
          <SheetDescription>
            Every host on this workspace is now on 0.58.1. Agents kept their
            sessions.
          </SheetDescription>
        </SheetHeader>
        <SheetFooter>
          <div style={{ display: "flex", justifyContent: "flex-end", gap: 8 }}>
            <Button variant="outline">Dismiss</Button>
            <Button>Read the changelog</Button>
          </div>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  </AgentScreen>
);

export const SideBottom = () => (
  <AgentScreen>
    <Sheet defaultOpen>
      <SheetTrigger asChild>
        <Button size="sm" variant="outline">
          Send files
        </Button>
      </SheetTrigger>
      <SheetContent side="bottom">
        <SheetHeader>
          <SheetTitle>Send files to packager</SheetTitle>
          <SheetDescription>
            Files land in /work/inbox on This daemon (local).
          </SheetDescription>
        </SheetHeader>
        <div style={{ padding: "0 16px", display: "grid", gap: 6 }}>
          {["release-notes.md", "worker-manifest.json"].map((name) => (
            <div
              key={name}
              style={{
                fontSize: 13,
                fontFamily: "var(--font-mono)",
                padding: "6px 8px",
                borderRadius: 6,
                border: "1px solid var(--border)",
              }}
            >
              {name}
            </div>
          ))}
        </div>
        <SheetFooter>
          <div style={{ display: "flex", justifyContent: "flex-end", gap: 8 }}>
            <Button variant="outline">Cancel</Button>
            <Button>Upload 2 files</Button>
          </div>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  </AgentScreen>
);
