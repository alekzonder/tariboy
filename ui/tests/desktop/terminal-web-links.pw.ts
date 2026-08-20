import { readFile } from "node:fs/promises";

import { expect, test, waitForMainWindow } from "./fixture";
import type { W3CClient } from "./w3c";

// The gesture needs a live terminal with real link text in it. The stub harness
// holds its tmux pane open for the whole test, and the text under test is typed
// by the operator through Compose and echoed back by the pane's line discipline
// — so nothing about the harness or the daemon has to know about this test.
const AGENT = "web-links-e2e";
const HARNESS_HOLD_SECONDS = 240;

// One rendered line carrying all three cases. Kept short so it cannot wrap: a
// wrapped line would split the link across two xterm rows and the DOM range
// below would no longer describe one clickable rectangle.
const WEB_LINK = "https://example.test/terminal-link";
const FILE_TEXT = "file:///tmp/nope-file";
const PATH_TEXT = "/tmp/nope-path";
const TERMINAL_LINE = `${WEB_LINK} ${FILE_TEXT} ${PATH_TEXT}`;

// WebDriver's code point for the Meta (Command) key.
const META_KEY = "\uE03D";
// Typed after a plain click to prove the click left xterm's own input working.
const INPUT_MARKER = "zqx";

const SURFACE = '[data-terminal-surface="standalone"]';

interface Point {
  x: number;
  y: number;
}

/** Every accepted open, one serialized URL per line. Missing file reads as no
 * record at all: before the fixture creates it there is nothing to observe, and
 * that is exactly what the first case must be able to report. */
async function recordedOpens(path: string): Promise<string[]> {
  const text = await readFile(path, "utf8").catch(() => "");
  return text.split("\n").filter((line) => line.length > 0);
}

async function clickButton(desktop: W3CClient, label: string): Promise<void> {
  const button = await desktop.findElement("xpath", `//button[normalize-space(.)='${label}']`);
  await desktop.elementClick(button);
}

/** Write text into the live pane through the operator's own Compose path. The
 * pane is sleeping, so the pty's line discipline echoes the text straight back
 * and xterm renders it. No trailing Enter is sent. */
async function injectTerminalText(desktop: W3CClient, text: string): Promise<void> {
  await clickButton(desktop, "Compose");
  const textarea = await desktop.findElement("css selector", '[aria-label="Text to inject"]');
  await desktop.elementSendKeys(textarea, text);
  await clickButton(desktop, "Send");
}

async function terminalText(desktop: W3CClient): Promise<string> {
  return desktop.execute<string>(`
    const rows = document.querySelector('${SURFACE} .xterm-rows');
    return rows ? rows.textContent : "";
  `);
}

/** Viewport centre of a rendered substring inside the terminal, resolved with a
 * DOM range over the row that carries it. Returns null while the text is not
 * rendered, or when it straddles two rows. */
async function locate(desktop: W3CClient, needle: string): Promise<Point | null> {
  return desktop.execute<Point | null>(`
    const needle = arguments[0];
    const rows = document.querySelector('${SURFACE} .xterm-rows');
    if (!rows) return null;
    for (const row of rows.children) {
      const walker = document.createTreeWalker(row, NodeFilter.SHOW_TEXT);
      const nodes = [];
      let text = "";
      for (let node = walker.nextNode(); node; node = walker.nextNode()) {
        nodes.push({ node, start: text.length });
        text += node.data;
      }
      const index = text.indexOf(needle);
      if (index < 0) continue;
      const at = (offset) => {
        let found = nodes[0];
        for (const entry of nodes) if (entry.start <= offset) found = entry;
        return { node: found.node, offset: offset - found.start };
      };
      const start = at(index);
      const end = at(index + needle.length - 1);
      const range = document.createRange();
      range.setStart(start.node, start.offset);
      range.setEnd(end.node, end.offset + 1);
      const rect = range.getBoundingClientRect();
      if (rect.width === 0 || rect.height === 0) return null;
      return { x: Math.round(rect.left + rect.width / 2), y: Math.round(rect.top + rect.height / 2) };
    }
    return null;
  `, [needle]);
}

async function pointOf(desktop: W3CClient, needle: string): Promise<Point> {
  const point = await expect.poll(async () => locate(desktop, needle), {
    timeout: 30_000,
  }).not.toBeNull().then(async () => locate(desktop, needle));
  if (!point) throw new Error(`terminal text is not rendered on a single row: ${needle}`);
  return point;
}

