import { expect, test, waitForMainWindow } from "./fixture";
import type { W3CClient } from "./w3c";
import { WORKSPACE_STATE_KEY } from "@/pages/terminals/workspaceState";

// The customer sees the failure in the packaged desktop app, so the gesture is
// driven here exactly as a person drives it: WebDriver pointer actions on the
// real WebKit surface, no synthetic DragEvent and no direct call into the layout
// model. Two stub agents are enough — the bug is in where a dragged tab is
// dropped, not in what the terminal shows.
const ALPHA = "ws-split-alpha";
const BETA = "ws-split-beta";
const HARNESS_HOLD_SECONDS = 240;

interface Rect {
  x: number;
  y: number;
  width: number;
  height: number;
}

async function rectOf(desktop: W3CClient, selector: string): Promise<Rect | null> {
  return desktop.execute<Rect | null>(`
    const element = document.querySelector(arguments[0]);
    if (!element) return null;
    const rect = element.getBoundingClientRect();
    if (rect.width === 0 || rect.height === 0) return null;
    return { x: rect.left, y: rect.top, width: rect.width, height: rect.height };
  `, [selector]);
}

/** Rectangle of the pane (flexlayout tabset) that carries one terminal. The tab
 * handle is the only per-terminal anchor inside a pane, so the pane is resolved
 * from it upwards. */
async function paneRect(desktop: W3CClient, agent: string): Promise<Rect | null> {
  return desktop.execute<Rect | null>(`
    const handle = document.querySelector('[data-testid="workspace-drag-' + arguments[0] + '"]');
    const pane = handle && handle.closest(".flexlayout__tabset");
    if (!pane) return null;
    const rect = pane.getBoundingClientRect();
    if (rect.width === 0 || rect.height === 0) return null;
    return { x: rect.left, y: rect.top, width: rect.width, height: rect.height };
  `, [agent]);
}

/** Everything needed to explain a layout that came out wrong: the panes, the
 * tabs inside them, the workspace box and the sidebar rows the drags started
 * from. Only read when an assertion has already failed. */
async function layoutDump(desktop: W3CClient): Promise<unknown> {
  return desktop.execute<unknown>(`
    const box = (element) => {
      const rect = element.getBoundingClientRect();
      return { x: Math.round(rect.left), y: Math.round(rect.top), w: Math.round(rect.width), h: Math.round(rect.height) };
    };
    return {
      viewport: { w: window.innerWidth, h: window.innerHeight },
      workspace: (() => {
        const root = document.querySelector('[data-testid="terminal-workspace"]');
        return root ? box(root) : null;
      })(),
      tabsets: Array.from(document.querySelectorAll(".flexlayout__tabset")).map((pane) => ({
        box: box(pane),
        tabs: Array.from(pane.querySelectorAll("[data-workspace-node-id]"))
          .map((tab) => (tab.textContent || "").trim()),
      })),
      rows: Array.from(document.querySelectorAll('[aria-label^="Open "]'))
        .map((row) => ({ label: row.getAttribute("aria-label"), box: box(row) })),
    };
  `);
}

async function requirePane(desktop: W3CClient, agent: string): Promise<Rect> {
  await expect.poll(async () => paneRect(desktop, agent), { timeout: 30_000 }).not.toBeNull();
  const rect = await paneRect(desktop, agent);
  if (!rect) throw new Error(`pane for ${agent} disappeared`);
  return rect;
}

/** One pointer drag: press at the source, cross the drag threshold, travel to
 * the destination, assert the dock the app decided on, release. Same shape as
 * the browser-level spec, expressed through WebDriver input sources. */
