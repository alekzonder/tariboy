import { Label, Switch } from "tariboy-ui";

const help: React.CSSProperties = { fontSize: 12, color: "var(--muted-foreground)" };

const Row = ({
  id,
  label,
  hint,
  children,
}: {
  id: string;
  label: string;
  hint: string;
  children: React.ReactNode;
}) => (
  <div
    style={{
      display: "flex",
      alignItems: "center",
      justifyContent: "space-between",
      gap: 16,
    }}
  >
    <div>
      <Label htmlFor={id}>{label}</Label>
      <p style={{ ...help, margin: "2px 0 0" }}>{hint}</p>
    </div>
    {children}
  </div>
);

export const SettingsRows = () => (
  <div style={{ display: "grid", gap: 14, width: 400 }}>
    <Row
      id="preview-switch-interactive"
      label="Interactive"
      hint="Attach a terminal console."
    >
      <Switch id="preview-switch-interactive" defaultChecked />
    </Row>
    <Row
      id="preview-switch-goal"
      label="Goal"
      hint="Deliver the agent's current Native Task goal."
    >
      <Switch id="preview-switch-goal" />
    </Row>
    <Row
      id="preview-switch-start"
      label="Start now"
      hint="Off creates the agent in stopped state."
    >
      <Switch id="preview-switch-start" defaultChecked />
    </Row>
  </div>
);

export const CheckedStates = () => (
  <div style={{ display: "flex", alignItems: "center", gap: 20 }}>
    <Label htmlFor="preview-switch-on" style={{ gap: 8 }}>
      <Switch id="preview-switch-on" defaultChecked />
      loop enabled
    </Label>
    <Label htmlFor="preview-switch-off" style={{ gap: 8 }}>
      <Switch id="preview-switch-off" />
      loop disabled
    </Label>
  </div>
);

export const Sizes = () => (
  <div style={{ display: "flex", alignItems: "center", gap: 20 }}>
    <Label htmlFor="preview-switch-sm-on" style={{ gap: 8 }}>
      <Switch id="preview-switch-sm-on" size="sm" defaultChecked />
      sm · on
    </Label>
    <Label htmlFor="preview-switch-sm-off" style={{ gap: 8 }}>
      <Switch id="preview-switch-sm-off" size="sm" />
      sm · off
    </Label>
    <Label htmlFor="preview-switch-md-on" style={{ gap: 8 }}>
      <Switch id="preview-switch-md-on" defaultChecked />
      default · on
    </Label>
    <Label htmlFor="preview-switch-md-off" style={{ gap: 8 }}>
      <Switch id="preview-switch-md-off" />
      default · off
    </Label>
  </div>
);

export const DisabledAndInvalid = () => (
  <div style={{ display: "grid", gap: 14, width: 400 }}>
    <Row
      id="preview-switch-disabled-on"
      label="Interactive"
      hint="Forced on for Terminal-only images."
    >
      <Switch id="preview-switch-disabled-on" defaultChecked disabled />
    </Row>
    <Row
      id="preview-switch-disabled-off"
      label="Auto update"
      hint="Unavailable while build-01 is disconnected."
    >
      <Switch id="preview-switch-disabled-off" disabled />
    </Row>
    <Row
      id="preview-switch-invalid"
      label="Budget enforcement"
      hint="Needs a budget rule before it can be turned off."
    >
      <Switch id="preview-switch-invalid" aria-invalid defaultChecked />
    </Row>
  </div>
);
