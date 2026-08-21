import { expect, test, waitForMainWindow } from "./fixture";
import type { W3CClient, W3CElement } from "./w3c";

async function findEventually(
  desktop: W3CClient,
  using: "css selector" | "xpath",
  selector: string,
): Promise<W3CElement> {
  await expect.poll(async () => {
    try {
      return await desktop.findElement(using, selector);
    } catch {
      return null;
    }
  }).not.toBeNull();
  return desktop.findElement(using, selector);
}

test("selects a harness in the New agent dialog", async ({ desktop }) => {
  await waitForMainWindow(desktop);

  const openDialog = await desktop.findElement(
    "xpath",
    "//main//button[normalize-space(.)='New agent']",
  );
  await expect(desktop.elementText(openDialog)).resolves.toBe("New agent");
  await desktop.elementClick(openDialog);

  const harness = await expect.poll(async () => {
    try {
      return await desktop.findElement(
        "css selector",
        "#create-agent-harness",
      );
    } catch {
      return null;
    }
  }).not.toBeNull().then(async () => desktop.findElement("css selector", "#create-agent-harness"));

  await expect(desktop.elementProperty(harness, "value")).resolves.not.toBe("codex");
  await desktop.elementSendKeys(harness, "codex");
  await expect.poll(() => desktop.elementProperty(harness, "value")).toBe("codex");
});

