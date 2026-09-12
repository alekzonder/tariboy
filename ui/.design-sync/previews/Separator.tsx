import { Badge, Separator } from "tariboy-ui";

const panel: React.CSSProperties = {
  width: 400,
  border: "1px solid var(--border)",
  borderRadius: 10,
  padding: 12,
  display: "grid",
  gap: 10,
  fontSize: 13,
};

const muted: React.CSSProperties = {
  color: "var(--muted-foreground)",
  fontSize: 12,
};

export const SectionDivider = () => (
  <div style={panel}>
    <div>
      <div style={{ fontWeight: 500 }}>builder</div>
      <div style={muted}>worker:v2 · This daemon (local)</div>
    </div>
    <Separator />
    <div>
      <div style={{ fontWeight: 500 }}>Current goal</div>
      <div style={muted}>TB-142 — split the packager image cache</div>
    </div>
    <Separator />
    <div>
      <div style={{ fontWeight: 500 }}>Budget</div>
      <div style={muted}>$4.10 of $20.00 used in the last 24h</div>
    </div>
  </div>
);

export const VerticalInlineMeta = () => (
  <div
    style={{
      display: "flex",
      alignItems: "center",
      gap: 10,
      fontSize: 13,
      border: "1px solid var(--border)",
      borderRadius: 10,
      padding: "8px 12px",
      width: "fit-content",
    }}
  >
    <span style={{ fontWeight: 500 }}>reviewer</span>
    <Separator orientation="vertical" style={{ height: 16 }} />
    <span style={muted}>worker:v1</span>
    <Separator orientation="vertical" style={{ height: 16 }} />
    <span style={muted}>build-01</span>
    <Separator orientation="vertical" style={{ height: 16 }} />
    <Badge variant="secondary">stopped</Badge>
  </div>
);

export const ListGroups = () => (
  <div style={{ ...panel, gap: 0, padding: 0, overflow: "hidden" }}>
    <div style={{ padding: "8px 12px", ...muted }}>Hosts</div>
    <Separator />
    <div style={{ padding: "8px 12px", fontWeight: 500 }}>
      This daemon (local)
    </div>
    <div style={{ padding: "8px 12px", fontWeight: 500 }}>build-01</div>
    <Separator />
    <div style={{ padding: "8px 12px", ...muted }}>Images</div>
    <Separator />
    <div style={{ padding: "8px 12px", fontWeight: 500 }}>worker:v2</div>
    <div style={{ padding: "8px 12px", fontWeight: 500 }}>bare:latest</div>
  </div>
);

export const DialogFooterRule = () => (
  <div style={{ ...panel, gap: 12 }}>
    <div>
      <div style={{ fontWeight: 500 }}>Remove worker:v1</div>
      <div style={muted}>
        Two agents still reference this image and will fail to restart.
      </div>
    </div>
    <Separator />
    <div style={{ display: "flex", justifyContent: "flex-end", gap: 8, ...muted }}>
      <span>Esc to cancel</span>
      <Separator orientation="vertical" style={{ height: 14 }} />
      <span>Enter to remove</span>
    </div>
  </div>
);
