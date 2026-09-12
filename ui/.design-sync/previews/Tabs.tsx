import {
  Badge,
  Button,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "tariboy-ui";

const WORKSPACE_TABS = [
  ["console", "Console"],
  ["autopilot", "Autopilot"],
  ["activity", "Activity"],
  ["tasks", "Tasks"],
  ["configuration", "Configuration"],
  ["advanced", "Advanced"],
] as const;

export const AgentWorkspace = () => (
  <Tabs defaultValue="console" style={{ width: 640 }}>
    <TabsList>
      {WORKSPACE_TABS.map(([value, label]) => (
        <TabsTrigger key={value} value={value}>
          {label}
        </TabsTrigger>
      ))}
    </TabsList>
    <TabsContent value="console">
      <div
        style={{
          border: "1px solid var(--border)",
          borderRadius: 10,
          padding: "10px 12px",
          font: "13px/20px ui-monospace, SFMono-Regular, Menlo, monospace",
          color: "var(--foreground)",
          whiteSpace: "pre-wrap",
        }}
      >
        {"builder · iteration 42 · worker:v2\n$ tariboy task claim TB-142"}
      </div>
    </TabsContent>
  </Tabs>
);

export const MessageQueueViews = () => (
  <Tabs defaultValue="queue" style={{ width: 420 }}>
    <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
      <TabsList>
        <TabsTrigger value="queue">Queue</TabsTrigger>
        <TabsTrigger value="archive">Archive</TabsTrigger>
        <TabsTrigger value="dlq">DLQ</TabsTrigger>
      </TabsList>
      <Badge variant="secondary">12 pending</Badge>
    </div>
    <TabsContent value="queue">
      <div style={{ display: "grid", gap: 6, fontSize: 13 }}>
        <div>
          <span style={{ fontFamily: "ui-monospace, monospace" }}>reviewer</span>
          <span style={{ color: "var(--muted-foreground)" }}>
            {" "}· review TB-142 before packaging
          </span>
        </div>
        <div>
          <span style={{ fontFamily: "ui-monospace, monospace" }}>docs-bot</span>
          <span style={{ color: "var(--muted-foreground)" }}>
            {" "}· regenerate the CLI reference
          </span>
        </div>
      </div>
    </TabsContent>
  </Tabs>
);

export const WithDisabledTab = () => (
  <Tabs defaultValue="configuration" style={{ width: 480 }}>
    <TabsList>
      <TabsTrigger value="configuration">Configuration</TabsTrigger>
      <TabsTrigger value="activity">Activity</TabsTrigger>
      <TabsTrigger value="advanced" disabled>
        Advanced
      </TabsTrigger>
    </TabsList>
    <TabsContent value="configuration">
      <p style={{ margin: "0 0 8px", fontSize: 13, color: "var(--muted-foreground)" }}>
        Host build-01 is unavailable; advanced actions are disabled until it
        reconnects.
      </p>
      <Button variant="outline" size="sm" disabled>
        Restart builder
      </Button>
    </TabsContent>
  </Tabs>
);
