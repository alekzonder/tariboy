import { expect, test, waitForMainWindow } from "./fixture";
import type { W3CClient, W3CElement } from "./w3c";

// A live tmux pane is required for the scrollback toolbar to render at all, so
// the agent runs the stub harness with a long sleep: the iteration keeps its
// tmux session attached for the whole test instead of exiting immediately.
const AGENT = "scrollback-e2e";
const HARNESS_HOLD_SECONDS = 120;

// The raw bytes TuiScreen writes to the terminal socket for copy-mode. Spelled
// out here rather than imported so the e2e asserts the wire bytes independently
// of the production constant.
const ENTER_COPY_MODE = "\x02[";
const PAGE_BACK = "\x1b[5~";
const PAGE_FORWARD = "\x1b[6~";
const EXIT_COPY_MODE = "q";
const ENTER_KEY = "\r";

/** Record every websocket frame the UI sends, so the test can assert the exact
 * copy-mode bytes rather than trusting a label. Terminal keystrokes go out as
 * binary UTF-8 frames, so they are decoded back to text here. Installed before
 * any terminal socket is dialled. */
async function recordSocketWrites(desktop: W3CClient): Promise<void> {
  await desktop.execute(`
    window.__terminalSends = [];
    if (!window.__terminalSendsPatched) {
      window.__terminalSendsPatched = true;
      const original = WebSocket.prototype.send;
      const decoder = new TextDecoder();
      WebSocket.prototype.send = function (data) {
        window.__terminalSends.push(typeof data === "string" ? data : decoder.decode(data));
        return original.call(this, data);
      };
    }
    return true;
  `);
}

/** Frames the terminal toolbar produced, with the {cols,rows} resize frames
 * dropped — those belong to the untouched FitAddon path. */
async function keySends(desktop: W3CClient): Promise<string[]> {
  const frames = await desktop.execute<string[]>(`return window.__terminalSends || [];`);
  return frames.filter((frame) => {
    if (frame.startsWith("{")) return false;
    // xterm answers device-attribute and foreground/background colour probes
    // over the same socket as operator input. Those replies can arrive after
    // resetSends on a slower WebView and are not terminal-toolbar keystrokes.
    const csiDeviceReply = frame.charCodeAt(0) === 0x1b
      && frame[1] === "["
      && (frame[2] === "?" || frame[2] === ">")
      && frame.endsWith("c");
    const oscColourReply = frame.charCodeAt(0) === 0x1b
      && (frame.startsWith("\x1b]10;rgb:") || frame.startsWith("\x1b]11;rgb:"))
      && frame.charCodeAt(frame.length - 1) === 0x5c;
    return !csiDeviceReply && !oscColourReply;
  });
}

async function resetSends(desktop: W3CClient): Promise<void> {
  await desktop.execute(`window.__terminalSends = []; return true;`);
}

/** Click a button by its visible label, waiting for it to be rendered first.
 *
 * WebDriver's findElement does not retry: called in the same breath as a view
 * switch it either wins the race with React's render or fails outright with
 * 404 "no such element". Every lookup in this file therefore polls the DOM
 * from the page itself and only then asks the driver for the element. */
async function clickButton(desktop: W3CClient, label: string): Promise<void> {
  await expect.poll(async () => desktop.execute<boolean>(`
    return [...document.querySelectorAll("button")].some(
      (candidate) => (candidate.textContent || "").replace(/\\s+/g, " ").trim() === ${JSON.stringify(label)},
    );
  `), { timeout: 60_000 }).toBe(true);
  const button = await desktop.findElement("xpath", `//button[normalize-space(.)='${label}']`);
  await desktop.elementClick(button);
}

/** Expand the agent sidebar if a previous spec left it collapsed.
 *
 * The sidebar is rendered conditionally on `sidebar.hidden`, and that flag is
 * persisted in the webview's localStorage — which outlives a spec boundary,
 * unlike the per-test daemon base dir. titlebar-drag runs right before this
 * file and toggles the control, so the sidebar can be gone before we start.
 * That spec already accepts either initial label; this one does the same
 * instead of depending on the visibility it inherits. */
async function ensureSidebarShown(desktop: W3CClient): Promise<void> {
  const toggleLabel = async () => desktop.execute<string>(`
    const toggle = document.querySelector('header button[aria-label$="agents"]');
    return toggle ? toggle.getAttribute("aria-label") : "no toggle";
  `);
  await expect.poll(toggleLabel, { timeout: 60_000 }).not.toBe("no toggle");
  if (await toggleLabel() === "Show agents") {
    const toggle = await desktop.findElement(
      "css selector",
      'header button[aria-label$="agents"]',
    );
    await desktop.elementClick(toggle);
    await expect.poll(toggleLabel, { timeout: 60_000 }).toBe("Hide agents");
  }
}

/** Wait for an agent's sidebar entry and hand back its element.
 *
 * The sidebar is fed by a 3s poll of the aggregate agents list, so an agent
 * created over HTTP mid-test appears there a tick later than it exists on the
 * daemon. The poll reports what it actually saw, so a timeout distinguishes
 * "the entry is merely late" from "there is no sidebar at all" or "the sidebar
 * lists other agents but not this one" without extra instrumentation. */
async function awaitSidebarEntry(desktop: W3CClient, agent: string): Promise<W3CElement> {
  const selector = `[aria-label="Open ${agent}"]`;
  await ensureSidebarShown(desktop);
  await expect.poll(async () => desktop.execute<string>(`
    if (document.querySelector(${JSON.stringify(selector)})) return "ready";
    if (!document.querySelector("aside")) return "no sidebar";
    const listed = [...document.querySelectorAll('aside [aria-label^="Open "]')]
      .map((entry) => entry.getAttribute("aria-label"))
      .join(", ");
    return "sidebar without the entry: " + (listed || "(no entries)");
  `), { timeout: 60_000 }).toBe("ready");
  return desktop.findElement("css selector", selector);
}

