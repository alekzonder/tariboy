import { Input, Label, Switch, Textarea } from "tariboy-ui";
import { Lock } from "lucide-react";

const help: React.CSSProperties = { fontSize: 12, color: "var(--muted-foreground)" };

export const FieldLabel = () => (
  <div style={{ display: "grid", gap: 6, width: 320 }}>
    <Label htmlFor="preview-label-name">name</Label>
    <Input id="preview-label-name" placeholder="empty = generated" />
  </div>
);

export const FormLabels = () => (
  <div style={{ display: "grid", gap: 12, width: 360 }}>
    <div style={{ display: "grid", gap: 6 }}>
      <Label htmlFor="preview-label-image">image *</Label>
      <Input id="preview-label-image" defaultValue="worker:v2" />
    </div>
    <div style={{ display: "grid", gap: 6 }}>
      <Label htmlFor="preview-label-cwd">cwd</Label>
      <Input id="preview-label-cwd" placeholder="empty = managed workdir" />
    </div>
    <div style={{ display: "grid", gap: 6 }}>
      <Label htmlFor="preview-label-notes">notes</Label>
      <Textarea
        id="preview-label-notes"
        placeholder="one-shot exec prompt (optional)"
      />
    </div>
  </div>
);

export const WithIconAndHelp = () => (
  <div style={{ display: "grid", gap: 6, width: 360 }}>
    <Label htmlFor="preview-label-token">
      <Lock />
      daemon token
    </Label>
    <Input id="preview-label-token" type="password" defaultValue="tb_live_9ac71e3f" />
    <span style={help}>Stored in the OS keychain, never in the project.</span>
  </div>
);

export const SwitchRow = () => (
  <div
    style={{
      display: "flex",
      alignItems: "center",
      justifyContent: "space-between",
      gap: 12,
      width: 360,
    }}
  >
    <div>
      <Label htmlFor="preview-label-interactive">Interactive</Label>
      <p style={{ ...help, margin: "2px 0 0" }}>Attach a terminal console.</p>
    </div>
    <Switch id="preview-label-interactive" defaultChecked />
  </div>
);

export const Disabled = () => (
  <div
    className="group"
    data-disabled="true"
    style={{ display: "grid", gap: 6, width: 320 }}
  >
    <Label htmlFor="preview-label-host">Host</Label>
    <Input id="preview-label-host" defaultValue="This daemon (local)" disabled />
    <span style={help}>The host cannot be changed after creation.</span>
  </div>
);