/** A cell far from the text under test, used to force a cell transition before
 * the pointer lands on the target.
 *
 * The point has to hit `.xterm-screen` itself. xterm registers its mousemove
 * listener on the screen element (Linkifier.ts:55, constructed with
 * `this.screenElement` at CoreBrowserTerminal.ts:491), so a hop to a point
 * outside it moves the pointer without xterm ever seeing the move, and
 * `_lastBufferCell` keeps whatever the previous gesture left there. A corner of
 * the screen rectangle is not automatically such a point. Measured once on the
 * e2e surface (1156x593), `.xterm-screen` and `.xterm-rows` reported the same
 * rectangle, and its bottom edge sat 4px below the bottom of the
 * `overflow-hidden` `data-terminal-surface` ancestor, so the earlier park point
 * — 4px above that rectangle's bottom — hit-tested off the terminal. Those are
 * numbers from one observation at one window size, not a property of xterm, so
 * nothing here depends on them: candidates are anchored on the target's own row
 * and hit-tested with `elementFromPoint`, and the first one that really lands on
 * the screen wins. That pair — the row anchor and the hit test — is what fixed
 * the hop; reading the rectangle from the screen element rather than the row
 * container is not, on its own, a behavioural change. */
async function parkPoint(desktop: W3CClient, target: Point): Promise<Point> {
  const find = async (): Promise<Point | null> => desktop.execute<Point | null>(`
    const target = arguments[0];
    const screen = document.querySelector('${SURFACE} .xterm-screen');
    if (!screen) return null;
    const rect = screen.getBoundingClientRect();
    if (rect.width === 0 || rect.height === 0) return null;
    // Anchored on the target's own row, which is known to be hit-testable. In
    // the single measurement described above — one window size — only the
    // screen element's BOTTOM edge fell outside the clipping ancestor; its
    // left, right and top edges sat inside it. So a bottom-anchored corner is
    // the one candidate shape worth avoiding, and none of these use one.
    const candidates = [
      { x: rect.right - 8, y: target.y },
      { x: rect.left + 8, y: target.y },
      { x: target.x, y: target.y + 60 },
      { x: target.x, y: rect.top + 8 },
    ];
    for (const candidate of candidates) {
      const point = { x: Math.round(candidate.x), y: Math.round(candidate.y) };
      if (Math.abs(point.x - target.x) < 24 && Math.abs(point.y - target.y) < 24) continue;
      const hit = document.elementFromPoint(point.x, point.y);
      if (hit && (hit === screen || screen.contains(hit))) return point;
    }
    return null;
  `, [target]);
  // Polled, not paused: right after the route switch the hit test can land on
  // the surrounding container for every candidate while the pane is still being
  // laid out, and that was measured failing the whole spec on the first look.
  for (let attempt = 0; attempt < 20; attempt += 1) {
    const point = await find();
    if (point) return point;
    await new Promise((resolveWait) => setTimeout(resolveWait, 100));
  }
  throw new Error("no park cell lands on the terminal screen element");
}

/** Move the pointer, on its own, to one viewport point. */
async function movePointerTo(desktop: W3CClient, point: Point): Promise<void> {
  await desktop.performActions([
    {
      type: "pointer",
      id: "mouse",
      parameters: { pointerType: "mouse" },
      actions: [
        { type: "pointerMove", duration: 0, origin: "viewport", x: point.x, y: point.y },
        { type: "pause", duration: 50 },
      ],
    },
  ]);
}

/** True while xterm holds a resolved link under the pointer. Linkifier sets
 * `_currentLink` and adds `xterm-cursor-pointer` in the same step
 * (Linkifier.ts:265-273 and _linkHover at :326-334), and `_handleMouseUp`
 * returns without activating anything unless `_currentLink` is set
 * (Linkifier.ts:220-232) — so this class is a direct read of the precondition
 * the gesture depends on, not a proxy for it. */
async function linkIsHot(desktop: W3CClient): Promise<boolean> {
  return desktop.execute<boolean>(`
    const surface = document.querySelector('${SURFACE}');
    if (!surface) return false;
    return surface.matches('.xterm-cursor-pointer')
      || surface.querySelector('.xterm-cursor-pointer') !== null;
  `);
}

