import { spawnSync } from "node:child_process";
import { access } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { expect, test, waitForMainWindow } from "./fixture";

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../../..");
const tools = join(repositoryRoot, "desktop/src-tauri/resources/bin/linux-x86_64/tariboy-tools");

test("opens an agent customer question from the production Desktop notification flow", async ({ desktop, desktopWorker }) => {
  desktopWorker.registerAgentForCleanup("question-requester");
  await waitForMainWindow(desktop);

  await desktop.execute(`
    window.__customerQuestionSetup = "running";
    window.__TAURI_INTERNALS__.invoke("daemon_status")
      .then(async (status) => {
        window.__customerQuestionBaseURL = status.base_url;
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
        const principals = await call("GET", "/api/task-principals");
        await call("POST", "/api/agents", {
          image: "basic:latest", name: "question-requester", harness: "stub",
          interactive: true, loop: false, env: "STUB_SLEEP=300,STUB_CALL_DONE=0",
        });
        await call("POST", "/api/agents/question-requester/start", {});
        await call("POST", "/api/task-queues", {
          prefix: "ASK", name: "Desktop questions", owners: ["question-requester"],
        });
        const task = await call("POST", "/api/tasks", {
          queue: "ASK", title: "Choose the production behavior", assignee: "question-requester",
        });
        window.__customerQuestionTaskKey = task.key;
        window.__customerQuestionCustomer = principals.customer;
        window.__customerQuestionSetup = "ready";
      })
      .catch((error) => { window.__customerQuestionSetup = "error: " + String(error); });
    return true;
  `);
  await expect.poll(() => desktop.execute<string>("return window.__customerQuestionSetup || '';"), {
    timeout: 60_000,
  }).toBe("ready");

  const setup = await desktop.execute<{ customer: string; taskKey: string }>(`
    return {
      customer: window.__customerQuestionCustomer,
      taskKey: window.__customerQuestionTaskKey,
    };
  `);
  const socket = join(desktopWorker.runtimeDir, "question-requester.sock");
  await expect.poll(async () => {
    try { await access(socket); return true; } catch { return false; }
  }, { timeout: 30_000 }).toBe(true);
  const commented = spawnSync(tools, [
    "--json", "tasks", "comment", setup.taskKey,
    `Need a decision from @${setup.customer}`,
  ], {
    env: { ...process.env, TARIBOY_TOOLS_SOCKET: socket },
    encoding: "utf8",
    timeout: 30_000,
  });
  expect(commented.status, commented.stderr || commented.stdout).toBe(0);

  await expect.poll(() => desktop.execute<boolean>(`
    return [...document.querySelectorAll('[role="img"]')]
      .some((element) => element.getAttribute("aria-label")
        === "Unread customer question for question-requester on This daemon (local)");
  `), { timeout: 30_000 }).toBe(true);

  await expect.poll(() => desktop.execute<Record<string, string> | null>(`
    if (window.__customerQuestionNotification) return window.__customerQuestionNotification;
    if (window.__customerQuestionNotificationPending) return null;
    window.__customerQuestionNotificationPending = true;
    fetch(window.__customerQuestionBaseURL + "/api/task-notifications")
      .then((response) => response.json())
      .then((envelope) => {
        window.__customerQuestionNotification = envelope.result.notifications
          .find((item) => item.task_key === window.__customerQuestionTaskKey) || null;
      })
      .finally(() => { window.__customerQuestionNotificationPending = false; });
    return null;
  `)).not.toBeNull();
  const notification = await desktop.execute<{ id: string; task_key: string }>(
    "return window.__customerQuestionNotification;",
  );
  await desktop.execute(`
    window.__customerQuestionActivation = "running";
    window.__TAURI_INTERNALS__.invoke("task_notification_activate_test", {
      input: {
        host_id: "",
        notification_id: ${JSON.stringify(notification.id)},
        task_key: ${JSON.stringify(notification.task_key)},
      },
    })
      .then(() => { window.__customerQuestionActivation = "activated"; })
      .catch((error) => { window.__customerQuestionActivation = "error: " + String(error); });
    return true;
  `);
  await expect.poll(() => desktop.execute<string>(
    "return window.__customerQuestionActivation || '';",
  )).toBe("activated");

  await expect.poll(() => desktop.execute<string>("return window.location.hash;"), {
    timeout: 30_000,
  }).toBe(`#/servers/local/tasks?task=${encodeURIComponent(setup.taskKey)}`);
  await expect.poll(() => desktop.execute<string>(`
    const label = [...document.querySelectorAll('.task-detail-panel label')]
      .find((candidate) => candidate.textContent.trim().startsWith("Title"));
    return label?.querySelector("input")?.value || "";
  `)).toBe("Choose the production behavior");
  expect(await desktop.execute<string>(
    "return document.querySelector('.task-detail-panel')?.innerText || '';",
  )).toContain(setup.taskKey);
});
