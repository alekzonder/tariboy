import { spawnSync } from "node:child_process"
import { join } from "node:path"
import { expect, test, waitForMainWindow } from "./fixture"

test("configures and inspects managed tasks in the production Desktop", async ({ desktop }) => {
  await waitForMainWindow(desktop)
  await desktop.execute(`
    window.__workflowSetup = "running";
    window.__TAURI_INTERNALS__.invoke("daemon_status").then(async (status) => {
      const call = async (method, path, body) => {
        const response = await fetch(status.base_url + path, { method, headers: { "content-type": "application/json" }, body: JSON.stringify(body || {}) });
        const envelope = await response.json();
        if (!response.ok || !envelope.ok) throw new Error(path + ": " + JSON.stringify(envelope.error));
        return envelope.result;
      };
      await call("POST", "/api/agents", { image: "basic:latest", name: "workflow-desktop", harness: "codex", loop: false });
      await call("POST", "/api/agents", { image: "basic:latest", name: "workflow-desktop-qa", harness: "codex", loop: false });
      await call("POST", "/api/task-queues", { prefix: "WFD", name: "Workflow Desktop" });
      window.__workflowCall = call;
      window.__workflowBaseDir = status.base_dir;
      window.__workflowSetup = "ready";
    }).catch((error) => { window.__workflowSetup = "error: " + String(error); });
    return true;
  `)
  await expect.poll(() => desktop.execute<string>("return window.__workflowSetup || '';"), { timeout: 60_000 }).toBe("ready")
  await desktop.execute(`window.location.hash = "#/servers/local/tasks"; return true;`)
  await expect.poll(() => desktop.execute<string>("return document.body?.innerText || '';"), { timeout: 30_000 }).toContain("Queues")
  await desktop.elementClick(await desktop.findElement("xpath", "//button[normalize-space(.)='Queues']"))
  await expect.poll(() => desktop.execute<string>("return document.body?.innerText || '';" )).toContain("Workflow Desktop")

  await desktop.elementSendKeys(await desktop.findElement("css selector", '[aria-label="Pool name WFD"]'), "workers")
  await desktop.elementSendKeys(await desktop.findElement("css selector", '[aria-label="Pool agents WFD"]'), "workflow-desktop, workflow-desktop-qa")
  await desktop.elementClick(await desktop.findElement("xpath", "//article[.//strong[normalize-space(.)='WFD']]//button[normalize-space(.)='Save pool']"))
  await expect.poll(() => desktop.execute<string>("return document.body?.innerText || '';" )).toContain("workflow-desktop-qa")

  await desktop.elementClick(await desktop.findElement("xpath", "//article[.//strong[normalize-space(.)='WFD']]//summary[contains(.,'Create definition')]"))
  const definition = JSON.stringify({ name: "desktop-flow", version: 1, initial_status: "work", statuses: [
    { id: "work", requirements: [{ id: "implementation", pool: "workers", dispatch: "require_all", inputs: [], produces: ["result"], outcomes: ["done"] }], transitions: [{ when: "implementation.all(done)", to: "done" }] },
    { id: "done", requirements: [], transitions: [], terminal: true },
  ] })
  await desktop.elementSendKeys(await desktop.findElement("css selector", '[aria-label="Workflow definition WFD"]'), definition)
  await desktop.elementClick(await desktop.findElement("xpath", "//article[.//strong[normalize-space(.)='WFD']]//button[normalize-space(.)='Validate and publish']"))
  await expect.poll(() => desktop.execute<string>("return document.body?.innerText || '';" )).toContain("Workflow published")
  await desktop.elementClick(await desktop.findElement("xpath", "//article[.//strong[normalize-space(.)='WFD']]//button[normalize-space(.)='Load versions']"))
  await expect.poll(() => desktop.execute<boolean>(`return !!document.querySelector('[aria-label="Published workflow version WFD"] option[value]:not([value=""])');`)).toBe(true)
  await desktop.execute(`
    const select = document.querySelector('[aria-label="Published workflow version WFD"]');
    select.value = [...select.options].find((option) => option.value).value;
    select.dispatchEvent(new Event("change", { bubbles: true })); return true;
  `)
  await desktop.elementClick(await desktop.findElement("xpath", "//article[.//strong[normalize-space(.)='WFD']]//button[normalize-space(.)='Activate workflow']"))
  await expect.poll(() => desktop.execute<string>("return document.body?.innerText || '';" )).toContain("Active: desktop-flow@1")
  await desktop.execute(`window.__workflowCall("POST", "/api/tasks", { queue: "WFD", title: "Managed desktop task", idempotency_key: "desktop-task" }).then(() => window.__workflowTaskReady = true); return true;`)
  await expect.poll(() => desktop.execute<boolean>("return window.__workflowTaskReady === true;")).toBe(true)
  const baseDir = await desktop.execute<string>("return window.__workflowBaseDir;")
  const seeded = spawnSync("python3", ["-c", `
import json, sqlite3, sys
db = sqlite3.connect(sys.argv[1], timeout=10)
task_id, revision = db.execute("SELECT id, revision FROM tasks WHERE task_key='WFD-1'").fetchone()
assignments = [row[0] for row in db.execute("SELECT a.id FROM task_assignments a JOIN task_requirement_executions r ON r.id=a.requirement_execution_id JOIN task_status_executions s ON s.id=r.status_execution_id WHERE s.task_id=? ORDER BY a.id", (task_id,))]
requirement_id = db.execute("SELECT requirement_execution_id FROM task_assignments WHERE id=?", (assignments[0],)).fetchone()[0]
now = "2026-08-07T14:00:00Z"
db.execute("UPDATE task_status_executions SET state='frozen' WHERE task_id=? AND state='active'", (task_id,))
db.execute("INSERT INTO task_artifacts(task_id,assignment_id,name,type,content,metadata,revision,created_by,created_at,updated_at) VALUES(?,?,?,'markdown',?,'{}',1,'agent:workflow-desktop',?,?)", (task_id, assignments[0], 'incident-report', 'Runtime artifact', now, now))
question_id = db.execute("INSERT INTO task_workflow_questions(task_id,assignment_id,requirement_execution_id,question,context,blocking_scope,state,created_at) VALUES(?,?,?,?,?,'assignment','open',?)", (task_id, assignments[0], requirement_id, 'Which mitigation?', 'Production signal changed', now)).lastrowid
db.execute("INSERT INTO task_workflow_holds(task_id,assignment_id,requirement_execution_id,question_id,scope,reason,created_at) VALUES(?,?,?,?,?,?,?)", (task_id, assignments[0], requirement_id, question_id, 'assignment', 'Awaiting incident decision', now))
db.execute("INSERT INTO task_observations(task_id,assignment_id,kind,payload,observed_at) VALUES(?,?,'logs',?,?)", (task_id, assignments[0], json.dumps({'service': 'api'}), now))
db.execute("INSERT INTO task_events(event_id,task_id,queue_prefix,kind,actor,task_revision,payload,created_at) VALUES('desktop-workflow-error',?,'WFD','workflow.escalated','system',?,?,?)", (task_id, revision, json.dumps({'error_code': 'no_matching_transition', 'message': 'No transition matched'}), now))
db.commit()
`, join(baseDir, "tariboyd.db")], { encoding: "utf8" })
  expect(seeded.status, seeded.stderr).toBe(0)
  await desktop.elementClick(await desktop.findElement("xpath", "//button[normalize-space(.)='All tasks']"))
  await expect.poll(() => desktop.execute<string>("return document.body?.innerText || '';"), { timeout: 30_000 }).toContain("Managed desktop task")
  await desktop.elementClick(await desktop.findElement("css selector", '[data-testid="task-row-WFD-1"] .task-row-main'))
  await expect.poll(() => desktop.execute<string>("return document.querySelector('.task-detail-panel')?.innerText || '';"))
    .toContain("desktop-flow@1")
  await expect.poll(() => desktop.execute<string>("return document.querySelector('.task-detail-panel')?.innerText || '';"))
    .toContain("workflow-desktop-qa")
  const runtime = await desktop.execute<string>("return document.querySelector('.task-detail-panel')?.innerText || '';")
  expect(runtime).toContain("Awaiting incident decision")
  expect(runtime).toContain("Runtime artifact")
  expect(runtime).toContain("Which mitigation?")
  expect(runtime).toContain("logs")
  expect(runtime).toContain("No transition matched · no_matching_transition")
  expect(await desktop.execute<number>(`return document.querySelectorAll('.task-detail-panel [aria-label="Status"], .task-detail-panel [aria-label="Assignee"]').length;`)).toBe(0)
})