/** Put the pointer on the target with a link actually resolved under it.
 *
 * The park hop is an INTRA-spec measure. This one spec drives four gestures
 * through one WebDriver session and case 2 clicks the same point case 1 clicked,
 * so a later gesture can arrive with the pointer already on the target cell —
 * and `_handleMouseMove` only calls `_handleHover` when the cell differs from
 * `_lastBufferCell` (Linkifier.ts:83-85), a field the mouseleave handler does
 * not clear (:51-53). Hopping through a far cell first forces the transition —
 * but only if that cell really lands on the element xterm listens on, which is
 * what {@link parkPoint} hit-tests for; a hop that misses it is inert and leaves
 * `_lastBufferCell` exactly as the previous gesture left it. That loss mode
 * cannot explain a failure of the FIRST gesture: a freshly constructed terminal
 * has `_lastBufferCell` undefined, so the guard is true and `_handleHover` runs
 * unconditionally.
 *
 * Two further loss modes can leave xterm with no `_currentLink` after a move,
 * and both are re-driven here rather than waited out:
 *   - the renderer may not have a valid char size yet, in which case
 *     `_positionFromMouseEvent` is undefined and the move returns before
 *     resolving anything (Mouse.ts:33-37, Linkifier.ts:63-65).
 *   - a redraw of that row clears the resolved link through
 *     `onRenderedViewportChange` (Linkifier.ts:301-321); the pane under test is
 *     live, so a redraw can land inside the gesture.
 * Both are timing-shaped rather than order-shaped, which is why a predecessor
 * spec can change the outcome without carrying any state into this one: the
 * desktop fixture is per test (fixture.ts closes `desktopWorker` with
 * `{ scope: "test" }`, and every spec holds exactly one `test(`), so a
 * predecessor changes machine load and warm-up, nothing else.
 *
 * Waiting on the observed state instead of a fixed pause is what makes the
 * press deterministic. The returned boolean is the observed outcome, and the
 * caller asserts it wherever a link was expected — a hover that never resolved
 * must not be able to pass silently. */
async function hoverAt(desktop: W3CClient, point: Point, expectLink: boolean): Promise<boolean> {
  const park = await parkPoint(desktop, point);
  const attempts = expectLink ? 10 : 1;
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    await movePointerTo(desktop, park);
    await movePointerTo(desktop, point);
    if (!expectLink) return false;
    for (let poll = 0; poll < 10; poll += 1) {
      if (await linkIsHot(desktop)) return true;
      await new Promise((resolveWait) => setTimeout(resolveWait, 100));
    }
  }
  return false;
}

/** A real pointer click through the WebDriver input source, optionally with
 * Meta held for the whole gesture. xterm resolves the link on mouse move and
 * activates it on mouse up, so the move, the press and the release all have to
 * be delivered by the driver at the same viewport point. The move is a separate,
 * verified step (see hoverAt); WebDriver keeps the pointer where that step left
 * it, so the press below lands on the same cell. */
async function clickAt(
  desktop: W3CClient,
  point: Point,
  meta: boolean,
  expectLink = true,
): Promise<boolean> {
  // Input state — pointer position and any still-depressed key — outlives a
  // performActions call and is only dropped by releaseActions. Four gestures run
  // through this one session, case 1 holds Meta for the whole gesture, and case
  // 2 clicks the point case 1 clicked, so each gesture starts from released
  // state to keep the previous one out of it. Nothing crosses a spec boundary
  // either way: the desktop fixture is per test.
  await desktop.releaseActions();
  const hot = await hoverAt(desktop, point, expectLink);
  const pointerActions = [
    { type: "pointerDown", button: 0 },
    { type: "pause", duration: 50 },
    { type: "pointerUp", button: 0 },
    { type: "pause", duration: 300 },
  ];
  const pointer = {
    type: "pointer",
    id: "mouse",
    parameters: { pointerType: "mouse" },
    actions: meta
      ? [{ type: "pause", duration: 0 }, ...pointerActions, { type: "pause", duration: 0 }]
      : pointerActions,
  };
  if (!meta) {
    await desktop.performActions([pointer]);
    return hot;
  }
  await desktop.performActions([
    {
      type: "key",
      id: "keyboard",
      actions: [
        { type: "keyDown", value: META_KEY },
        ...pointerActions.map(() => ({ type: "pause", duration: 0 })),
        { type: "keyUp", value: META_KEY },
      ],
    },
    pointer,
  ]);
  return hot;
}

async function typeMarker(desktop: W3CClient, marker: string): Promise<void> {
  await desktop.performActions([
    {
      type: "key",
      id: "keyboard",
      actions: [...marker].flatMap((character) => [
        { type: "keyDown", value: character },
        { type: "keyUp", value: character },
      ]),
    },
  ]);
}

