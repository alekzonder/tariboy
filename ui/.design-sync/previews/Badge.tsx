import { Badge } from "tariboy-ui";
import { CircleDot, GitBranch, ShieldAlert, Square } from "lucide-react";

const row: React.CSSProperties = {
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: 8,
};

export const AgentState = () => (
  <div style={row}>
    <Badge>running</Badge>
    <Badge variant="secondary">stopped</Badge>
    <Badge variant="destructive">out of budget</Badge>
    <Badge variant="outline">starting</Badge>
  </div>
);

export const Variants = () => (
  <div style={row}>
    <Badge variant="default">running</Badge>
    <Badge variant="secondary">Terminal-only</Badge>
    <Badge variant="destructive">over</Badge>
    <Badge variant="outline">idle finish</Badge>
    <Badge variant="ghost">draft</Badge>
    <Badge variant="link">TB-142</Badge>
  </div>
);

export const WithIcons = () => (
  <div style={row}>
    <Badge>
      <CircleDot data-icon="inline-start" />
      running
    </Badge>
    <Badge variant="secondary">
      <Square data-icon="inline-start" />
      stopped
    </Badge>
    <Badge variant="destructive">
      <ShieldAlert data-icon="inline-start" />
      out of budget
    </Badge>
    <Badge variant="outline">
      <GitBranch data-icon="inline-start" />
      main
    </Badge>
  </div>
);

export const InAgentList = () => (
  <div
    style={{
      display: "grid",
      gap: 6,
      maxWidth: 380,
      border: "1px solid var(--border)",
      borderRadius: 10,
      padding: 10,
      fontSize: 13,
    }}
  >
    {[
      { name: "builder", image: "worker:v2", badge: <Badge>running</Badge> },
      {
        name: "reviewer",
        image: "worker:v1",
        badge: <Badge variant="secondary">stopped</Badge>,
      },
      {
        name: "packager",
        image: "bare:latest",
        badge: <Badge variant="destructive">out of budget</Badge>,
      },
      {
        name: "docs-bot",
        image: "bare:latest",
        badge: <Badge variant="secondary">Terminal-only</Badge>,
      },
    ].map((agent) => (
      <div
        key={agent.name}
        style={{ display: "flex", alignItems: "center", gap: 8 }}
      >
        <span style={{ fontWeight: 500 }}>{agent.name}</span>
        <span style={{ color: "var(--muted-foreground)", fontSize: 12 }}>
          {agent.image}
        </span>
        <span style={{ marginLeft: "auto" }}>{agent.badge}</span>
      </div>
    ))}
  </div>
);
