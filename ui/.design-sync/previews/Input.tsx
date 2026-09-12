import { Input, Label } from "tariboy-ui";

const field: React.CSSProperties = { display: "grid", gap: 6 };
const help: React.CSSProperties = { fontSize: 12, color: "var(--muted-foreground)" };

export const LabelledField = () => (
  <div style={{ ...field, width: 320 }}>
    <Label htmlFor="preview-agent-name">name</Label>
    <Input id="preview-agent-name" placeholder="empty = generated" />
    <span style={help}>Leave empty and the daemon names the agent.</span>
  </div>
);

export const IdentityForm = () => (
  <div
    style={{
      display: "grid",
      gridTemplateColumns: "1fr 1fr",
      gap: 12,
      width: 460,
    }}
  >
    <div style={field}>
      <Label htmlFor="preview-identity-name">name</Label>
      <Input id="preview-identity-name" defaultValue="builder" />
    </div>
    <div style={field}>
      <Label htmlFor="preview-identity-alias">alias</Label>
      <Input id="preview-identity-alias" defaultValue="bld" />
    </div>
    <div style={field}>
      <Label htmlFor="preview-identity-group">group</Label>
      <Input id="preview-identity-group" defaultValue="release-train" />
    </div>
    <div style={field}>
      <Label htmlFor="preview-identity-color">color</Label>
      <Input id="preview-identity-color" placeholder="#rrggbb" />
    </div>
  </div>
);

export const States = () => (
  <div style={{ display: "grid", gap: 12, width: 340 }}>
    <div style={field}>
      <Label htmlFor="preview-state-value">image</Label>
      <Input id="preview-state-value" defaultValue="worker:v2" />
    </div>
    <div style={field}>
      <Label htmlFor="preview-state-placeholder">cwd</Label>
      <Input
        id="preview-state-placeholder"
        placeholder="empty = managed workdir"
      />
    </div>
    <div style={field}>
      <Label htmlFor="preview-state-disabled">host</Label>
      <Input
        id="preview-state-disabled"
        defaultValue="This daemon (local)"
        disabled
      />
    </div>
    <div style={field}>
      <Label htmlFor="preview-state-invalid">limit USD</Label>
      <Input id="preview-state-invalid" defaultValue="-20" aria-invalid />
      <span style={{ ...help, color: "var(--destructive)" }}>
        Budget limit must be a positive amount.
      </span>
    </div>
  </div>
);

export const Types = () => (
  <div style={{ display: "grid", gap: 12, width: 340 }}>
    <div style={field}>
      <Label htmlFor="preview-type-search">Search agents</Label>
      <Input id="preview-type-search" type="search" placeholder="Search agents…" />
    </div>
    <div style={field}>
      <Label htmlFor="preview-type-number">maximum queued messages</Label>
      <Input id="preview-type-number" type="number" min={1} defaultValue={25} />
    </div>
    <div style={field}>
      <Label htmlFor="preview-type-mono">resolved image ref</Label>
      <Input
        id="preview-type-mono"
        defaultValue="ghcr.io/tariboy/worker:v2"
        style={{ fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace" }}
        readOnly
      />
    </div>
  </div>
);