/** Give a rejected gesture time to reach the native boundary before claiming it
 * never did. Without this an assertion of "nothing was recorded" would also
 * hold while the record was still in flight. */
async function settle(): Promise<void> {
  await new Promise((resolveWait) => setTimeout(resolveWait, 2_000));
}

test("Command-click opens only explicit web links from the real terminal", async ({ desktop, desktopWorker }) => {
  test.setTimeout(240_000);
  const observationPath = desktopWorker.externalOpenLog;
  await waitForMainWindow(desktop);

  await desktop.execute(`
    window.__webLinksSetup = "running";
    window.__TAURI_INTERNALS__.invoke("daemon_status")
      .then(async (status) => {
        window.__webLinksBaseURL = status.base_url;
        window.__webLinksStatus = { base_dir: status.base_dir, base_url: status.base_url };
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
        window.__webLinksSetup = "ready";
      })
      .catch((error) => { window.__webLinksSetup = "error: " + String(error); });
    return true;
  `);
  await expect.poll(() => desktop.execute<string>("return window.__webLinksSetup || '';"), {
    timeout: 60_000,
  }).toBe("ready");

  // CLAUDE.md isolation: this run must drive the fixture's throwaway daemon and
  // write its observations below the fixture's own temp root — never the
  // operator's live daemon on 127.0.0.1:9990 under ~/.tariboy.
  const isolation = await desktop.execute<{ base_dir: string; base_url: string }>(
    "return window.__webLinksStatus;",
  );
  expect(isolation.base_dir).toContain("tariboy-desktop-e2e-");
  expect(isolation.base_dir).not.toContain("/.tariboy");
  expect(isolation.base_url).not.toContain(":9990");

  await desktop.execute(
    `window.location.hash = ${JSON.stringify(`#/agents/local/${AGENT}/console`)}; return true;`,
  );
  await expect.poll(async () => desktop.execute<boolean>(
    `return document.querySelector('${SURFACE} .xterm-rows') !== null;`,
  ), { timeout: 60_000 }).toBe(true);

  await injectTerminalText(desktop, TERMINAL_LINE);
  await expect.poll(async () => terminalText(desktop), { timeout: 30_000 }).toContain(WEB_LINK);

  await test.step("case 1 - a Command-click on an https link records exactly one open", async () => {
    expect(await recordedOpens(observationPath)).toEqual([]);
    const resolved = await clickAt(desktop, await pointOf(desktop, WEB_LINK), true);
    // The gesture is only meaningful if xterm had the link resolved under the
    // pointer when the press landed; without this a hover that never arrived
    // would be indistinguishable from one that did.
    expect(resolved, "xterm resolved the link under the pointer for case 1").toBe(true);
    await expect.poll(async () => recordedOpens(observationPath), { timeout: 20_000 })
      .toEqual([WEB_LINK]);
  });

  await test.step("case 2 - an ordinary click on the same link records nothing and leaves xterm usable", async () => {
    const resolved = await clickAt(desktop, await pointOf(desktop, WEB_LINK), false);
    // Without a link resolved under the pointer this case would pass for the
    // wrong reason: a click that lands on no link records nothing either, and
    // the INPUT_MARKER control below only proves the terminal is alive.
    expect(resolved, "xterm resolved the link under the pointer for case 2").toBe(true);
    await settle();
    expect(await recordedOpens(observationPath)).toEqual([WEB_LINK]);

    // The click must have landed on a working terminal, otherwise "nothing was
    // recorded" would hold for the wrong reason.
    await typeMarker(desktop, INPUT_MARKER);
    await expect.poll(async () => terminalText(desktop), { timeout: 30_000 }).toContain(INPUT_MARKER);
  });

  await test.step("case 3 - Command-clicking a file URL or a bare path records nothing", async () => {
    // Neither of these is a web link the app will open, so there is no hover
    // state to wait for — the point of the case is that the gesture lands and
    // records nothing.
    await clickAt(desktop, await pointOf(desktop, FILE_TEXT), true, false);
    await clickAt(desktop, await pointOf(desktop, PATH_TEXT), true, false);
    await settle();
    expect(await recordedOpens(observationPath)).toEqual([WEB_LINK]);
  });

  await desktop.execute(`
    fetch(window.__webLinksBaseURL + "/api/agents/" + ${JSON.stringify(AGENT)} + "/stop", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: "{}",
    }).catch(() => {});
    return true;
  `);
});