async function pointerDrag(
  desktop: W3CClient,
  from: { x: number; y: number },
  to: { x: number; y: number },
  expectedDock?: "left" | "right" | "top" | "bottom",
): Promise<void> {
  const round = (value: number): number => Math.round(value);
  await desktop.releaseActions();
  await desktop.performActions([{
    type: "pointer",
    id: "mouse",
    parameters: { pointerType: "mouse" },
    actions: [
      { type: "pointerMove", duration: 0, origin: "viewport", x: round(from.x), y: round(from.y) },
      { type: "pointerDown", button: 0 },
      { type: "pause", duration: 50 },
      { type: "pointerMove", duration: 50, origin: "viewport", x: round(from.x) + 12, y: round(from.y) },
      { type: "pointerMove", duration: 150, origin: "viewport", x: round(to.x), y: round(to.y) },
      { type: "pause", duration: 100 },
      { type: "pointerMove", duration: 50, origin: "viewport", x: round(to.x), y: round(to.y) },
      { type: "pause", duration: 100 },
    ],
  }]);

  if (expectedDock) {
    await expect.poll(async () => desktop.execute<string | null>(`
      const preview = document.querySelector('[data-testid="workspace-drop-preview"]');
      return preview ? preview.getAttribute("data-dock") : null;
    `), { timeout: 10_000 }).toBe(expectedDock);
  }

  await desktop.performActions([{
    type: "pointer",
    id: "mouse",
    parameters: { pointerType: "mouse" },
    actions: [
      { type: "pointerUp", button: 0 },
      { type: "pause", duration: 200 },
    ],
  }]);
  await expect.poll(async () => desktop.execute<number>(`
    return document.querySelectorAll('[data-testid="workspace-drop-preview"]').length;
  `), { timeout: 10_000 }).toBe(0);
}

/** Open the global canvas through the real titlebar link, as an operator does. */
async function openWorkspaceView(desktop: W3CClient): Promise<void> {
  await expect.poll(() => rectOf(desktop, 'header a[href$="/workspace"]'), {
    timeout: 60_000,
  }).not.toBeNull();
  await desktop.elementClick(await desktop.findElement(
    "css selector",
    'header a[href$="/workspace"]',
  ));
  await expect.poll(() => desktop.execute<string>("return window.location.hash;"), {
    timeout: 10_000,
  }).toBe("#/workspace");
}

async function sidebarButton(desktop: W3CClient, agent: string): Promise<Rect> {
  await expect.poll(
    async () => rectOf(desktop, `[aria-label="Open ${agent}"]`),
    { timeout: 60_000 },
  ).not.toBeNull();
  // Precaution, not a diagnosed failure: the full run reaches this spec with
  // every agent the earlier specs left behind still listed, and a pointer
  // action cannot press a row that scrolled out of the viewport. A person
  // scrolls to the row before grabbing it.
  await desktop.execute(`
    const row = document.querySelector('[aria-label="Open ' + arguments[0] + '"]');
    if (row) row.scrollIntoView({ block: "center" });
    return true;
  `, [agent]);
  const rect = await rectOf(desktop, `[aria-label="Open ${agent}"]`);
  if (!rect) throw new Error(`sidebar entry for ${agent} disappeared`);
  return rect;
}

const centre = (rect: Rect): { x: number; y: number } => ({
  x: rect.x + rect.width / 2,
  y: rect.y + rect.height / 2,
});

