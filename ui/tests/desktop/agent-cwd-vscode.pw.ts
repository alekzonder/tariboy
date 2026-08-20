import { expect, test, waitForMainWindow } from "./fixture";

test("shows the effective agent cwd and its VS Code action", async ({ desktop }) => {
  await waitForMainWindow(desktop);

  const started = await desktop.execute<boolean>(`
    window.__agentCwdSetup = { state: "pending" };
    window.__TAURI_INTERNALS__.invoke("daemon_status")
      .then((status) => fetch(status.base_url + "/api/agents", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ image: "basic:latest", name: "cwd-vscode-e2e", interactive: false, loop: false }),
      }).then((response) => ({ response, baseURL: status.base_url })))
      .then(async ({ response, baseURL }) => {
        const payload = await response.json();
        if (!response.ok || !payload.ok) throw new Error(payload?.error?.message || "agent create failed");
        return fetch(baseURL + "/api/agents/cwd-vscode-e2e");
      })
      .then(async (response) => {
        const payload = await response.json();
        if (!response.ok || !payload.ok) throw new Error(payload?.error?.message || "agent inspect failed");
        window.__agentCwdSetup = { state: "ready", cwd: payload.result.cwd };
      })
      .catch((error) => { window.__agentCwdSetup = { state: "error", message: String(error) }; });
    return true;
  `);
  expect(started).toBe(true);

  await expect.poll(async () => {
    const value = await desktop.execute<{ state: string; message?: string }>(
      "return window.__agentCwdSetup || { state: 'missing' };",
    );
    return value.state === "error" ? `error: ${value.message ?? "unknown"}` : value.state;
  }, { timeout: 60_000 }).toBe("ready");
  const setup = await desktop.execute<{ state: string; cwd: string }>(
    "return window.__agentCwdSetup;",
  );

  await desktop.execute(`window.location.hash = "#/agents/local/cwd-vscode-e2e/console"; return true;`);

  const cwd = await expect.poll(async () => {
    try {
      const element = await desktop.findElement("css selector", '[data-testid="agent-cwd"]');
      return await desktop.elementText(element);
    } catch {
      return "";
    }
  }).toBe(setup.cwd).then(async () => desktop.findElement("css selector", '[data-testid="agent-cwd"]'));
  await expect(desktop.elementText(cwd)).resolves.toBe(setup.cwd);

  const action = await expect.poll(async () => {
    try {
      return await desktop.findElement("css selector", '[data-testid="open-agent-cwd-vscode"]');
    } catch {
      return null;
    }
  }).not.toBeNull().then(() => desktop.findElement("css selector", '[data-testid="open-agent-cwd-vscode"]'));
  await expect(desktop.elementText(action)).resolves.toBe("Open in VS Code");
});