async function toolbarText(desktop: W3CClient): Promise<string> {
  return desktop.execute<string>(`
    const toolbar = document.querySelector('[data-testid="terminal-toolbar"]');
    return toolbar ? toolbar.innerText : "";
  `);
}

/** Wait until a live terminal of the given surface has mounted its toolbar. */
async function awaitLiveTerminal(desktop: W3CClient, surface: string): Promise<void> {
  await expect.poll(async () => desktop.execute<string>(`
    const screen = document.querySelector('[data-terminal-surface="${surface}"]');
    const toolbar = document.querySelector('[data-testid="terminal-toolbar"]');
    if (!screen || !toolbar) return "no live terminal";
    return toolbar.innerText.includes("Scrollback") ? "ready" : "no scrollback control";
  `), { timeout: 60_000 }).toBe("ready");
}

/** Drive one terminal surface through the whole scrollback cycle. */
async function exerciseScrollback(desktop: W3CClient, surface: string): Promise<void> {
  await awaitLiveTerminal(desktop, surface);

  // Live mode: no scrollback state is claimed before the operator asks for it.
  const live = await toolbarText(desktop);
  expect(live).not.toContain("Viewing scrollback");
  expect(live).not.toContain("Exit scrollback");

  await resetSends(desktop);
  await clickButton(desktop, "Scrollback");
  await expect.poll(async () => toolbarText(desktop)).toContain("Viewing scrollback");
  const browsing = await toolbarText(desktop);
  expect(browsing).toContain("Exit scrollback");
  expect(browsing).toContain("Page back");
  expect(browsing).toContain("Page forward");

  await clickButton(desktop, "Page back");
  await clickButton(desktop, "Page forward");

  // The inner harness mouse path stays untouched: a wheel over the terminal is
  // not translated into any frame, and no tmux mouse-mode sequence is written.
  const beforeWheel = await keySends(desktop);
  await desktop.execute(`
    const screen = document.querySelector('[data-terminal-surface="${surface}"]');
    screen.dispatchEvent(new WheelEvent("wheel", { deltaY: -240, bubbles: true, cancelable: true }));
    return true;
  `);
  expect(await keySends(desktop)).toEqual(beforeWheel);

  await clickButton(desktop, "Exit scrollback");
  await expect.poll(async () => toolbarText(desktop)).not.toContain("Viewing scrollback");
  const returned = await toolbarText(desktop);
  expect(returned).not.toContain("Exit scrollback");

  // Back on live output: an ordinary hotkey reaches the session again.
  await clickButton(desktop, "Enter");

  expect(await keySends(desktop)).toEqual([
    ENTER_COPY_MODE,
    PAGE_BACK,
    PAGE_FORWARD,
    EXIT_COPY_MODE,
    ENTER_KEY,
  ]);
}

test("the shared terminal toolbar browses tmux scrollback on Console and Workspace", async ({ desktop }) => {
  test.setTimeout(180_000);
  await waitForMainWindow(desktop);
  await recordSocketWrites(desktop);

  await desktop.execute(`
    window.__scrollbackSetup = "running";
    window.__TAURI_INTERNALS__.invoke("daemon_status")
      .then(async (status) => {
        window.__scrollbackBaseURL = status.base_url;
        window.__scrollbackStatus = { base_dir: status.base_dir, base_url: status.base_url };
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
        await call("POST", "/api/agents", {
          image: "basic:latest", name: ${JSON.stringify(AGENT)}, harness: "stub",
          interactive: true, loop: false, env: "STUB_SLEEP=${HARNESS_HOLD_SECONDS},STUB_CALL_DONE=0",
        });
        await call("POST", "/api/agents/" + ${JSON.stringify(AGENT)} + "/start", {});
        window.__scrollbackSetup = "ready";
      })
      .catch((error) => { window.__scrollbackSetup = "error: " + String(error); });
    return true;
  `);
  await expect.poll(() => desktop.execute<string>("return window.__scrollbackSetup || '';"), {
    timeout: 60_000,
  }).toBe("ready");

  // CLAUDE.md isolation: this run must drive the fixture's throwaway daemon, not
  // the operator's live one on 127.0.0.1:9990 under ~/.tariboy.
  const isolation = await desktop.execute<{ base_dir: string; base_url: string }>(
    "return window.__scrollbackStatus;",
  );
  expect(isolation.base_dir).toContain("tariboy-desktop-e2e-");
  expect(isolation.base_dir).not.toContain("/.tariboy");
  expect(isolation.base_url).not.toContain(":9990");

  // Standalone Console surface.
  await desktop.execute(
    `window.location.hash = ${JSON.stringify(`#/agents/local/${AGENT}/console`)}; return true;`,
  );
  await exerciseScrollback(desktop, "standalone");

  // The same shared component embedded in a Workspace tile.
  const workspaceLink = await desktop.findElement(
    "css selector",
    'header a[href$="/workspace"]',
  );
  await desktop.elementClick(workspaceLink);
  await expect.poll(() => desktop.execute<string>("return window.location.hash;"))
    .toBe("#/workspace");
  const sidebarEntry = await awaitSidebarEntry(desktop, AGENT);
  await desktop.elementClick(sidebarEntry);
  await exerciseScrollback(desktop, "workspace");

  await desktop.execute(`
    fetch(window.__scrollbackBaseURL + "/api/agents/" + ${JSON.stringify(AGENT)} + "/stop", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: "{}",
    }).catch(() => {});
    return true;
  `);
});