test("dragging a terminal onto a neighbour's top edge splits the workspace vertically", async ({ desktop }) => {
  test.setTimeout(300_000);
  await waitForMainWindow(desktop);

  // The desktop app is shared by every spec in this worker and the workspace
  // layout is persisted in localStorage, so an earlier spec can leave a pane
  // behind — and this scenario is only meaningful on an empty workspace.
  // Start from a known-empty one instead of inheriting whoever ran before.
  await desktop.execute(`
    localStorage.removeItem(${JSON.stringify(WORKSPACE_STATE_KEY)});
    location.reload();
    return true;
  `);
  await waitForMainWindow(desktop);
  expect(await desktop.execute<string | null>(
    `return localStorage.getItem(${JSON.stringify(WORKSPACE_STATE_KEY)});`,
  )).toBeNull();

  await desktop.execute(`
    window.__wsSplitSetup = "running";
    window.__TAURI_INTERNALS__.invoke("daemon_status")
      .then(async (status) => {
        window.__wsSplitStatus = { base_dir: status.base_dir, base_url: status.base_url };
        window.__wsSplitBaseURL = status.base_url;
        const call = async (method, path, body) => {
          const response = await fetch(status.base_url + path, {
            method,
            headers: body === undefined ? undefined : { "content-type": "application/json" },
            body: body === undefined ? undefined : JSON.stringify(body),
          });
          const envelope = await response.json();
          if (!response.ok || !envelope.ok) throw new Error(path + ": " + JSON.stringify(envelope.error));
          return envelope.result;
        };
        for (const name of [${JSON.stringify(ALPHA)}, ${JSON.stringify(BETA)}]) {
          await call("POST", "/api/agents", {
            image: "basic:latest", name, harness: "stub",
            interactive: true, loop: false, env: "STUB_SLEEP=${HARNESS_HOLD_SECONDS},STUB_CALL_DONE=0",
          });
          await call("POST", "/api/agents/" + name + "/start", {});
        }
        window.__wsSplitSetup = "ready";
      })
      .catch((error) => { window.__wsSplitSetup = "error: " + String(error); });
    return true;
  `);
  await expect.poll(() => desktop.execute<string>("return window.__wsSplitSetup || '';"), {
    timeout: 90_000,
  }).toBe("ready");

  // CLAUDE.md isolation: this run must drive the fixture's throwaway daemon, not
  // the operator's live one on 127.0.0.1:9990 under ~/.tariboy.
  const isolation = await desktop.execute<{ base_dir: string; base_url: string }>(
    "return window.__wsSplitStatus;",
  );
  expect(isolation.base_dir).toContain("tariboy-desktop-e2e-");
  expect(isolation.base_dir).not.toContain("/.tariboy");
  expect(isolation.base_url).not.toContain(":9990");

  // Workspace is a global route opened from the titlebar.
  await desktop.execute(`window.location.hash = "#/"; return true;`);
  await openWorkspaceView(desktop);
  await expect.poll(async () => rectOf(desktop, '[data-testid="terminal-workspace"]'), {
    timeout: 60_000,
  }).not.toBeNull();
  const workspace = await rectOf(desktop, '[data-testid="terminal-workspace"]');
  if (!workspace) throw new Error("workspace never rendered");

  await test.step("open two terminals side by side", async () => {
    await pointerDrag(desktop, centre(await sidebarButton(desktop, ALPHA)), centre(workspace));
    const alpha = await requirePane(desktop, ALPHA);

    await pointerDrag(
      desktop,
      centre(await sidebarButton(desktop, BETA)),
      { x: alpha.x + alpha.width - 4, y: alpha.y + alpha.height / 2 },
      "right",
    );
    await requirePane(desktop, BETA);
    // flexlayout applies the drop and re-measures over a couple of frames, so
    // the two panes can still report the same rect right after the release.
    // Poll: a lasting overlap means the side-by-side setup really failed.
    try {
      await expect.poll(async () => {
        const alphaAfter = await paneRect(desktop, ALPHA);
        const betaAfter = await paneRect(desktop, BETA);
        if (!alphaAfter || !betaAfter) return null;
        return alphaAfter.x < betaAfter.x;
      }, { timeout: 15_000 }).toBe(true);
    } catch (error) {
      throw new Error(
        `${String(error)}\nlayout: ${JSON.stringify(await layoutDump(desktop))}`,
        { cause: error },
      );
    }
  });

  await test.step("drag beta onto alpha's top edge", async () => {
    const alpha = await requirePane(desktop, ALPHA);
    const betaHandle = await rectOf(desktop, `[data-testid="workspace-drag-${BETA}"]`);
    expect(betaHandle).not.toBeNull();

    await pointerDrag(
      desktop,
      centre(betaHandle!),
      { x: alpha.x + alpha.width / 2, y: alpha.y + 4 },
      "top",
    );

    // The whole point of the spec: beta must end up ABOVE alpha, in the same
    // column. Before the fix the drop was a silent no-op and beta stayed to the
    // right of alpha.
    await expect.poll(async () => {
      const movedAlpha = await paneRect(desktop, ALPHA);
      const movedBeta = await paneRect(desktop, BETA);
      if (!movedAlpha || !movedBeta) return null;
      const overlap = Math.min(movedBeta.x + movedBeta.width, movedAlpha.x + movedAlpha.width)
        - Math.max(movedBeta.x, movedAlpha.x);
      return {
        stacked: movedBeta.y + movedBeta.height <= movedAlpha.y + 2,
        sameColumn: overlap > Math.min(movedBeta.width, movedAlpha.width) / 2,
      };
    }, { timeout: 15_000 }).toEqual({ stacked: true, sameColumn: true });
  });

  await desktop.execute(`
    for (const name of [${JSON.stringify(ALPHA)}, ${JSON.stringify(BETA)}]) {
      fetch(window.__wsSplitBaseURL + "/api/agents/" + name + "/stop", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: "{}",
      }).catch(() => {});
    }
    return true;
  `);
});
