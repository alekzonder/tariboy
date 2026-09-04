import { expect, test, waitForMainWindow } from "./fixture";

test("configures an agent goal and task release fields", async ({ desktop, desktopWorker }) => {
  await waitForMainWindow(desktop);

  await desktop.execute(`
    window.__agentGoalSetup = "running";
    window.__TAURI_INTERNALS__.invoke("daemon_status")
      .then(async (status) => {
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
        await call("POST", "/api/agents", { name: "worker", image: "basic:latest", harness: "stub", loop: false });
        await call("POST", "/api/task-queues", { prefix: "TARI", name: "Goals" });
        await call("POST", "/api/tasks", { queue: "TARI", title: "Goal one", assignee: "agent:worker" });
        await call("POST", "/api/agents/worker/start");
        await call("POST", "/api/agents/worker/loop/enable");
        window.location.hash = "#/agents/local/worker/configuration";
        window.__agentGoalSetup = "ready";
      })
      .catch((error) => { window.__agentGoalSetup = "error: " + String(error); });
    return true;
  `);
  desktopWorker.registerAgentForCleanup("worker");
  await expect.poll(() => desktop.execute<string>("return window.__agentGoalSetup || '';"), {
    timeout: 60_000,
  }).toBe("ready");
  await expect.poll(() => desktop.execute<string>("return document.body.innerText")).toContain("worker");
  await desktop.execute(`window.location.hash = "#/agents/local/worker/configuration"; return true;`);
  await expect.poll(() => desktop.execute<string>("return document.body.innerText")).toContain("Current goal");
  await expect.poll(() => desktop.execute<string>(`
    return document.querySelector("#current-goal-task")?.value || "";
  `), { timeout: 30_000 }).toBe("TARI-1");

  await desktop.elementClick(await desktop.findElement("css selector", "#goal-enabled"));
  await desktop.elementClick(await desktop.findElement("xpath", "//button[normalize-space(.)='Save Goal settings']"));
  await expect.poll(() => desktop.execute<string>("return document.body.innerText")).toContain("Goal settings saved");
  await desktop.execute(`window.location.hash = "#/servers/local/tasks"; return true;`);
  await desktop.execute(`window.location.hash = "#/agents/local/worker/configuration"; return true;`);
  await expect.poll(() => desktop.execute<string>("return document.body.innerText")).toContain("Current goal");
  await expect.poll(() => desktop.execute<boolean>(`
    return document.querySelector("#goal-enabled")?.getAttribute("data-state") === "unchecked";
  `)).toBe(true);
  await expect.poll(() => desktop.execute<string>(`
    return document.querySelector("#current-goal-task")?.value || "";
  `)).toBe("No current goal");

  await desktop.elementClick(await desktop.findElement("css selector", "#goal-enabled"));
  await desktop.execute(`
    const input = document.querySelector("#goal-wait-customer-timeout");
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set.call(input, "120");
    input.dispatchEvent(new Event("input", { bubbles: true }));
    return true;
  `);
  await desktop.elementClick(await desktop.findElement("xpath", "//button[normalize-space(.)='Save Goal settings']"));
  await expect.poll(() => desktop.execute<string>("return document.body.innerText")).toContain("Goal settings saved");
  await desktop.execute(`window.location.hash = "#/servers/local/tasks"; return true;`);
  await desktop.execute(`window.location.hash = "#/agents/local/worker/configuration"; return true;`);
  await expect.poll(() => desktop.execute<string>("return document.body.innerText")).toContain("Current goal");
  await expect.poll(() => desktop.execute<boolean>(`
    return document.querySelector("#goal-enabled")?.getAttribute("data-state") === "checked";
  `)).toBe(true);
  await expect.poll(() => desktop.execute<string>(`
    return document.querySelector("#goal-wait-customer-timeout")?.value || "";
  `)).toBe("120");
  await expect.poll(() => desktop.execute<string>(`
    return document.querySelector("#current-goal-task")?.value || "";
  `), { timeout: 30_000 }).toBe("TARI-1");

  await desktop.execute(`
    window.__agentGoalPR = "saving";
    window.__TAURI_INTERNALS__.invoke("daemon_status")
      .then(async (status) => {
        const detailResponse = await fetch(status.base_url + "/api/tasks/TARI-1");
        const detailEnvelope = await detailResponse.json();
        const task = detailEnvelope.result.task;
        const response = await fetch(status.base_url + "/api/tasks/TARI-1", {
          method: "PATCH",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({
            title: task.title, description: task.description, status: task.status,
            assignee: task.assignee, manual_block_reason: task.manual_block_reason,
            priority: task.priority, pull_request: "https://github.com/acme/tariboy/pull/43",
            revision: task.revision,
          }),
        });
        const envelope = await response.json();
        if (!response.ok || !envelope.ok) throw new Error(JSON.stringify(envelope.error));
        window.__agentGoalPR = "saved";
      })
      .catch((error) => { window.__agentGoalPR = "error: " + String(error); });
    return true;
  `);
  await expect.poll(() => desktop.execute<string>("return window.__agentGoalPR || '';"), {
    timeout: 30_000,
  }).toBe("saved");

  await desktop.execute(`window.location.hash = "#/servers/local/tasks"; return true;`);
  await desktop.execute(`window.location.hash = "#/agents/local/worker/configuration"; return true;`);
  await expect.poll(() => desktop.execute<string>("return document.body.innerText")).toContain("Current goal");
  await expect.poll(() => desktop.execute<string>(`
    return document.querySelector("#current-goal-task")?.value || "";
  `), { timeout: 30_000 }).toBe("No current goal");

  await desktop.execute(`window.location.hash = "#/servers/local/tasks?task=TARI-1"; return true;`);
  await expect.poll(() => desktop.execute<string>("return document.body.innerText")).toContain("TARI-1");
  await expect.poll(() => desktop.execute<string>(`
    return [...document.querySelectorAll(".task-detail-panel label")]
      .find((label) => label.textContent.trim().startsWith("Pull request URL"))
      ?.querySelector("input")?.value || "";
  `)).toBe("https://github.com/acme/tariboy/pull/43");
});
