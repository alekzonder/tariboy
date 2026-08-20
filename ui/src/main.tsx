import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, HashRouter } from "react-router-dom";
import { ThemeProvider } from "@/components/theme-provider";
import { isDesktop, daemonState } from "@/lib/desktop";
import { setLocalBaseURL } from "@/lib/api";
import App from "./App";
import "./index.css";

/**
 * Which hash a freshly opened desktop window should land on, or null to leave
 * the current location alone. Agents is the desktop start screen, but a
 * reload of some other route must stay where it was.
 */
export function desktopStartHash(currentHash: string): string | null {
  if (!isDesktop()) return null;
  if (currentHash !== "" && currentHash !== "#" && currentHash !== "#/") return null;
  return "#/";
}

export function configureDesktopInputAssistance(root: HTMLElement): void {
  if (!isDesktop()) return;
  root.setAttribute("spellcheck", "false");
  root.setAttribute("autocorrect", "off");
  root.setAttribute("autocapitalize", "off");
}

async function boot() {
  // HashRouter in the desktop app: Tauri's asset protocol serves files, not an
  // SPA fallback, so a reload at a nested agent route would 404 before React loads. The
  // hash is never sent to the protocol. In a browser the URLs stay clean.
  // Picked here rather than at module scope so the entry file exports no
  // component and stays fast-refresh clean.
  const Router = isDesktop() ? HashRouter : BrowserRouter;

  // Resolve the local daemon's origin BEFORE the first render so no component
  // fires a request against the wrong (or a relative, unreachable) URL. Outside
  // Tauri this resolves to null immediately and nothing changes.
  const st = await daemonState();
  if (st?.base_url) setLocalBaseURL(st.base_url);

  const start = desktopStartHash(window.location.hash);
  if (start) window.location.hash = start;

  const root = document.getElementById("root");
  if (!root) return; // no mount point (e.g. a unit test importing this module)
  configureDesktopInputAssistance(root);
  createRoot(root).render(
    <StrictMode>
      <ThemeProvider defaultTheme="system">
        <Router>
          <App />
        </Router>
      </ThemeProvider>
    </StrictMode>,
  );
}

void boot();
