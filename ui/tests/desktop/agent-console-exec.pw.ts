import { expect, test, waitForMainWindow } from "./fixture";
import type { W3CClient } from "./w3c";

async function openConsole(desktop: W3CClient, name: string): Promise<void> {
  await desktop.execute(`window.location.hash = ${JSON.stringify(`#/agents/local/${name}/console`)}; return true;`);
  await expect.poll(async () => desktop.execute<string>(
    "return document.body ? document.body.innerText : '';",
  )).toContain(name);
}

async function execPrompt(desktop: W3CClient, prompt: string): Promise<void> {
  const input = await expect.poll(async () => {
    try {
      return await desktop.findElement("css selector", '[placeholder="one-shot exec prompt (optional)"]');
    } catch {
      return null;
    }
  }).not.toBeNull().then(() => desktop.findElement(
    "css selector", '[placeholder="one-shot exec prompt (optional)"]',
  ));
  await desktop.elementSendKeys(input, prompt);
  const button = await desktop.findElement("xpath", "//button[normalize-space(.)='Exec']");
  await desktop.elementClick(button);
}

test("manual Exec reaches the isolated daemon for interactive and non-interactive agents", async ({ desktop }) => {
  await waitForMainWindow(desktop);

  await desktop.execute(`
    window.__consoleExecSetup = "running";
    window.__consoleExecCalls = [];
    window.__TAURI_INTERNALS__.invoke("daemon_status")
      .then(async (status) => {
        window.__consoleExecBaseURL = status.base_url;
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
          image: "basic:latest", name: "exec-interactive-e2e", harness: "stub",
          interactive: true, loop: false,
        });
        await call("POST", "/api/agents", {
          image: "basic:latest", name: "exec-batch-e2e", harness: "stub",
          interactive: false, loop: false,
        });
        const originalFetch = window.fetch.bind(window);
        window.fetch = (input, init) => {
          const url = typeof input === "string" ? input : input.url;
          if (url.endsWith("/exec")) window.__consoleExecCalls.push({ url, body: init?.body ?? null });
          return originalFetch(input, init);
        };
        window.__consoleExecSetup = "ready";
      })
      .catch((error) => { window.__consoleExecSetup = "error: " + String(error); });
    return true;
  `);
  await expect.poll(() => desktop.execute<string>("return window.__consoleExecSetup || '';"), {
    timeout: 60_000,
  }).toBe("ready");

  await openConsole(desktop, "exec-interactive-e2e");
  await execPrompt(desktop, "interactive one-shot prompt");
  await expect.poll(() => desktop.execute<number>("return window.__consoleExecCalls.length;"), {
    timeout: 30_000,
  }).toBe(1);

  await openConsole(desktop, "exec-batch-e2e");
  await expect.poll(() => desktop.execute<string>("return document.body.innerText;"))
    .toContain("This agent has no interactive terminal.");
  await execPrompt(desktop, "batch one-shot prompt");
  await expect.poll(() => desktop.execute<number>("return window.__consoleExecCalls.length;"), {
    timeout: 30_000,
  }).toBe(2);

  const iterations = await expect.poll(async () => {
    await desktop.execute(`
      if (!window.__consoleExecIterationsPending) {
        window.__consoleExecIterationsPending = true;
        Promise.all(["exec-interactive-e2e", "exec-batch-e2e"].map(async (name) => {
          const response = await fetch(window.__consoleExecBaseURL + "/api/agents/" + name + "/iterations");
          const envelope = await response.json();
          if (!response.ok || !envelope.ok) throw new Error(name + ": " + JSON.stringify(envelope.error));
          return envelope.result.iterations;
        })).then((value) => { window.__consoleExecIterations = value; })
          .catch((error) => { window.__consoleExecIterations = "error: " + String(error); })
          .finally(() => { window.__consoleExecIterationsPending = false; });
      }
      return true;
    `);
    const value = await desktop.execute<unknown>("return window.__consoleExecIterations;");
    if (!Array.isArray(value)) return value;
    return value.map((rows) => Array.isArray(rows)
      && rows.some((row) => row.trigger === "manual" && row.status !== "running"));
  }, { timeout: 60_000 }).toEqual([true, true]).then(() => desktop.execute<unknown>(
    "return window.__consoleExecIterations;",
  ));
  expect(iterations).toEqual(expect.any(Array));

  const calls = await desktop.execute<Array<{ url: string; body: string | null }>>(
    "return window.__consoleExecCalls;",
  );
  expect(calls.map((call) => JSON.parse(call.body ?? "null"))).toEqual([
    { prompt: "interactive one-shot prompt" },
    { prompt: "batch one-shot prompt" },
  ]);
});