test("clones complete agent configuration through the production Desktop", async ({ desktop }) => {
  await waitForMainWindow(desktop);

  await desktop.execute(`
    window.__desktopCloneSetup = "running";
    window.__TAURI_INTERNALS__.invoke("daemon_status").then(async (status) => {
      window.__desktopCloneBaseURL = status.base_url;
      const response = await fetch(status.base_url + "/api/agents", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          image: "basic:latest", name: "clone-source", cwd: "",
          harness: "codex", model: "gpt-5", effort: "high",
          interactive: false, loop: false,
          interval_s: 12, timeout_s: 34, hard_timeout_s: 56,
          on_timeout: "stop", on_error: "restart", max_idle_iterations: 7,
          user_prompt: "desktop clone prompt",
          env: { CSV: "a,b", EQ: "a=b", LINES: "one\\ntwo" },
          messages_batch: 8, messages_max_queue: 900,
          group: "desktop-clone-team", alias: "Source alias",
          notes: "desktop clone notes", color: "#123abc"
        }),
      });
      const envelope = await response.json();
      if (!response.ok || !envelope.ok) {
        throw new Error("source create failed: " + JSON.stringify(envelope.error));
      }
      window.__desktopCloneSetup = "ready";
    }).catch((error) => { window.__desktopCloneSetup = "error: " + String(error); });
    return true;
  `);
  await expect.poll(
    () => desktop.execute<string>("return window.__desktopCloneSetup || '';"),
    { timeout: 60_000 },
  ).toBe("ready");

  const sourceRow = await findEventually(
    desktop,
    "xpath",
    "//button[@aria-label='Open clone-source']",
  );
  await desktop.performActions([{
    type: "pointer",
    id: "clone-context-mouse",
    parameters: { pointerType: "mouse" },
    actions: [
      { type: "pointerMove", duration: 0, origin: sourceRow, x: 0, y: 0 },
      { type: "pointerDown", button: 2 },
      { type: "pointerUp", button: 2 },
    ],
  }]);
  await desktop.elementClick(await findEventually(
    desktop,
    "xpath",
    "//*[@role='menuitem' and normalize-space(.)='Clone']",
  ));
  await desktop.releaseActions();

  await expect.poll(() => desktop.execute<string>(`
    return document.querySelector('[role="dialog"] h2')?.textContent || "";
  `)).toBe("Clone agent");
  await expect.poll(() => desktop.execute<boolean>(`
    return document.querySelector('#create-agent-alias')?.value === "Source alias"
      && document.querySelector('#create-agent-interval')?.value === "12"
      && document.querySelector('#create-agent-message-queue')?.value === "900";
  `)).toBe(true);

  const expectedValues: Array<[string, string]> = [
    ["#create-agent-name", ""],
    ["[aria-label='cwd']", ""],
    ["#create-agent-model", "gpt-5"],
    ["#create-agent-effort", "high"],
    ["#create-agent-alias", "Source alias"],
    ["#create-agent-group", "desktop-clone-team"],
    ["#create-agent-color", "#123abc"],
    ["#create-agent-notes", "desktop clone notes"],
    ["#create-agent-timeout", "34"],
    ["#create-agent-hard-timeout", "56"],
    ["#create-agent-max-idle", "7"],
    ["#create-agent-message-batch", "8"],
    ["#create-agent-message-queue", "900"],
    ["#create-agent-user-prompt", "desktop clone prompt"],
  ];
  for (const [selector, expected] of expectedValues) {
    const field = await desktop.findElement("css selector", selector);
    await expect(desktop.elementProperty(field, "value"), selector).resolves.toBe(expected);
  }
  await expect(desktop.elementProperty(
    await desktop.findElement("css selector", "#create-agent-env"),
    "value",
  )).resolves.toBe('{\n  "CSV": "a,b",\n  "EQ": "a=b",\n  "LINES": "one\\ntwo"\n}');
  await expect(desktop.execute<string>(`
    return document.querySelector('#create-agent-on-timeout')?.value || "";
  `)).resolves.toBe("stop");
  await expect(desktop.execute<string>(`
    return document.querySelector('#create-agent-on-error')?.value || "";
  `)).resolves.toBe("restart");
  await expect(desktop.execute<string>(`
    return document.querySelector('#create-agent-interactive')?.getAttribute('aria-checked') || "";
  `)).resolves.toBe("false");
  await expect(desktop.execute<string>(`
    return document.querySelector('#create-agent-autopilot')?.getAttribute('aria-checked') || "";
  `)).resolves.toBe("false");
  await expect(desktop.execute<string>(`
    return document.querySelector('#create-agent-start')?.getAttribute('aria-checked') || "";
  `)).resolves.toBe("false");

  await desktop.elementSendKeys(
    await desktop.findElement("css selector", "#create-agent-name"),
    "clone-copy",
  );
  await desktop.elementClick(await desktop.findElement(
    "xpath",
    "//button[normalize-space(.)='Create agent']",
  ));

  await expect.poll(() => desktop.execute<string>(`
    if (window.__desktopCloneInspectPending) return "pending";
    window.__desktopCloneInspectPending = true;
    fetch(window.__desktopCloneBaseURL + "/api/agents/clone-copy")
      .then((response) => response.json())
      .then((envelope) => {
        window.__desktopCloneInspect = envelope.ok ? envelope.result : null;
      })
      .finally(() => { window.__desktopCloneInspectPending = false; });
    return window.__desktopCloneInspect ? "ready" : "pending";
  `), { timeout: 30_000 }).toBe("ready");
  const result = await desktop.execute<Record<string, unknown>>(
    "return window.__desktopCloneInspect;",
  );
  expect(result).toMatchObject({
    name: "clone-copy",
    image: "basic:latest",
    configured_cwd: "",
    harness: "codex",
    model: "gpt-5",
    effort: "high",
    interactive: false,
    loop_enabled: false,
    enabled: false,
    interval_s: 12,
    timeout_s: 34,
    hard_timeout_s: 56,
    on_timeout: "stop",
    on_error: "restart",
    max_idle_iterations: 7,
    user_prompt: "desktop clone prompt",
    env: { CSV: "a,b", EQ: "a=b", LINES: "one\ntwo" },
    messages_batch: 8,
    messages_max_queue: 900,
    group: "desktop-clone-team",
    alias: "Source alias",
    notes: "desktop clone notes",
    color: "#123abc",
  });
  const source = await desktop.execute<Record<string, unknown>>(`
    return fetch(window.__desktopCloneBaseURL + "/api/agents/clone-source")
      .then((response) => response.json())
      .then((envelope) => envelope.result);
  `);
  expect(result.plugins).toEqual(source.plugins);
});
