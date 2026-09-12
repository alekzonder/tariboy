import { DaemonBanner } from "tariboy-ui";

// DaemonBanner picks its source of truth from the environment: in the desktop
// app Rust pushes daemon lifecycle transitions, in a browser the only signal is
// whether GET /api/status answers. A preview is a browser, so the BrowserBanner
// branch runs, its 2 s poll fails against a page with no daemon behind it, and
// the component renders the error bar it really ships — full-bleed destructive
// tone, centered copy.
//
// The desktop branch (starting / failed / down / version-mismatch bars, each
// with its Retry / Start / Open log actions) needs __TAURI_INTERNALS__ and a
// live Rust bridge and cannot be reached here; see the learnings file.

export const Unreachable = () => (
  <div style={{ width: 720 }}>
    <DaemonBanner />
  </div>
);

// The bar as the app stacks it: directly under the title bar, above the routed
// view. The frame is preview scaffolding; the bar is the shipped component.
export const UnderAppChrome = () => (
  <div
    style={{
      width: 720,
      border: "1px solid var(--border)",
      borderRadius: "var(--radius)",
      overflow: "hidden",
      background: "var(--background)",
    }}
  >
    <div
      style={{
        display: "flex",
        alignItems: "center",
        height: 48,
        padding: "0 16px",
        borderBottom: "1px solid var(--border)",
        fontSize: 14,
        color: "var(--muted-foreground)",
      }}
    >
      Tariboy
    </div>
    <DaemonBanner />
    <div style={{ padding: 24, fontSize: 14, color: "var(--muted-foreground)" }}>
      workspace
    </div>
  </div>
);
