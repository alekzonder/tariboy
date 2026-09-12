import { AppSettings, DesktopUpdatesProvider, MemoryRouter } from "tariboy-ui";

// AppSettings is the /app-settings route: the Desktop updates card plus the way
// back to the workspace. It reads DesktopUpdatesProvider's context (and throws
// without it), and the provider derives `desktop` from __TAURI_INTERNALS__, so
// in a browser — which is what a preview is — the card renders its real
// non-desktop shape: current version "—", the auto-download switch disabled at
// its default-on position, and the "updates are Desktop-only" status line in
// place of the check/download button.
//
// The desktop shape (the enabled "check and download" button, the progress bar,
// the request-error alert) needs the Tauri bridge and cannot be reached from a
// browser preview; see .design-sync/learnings/applied-c.md.

export const BrowserFallback = () => (
  <MemoryRouter>
    <DesktopUpdatesProvider>
      <div style={{ width: 820, height: 460, overflow: "hidden" }}>
        <AppSettings />
      </div>
    </DesktopUpdatesProvider>
  </MemoryRouter>
);
