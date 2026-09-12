import { AuditExportActions, Input } from "tariboy-ui";

// AuditExportActions is the Copy / Export pair that sits at the right end of an
// audit log header. Copy writes the rendered Markdown to the clipboard; Export
// is gated by an AlertDialog warning that the ZIP carries prompts, commands and
// model responses. With `iteration` it scopes to one iteration; without it the
// whole agent audit is exported.

const mono = {
  font: "12px ui-monospace, SFMono-Regular, Menlo, monospace",
  color: "var(--muted-foreground)",
} as const;

export const IterationHeader = () => (
  <div style={{ width: 520, display: "flex", flexDirection: "column", gap: 4 }}>
    <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8 }}>
      <div style={mono}>iteration 4f2a91 · ok</div>
      <AuditExportActions name="builder" iteration="4f2a91c7-2b60-4f1e-8f0a-91d2c7e51a33" />
    </div>
    <div
      style={{
        borderRadius: 6,
        border: "1px solid var(--border)",
        background: "var(--muted)",
        padding: 8,
        ...mono,
      }}
    >
      18:22:41 iteration loop · 18:22:49 Bash go test ./... · 18:23:07 iteration ok
    </div>
  </div>
);

export const FullAuditToolbar = () => (
  <div style={{ width: 560, display: "flex", alignItems: "center", gap: 8 }}>
    <div
      style={{
        height: 32,
        display: "flex",
        alignItems: "center",
        padding: "0 10px",
        borderRadius: 6,
        border: "1px solid var(--input)",
        fontSize: 13,
        color: "var(--muted-foreground)",
        whiteSpace: "nowrap",
      }}
    >
      all types
    </div>
    <Input aria-label="Search text" placeholder="search all fields…" defaultValue="updater" className="h-8" />
    <AuditExportActions name="builder" />
  </div>
);

export const AgentScoped = () => (
  <div style={{ width: 340, display: "flex", alignItems: "center", gap: 10 }}>
    <span style={{ fontSize: 13 }}>
      Full audit for <span style={{ font: "12px ui-monospace, SFMono-Regular, Menlo, monospace" }}>docs-bot</span>
    </span>
    <span style={{ marginLeft: "auto" }}>
      <AuditExportActions name="docs-bot" />
    </span>
  </div>
);
