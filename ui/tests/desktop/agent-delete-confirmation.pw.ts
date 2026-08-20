import { expect, test, waitForMainWindow } from "./fixture";
import type { W3CClient } from "./w3c";

async function bodyText(desktop: W3CClient): Promise<string> {
  return desktop.execute<string>("return document.body ? document.body.innerText : '';");
}

async function openConsole(desktop: W3CClient, name: string): Promise<void> {
  await desktop.execute(`window.location.hash = ${JSON.stringify(`#/agents/local/${name}/console`)}; return true;`);
  await expect.poll(() => bodyText(desktop)).toContain(name);
}

test("agent deletion requires confirmation in the production Desktop", async ({ desktop }) => {
  await waitForMainWindow(desktop);

  await desktop.execute(`
    window.__agentDeleteSetup = "running";
    window.__agentDeleteCalls = [];
    window.__TAURI_INTERNALS__.invoke("daemon_status")
      .then(async (status) => {
        const response = await fetch(status.base_url + "/api/agents", {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({
            image: "basic:latest", name: "delete-confirmation-e2e", harness: "stub",
            interactive: false, loop: false,
          }),
        });
        const envelope = await response.json();
        if (!response.ok || !envelope.ok) throw new Error(JSON.stringify(envelope.error));
        const originalFetch = window.fetch.bind(window);
        window.fetch = (input, init) => {
          const url = typeof input === "string" ? input : input.url;
          if ((init?.method ?? "GET").toUpperCase() === "DELETE") window.__agentDeleteCalls.push(url);
          return originalFetch(input, init);
        };
        window.__agentDeleteSetup = "ready";
      })
      .catch((error) => { window.__agentDeleteSetup = "error: " + String(error); });
    return true;
  `);
  await expect.poll(() => desktop.execute<string>("return window.__agentDeleteSetup || '';"), {
    timeout: 60_000,
  }).toBe("ready");

  await openConsole(desktop, "delete-confirmation-e2e");
  await desktop.elementClick(await desktop.findElement("xpath", "//button[normalize-space(.)='Delete']"));

  await expect.poll(() => bodyText(desktop)).toContain("Delete agent delete-confirmation-e2e?");
  await expect.poll(() => desktop.execute<number>("return window.__agentDeleteCalls.length;")).toBe(0);

  await desktop.elementClick(await desktop.findElement("xpath", "//button[normalize-space(.)='Cancel']"));
  await expect.poll(() => bodyText(desktop)).not.toContain("This action cannot be undone.");
  await expect.poll(() => desktop.execute<number>("return window.__agentDeleteCalls.length;")).toBe(0);

  await desktop.elementClick(await desktop.findElement("xpath", "//button[normalize-space(.)='Delete']"));
  await desktop.elementClick(await desktop.findElement("xpath", "//button[normalize-space(.)='Delete agent']"));

  await expect.poll(() => desktop.execute<number>("return window.__agentDeleteCalls.length;")).toBe(1);
  await expect.poll(() => desktop.execute<string>("return window.location.hash;")).toBe("#/");
  expect(await desktop.execute<string>("return window.__agentDeleteCalls[0];"))
    .toContain("/api/agents/delete-confirmation-e2e?force=true&purge=true");
});
