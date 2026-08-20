import { expect, test, waitForMainWindow } from "./fixture";
import type { W3CClient, W3CElement } from "./w3c";

async function drag(desktop: W3CClient, source: W3CElement, target: W3CElement): Promise<void> {
  await desktop.performActions([{
    type: "pointer",
    id: "task-priority-mouse",
    parameters: { pointerType: "mouse" },
    actions: [
      { type: "pointerMove", duration: 0, origin: source, x: 0, y: 0 },
      { type: "pointerDown", button: 0 },
      { type: "pause", duration: 150 },
      { type: "pointerMove", duration: 150, origin: source, x: 12, y: 0 },
      { type: "pointerMove", duration: 400, origin: target, x: -12, y: 0 },
      { type: "pointerMove", duration: 150, origin: target, x: 0, y: 0 },
      { type: "pause", duration: 150 },
      { type: "pointerUp", button: 0 },
    ],
  }]);
}

test("orders, edits, and reorders task priorities in the production Desktop", async ({ desktop }) => {
  await waitForMainWindow(desktop);

  await desktop.execute(`
    window.__taskPrioritySetup = "running";
    window.__TAURI_INTERNALS__.invoke("daemon_status")
      .then(async (status) => {
        window.__taskPriorityBaseURL = status.base_url;
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
        await call("POST", "/api/task-queues", { prefix: "PRI", name: "Priority Desktop E2E" });
        const critical = await call("POST", "/api/tasks", { queue: "PRI", title: "Critical first", priority: "P0" });
        const criticalSecond = await call("POST", "/api/tasks", { queue: "PRI", title: "Critical second", priority: "P0" });
        const low = await call("POST", "/api/tasks", { queue: "PRI", title: "Low last", priority: "P3" });
        window.__taskPriorityKeys = { critical: critical.key, criticalSecond: criticalSecond.key, low: low.key };
        window.__taskPrioritySetup = "ready";
      })
      .catch((error) => { window.__taskPrioritySetup = "error: " + String(error); });
    return true;
  `);
  await expect.poll(() => desktop.execute<string>("return window.__taskPrioritySetup || '';"), {
    timeout: 60_000,
  }).toBe("ready");

  await desktop.execute(`window.location.hash = "#/servers/local/tasks"; return true;`);
  await expect.poll(() => desktop.execute<string>("return document.body?.innerText || '';"))
    .toContain("Critical first");
  await expect.poll(() => desktop.execute<string[]>(`
    return [...document.querySelectorAll('[data-testid^="task-row-PRI-"]')]
      .map((row) => row.getAttribute("data-testid"));
  `)).toEqual(["task-row-PRI-1", "task-row-PRI-2", "task-row-PRI-3"]);
  await expect.poll(() => desktop.execute<string[]>(`
    return [...document.querySelectorAll('[data-testid^="task-row-PRI-"] .task-priority')]
      .map((marker) => marker.getAttribute("aria-label"));
  `)).toEqual(["P0 Critical", "P0 Critical", "P3 Low"]);

  await desktop.findElement("css selector", '[aria-label="Resize task navigation"]');
  const handlePosition = await desktop.execute<{ x: number; y: number }>(`
    const bounds = document.querySelector('[aria-label="Resize task navigation"]').getBoundingClientRect();
    return { x: Math.round(bounds.x + bounds.width / 2), y: Math.round(bounds.y + 80) };
  `);
  await desktop.performActions([{
    type: "pointer",
    id: "task-panel-resize-mouse",
    parameters: { pointerType: "mouse" },
    actions: [
      { type: "pointerMove", duration: 0, origin: "viewport", x: handlePosition.x, y: handlePosition.y },
      { type: "pointerDown", button: 0 },
      { type: "pointerMove", duration: 300, origin: "viewport", x: handlePosition.x + 72, y: handlePosition.y },
      { type: "pointerUp", button: 0 },
    ],
  }]);
  await expect.poll(() => desktop.execute<number>(`
    return JSON.parse(localStorage.getItem("tasks:workspace:v1") || "{}").navigationWidth || 0;
  `)).toBeGreaterThan(260);
  await expect.poll(() => desktop.execute<string>(`
    return document.querySelector('[aria-label="Resize task navigation"]')?.getAttribute("aria-valuenow") || "";
  `)).not.toBe("208");
  await desktop.elementClick(await desktop.findElement("css selector", '[data-testid="task-row-PRI-3"] .task-row-main'));
  await expect.poll(() => desktop.execute<boolean>(`
    return [...document.querySelectorAll('.task-detail-panel label')]
      .some((label) => label.textContent.trim().startsWith("Priority") && label.querySelector("select"));
  `)).toBe(true);
  await desktop.execute(`
    const label = [...document.querySelectorAll('.task-detail-panel label')]
      .find((candidate) => candidate.textContent.trim().startsWith("Priority"));
    const select = label.querySelector("select");
    select.value = "P0";
    select.dispatchEvent(new Event("change", { bubbles: true }));
    return true;
  `);
  await desktop.elementClick(await desktop.findElement("xpath", "//aside[contains(@class,'task-detail-panel')]//button[normalize-space(.)='Save task']"));
  await expect.poll(() => desktop.execute<string>(`
    return document.querySelector('[data-testid="task-row-PRI-3"] .task-priority')?.getAttribute("aria-label") || "";
  `)).toBe("P0 Critical");

  const secondGrip = await desktop.findElement("css selector", '[aria-label="Move PRI-2"]');
  const beforeFirst = await desktop.findElement("css selector", '[data-testid="drop-before-PRI-1"]');
  await drag(desktop, secondGrip, beforeFirst);
  await expect.poll(() => desktop.execute<boolean>(`
    if (window.__taskPriorityOrderPending) return false;
    window.__taskPriorityOrderPending = true;
    fetch(window.__taskPriorityBaseURL + "/api/tasks")
      .then((response) => response.json())
      .then((envelope) => {
        const tasks = envelope.result.tasks;
        const first = tasks.find((task) => task.key === "PRI-1");
        const second = tasks.find((task) => task.key === "PRI-2");
        window.__taskPriorityOrder = second.position < first.position;
      })
      .finally(() => { window.__taskPriorityOrderPending = false; });
    return window.__taskPriorityOrder === true;
  `), { timeout: 30_000 }).toBe(true);
});
