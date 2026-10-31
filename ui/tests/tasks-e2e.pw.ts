import { chmodSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { expect, test, type APIRequestContext, type Locator, type Page } from "playwright/test";

const daemonURL = "http://127.0.0.1:4176";

async function taskFromAPI(request: APIRequestContext, key: string) {
  const response = await request.get(`${daemonURL}/api/tasks/${key}`);
  expect(response.ok()).toBe(true);
  const envelope = await response.json();
  expect(envelope.ok).toBe(true);
  return envelope.result.task as {
    key: string;
    parent_key: string;
    position: number;
    priority: "P0" | "P1" | "P2" | "P3";
    title: string;
    description: string;
    status: string;
    assignee: string;
    manual_block_reason: string;
    pull_request: string;
  };
}

async function createTask(
  request: APIRequestContext,
  data: { title: string; priority: "P0" | "P1" | "P2" | "P3"; parent_key?: string },
) {
  const response = await request.post(`${daemonURL}/api/tasks`, {
    data: {
      queue: "TEST",
      idempotency_key: `tasks-browser-priority-${data.title.toLowerCase().replaceAll(" ", "-")}`,
      ...data,
    },
  });
  expect(response.ok()).toBe(true);
  const envelope = await response.json();
  expect(envelope.ok).toBe(true);
  return envelope.result as { key: string };
}

async function keyByTitle(request: APIRequestContext, title: string) {
  let key = "";
  await expect
    .poll(
      async () => {
        const response = await request.get(`${daemonURL}/api/tasks?status_view=all&limit=500`);
        if (!response.ok()) return "";
        const envelope = await response.json();
        const match = (envelope.result?.tasks ?? []).find(
          (task: { title: string }) => task.title === title,
        );
        key = match?.key ?? "";
        return key;
      },
      { timeout: 10_000 },
    )
    .not.toBe("");
  return key;
}

async function visibleTaskKeys(page: Page) {
  return page
    .locator("[data-testid^='task-row-']")
    .evaluateAll((rows) =>
      rows.map((row) => row.getAttribute("data-testid")?.replace("task-row-", "")),
    );
}

async function pointerDrag(
  page: Page,
  source: ReturnType<Page["getByRole"]>,
  target: ReturnType<Page["getByTestId"]>,
) {
  const sourceBox = await source.boundingBox();
  const targetBox = await target.boundingBox();
  expect(sourceBox).not.toBeNull();
  expect(targetBox).not.toBeNull();
  const startX = sourceBox!.x + sourceBox!.width / 2;
  const startY = sourceBox!.y + sourceBox!.height / 2;
  await page.mouse.move(startX, startY);
  await page.mouse.down();
  await page.mouse.move(startX + 8, startY, { steps: 2 });
  await page.mouse.move(targetBox!.x + targetBox!.width / 2, targetBox!.y + targetBox!.height / 2, {
    steps: 20,
  });
  // dnd-kit derives the drop destination from React pointer-move state. Do not
  // release the pointer until that state has observed the intended zone.
  const targetID = await target.getAttribute("data-testid");
  if (targetID?.startsWith("drop-inside-")) {
    // A row marks the re-parent target with its own ring, not a tree class.
    await expect(target.locator("..")).toHaveClass(/ring-primary/);
  } else {
    await expect(target).toHaveClass(/is-over/);
  }
  await page.mouse.up();
}

// The drawer keeps Details and Workflow collapsed in a <details>; open one
// before its controls are used. Its content stays mounted while collapsed.
// A <details> takes no accessible name from its summary, so it is found by it.
function detailsSection(scope: Locator, summary: string) {
  return scope.locator("details").filter({
    has: scope.page().locator("xpath=./summary", { hasText: new RegExp(`^\\s*${summary}\\s*$`) }),
  });
}

async function expandSection(scope: Locator, name: "Details" | "Workflow") {
  const section = detailsSection(scope, name);
  if ((await section.getAttribute("open")) === null) {
    await section.locator("xpath=./summary").click();
  }
  await expect(section).toHaveAttribute("open", "");
}

// An agent the daemon knows, so a closed assignee list or a Work tab can name it.
async function ensureAgent(request: APIRequestContext, name: string) {
  const existing = await request.get(`${daemonURL}/api/agents/${name}`);
  if (existing.ok()) return;
  const response = await request.post(`${daemonURL}/api/agents`, {
    data: { name, image: "bare:latest", cwd: "/tmp" },
  });
  const envelope = await response.json();
  expect(envelope.ok, JSON.stringify(envelope)).toBe(true);
}

async function assertNoLoadFailedToast(page: Page) {
  await expect(page.getByText(/network error: Load failed/i)).toHaveCount(0);
}

const TASKS_PAGE = "/tests/tasks-fixture.html#/tasks";

// The queue, assignee, "Waiting for me" and columns sit in the toolbar's
// Filters menu; a submenu row opens on click.
async function openFiltersSub(page: Page, name: RegExp) {
  await page.getByRole("button", { name: /^Filters/ }).click();
  await page.getByRole("menuitem", { name }).click();
}

async function openManageQueues(page: Page) {
  await openFiltersSub(page, /^Queue/);
  await page.getByRole("menuitem", { name: "Manage queues…" }).click();
}

async function pickAssignee(page: Page, name: string) {
  await openFiltersSub(page, /^Assignee/);
  await page.getByRole("menuitemradio", { name, exact: true }).click();
}

async function toggleWaitingForMe(page: Page) {
  await page.getByRole("button", { name: /^Filters/ }).click();
  await page.getByRole("menuitemcheckbox", { name: "Waiting for me" }).click();
}

test("Tasks production workspace opens, closes, resizes, and restores the detail sheet", async ({
  page,
  request,
}) => {
  let response = await request.post(`${daemonURL}/api/task-queues`, {
    data: { prefix: "RESIZE", name: "Resize browser" },
  });
  expect(response.ok()).toBe(true);
  response = await request.post(`${daemonURL}/api/tasks`, {
    data: {
      queue: "RESIZE",
      title: "Resize detail sheet",
      idempotency_key: "tasks-browser-resize-detail",
    },
  });
  expect(response.ok()).toBe(true);
  const resizeKey = (await response.json()).result.key as string;
  await page.goto(TASKS_PAGE);
  await expect(page.getByLabel("Search tasks")).toBeVisible();
  await expect(page.getByRole("separator", { name: "Resize task details" })).toHaveCount(0);

  await page.getByTestId(`task-row-${resizeKey}`).locator(".task-row-main").click();
  const detailHandle = page.getByRole("separator", { name: "Resize task details" });
  await expect(detailHandle).toBeVisible();

  await expect.poll(async () => (await page.getByRole("dialog").boundingBox())?.width).toBe(720);
  await expect
    .poll(async () => {
      const box = await page.getByRole("dialog").boundingBox();
      return box && box.x + box.width;
      // The sheet is an island: it stops 8px short of the window edge.
    })
    .toBe(1432);

  const detailBox = await detailHandle.boundingBox();
  expect(detailBox).not.toBeNull();
  await page.mouse.move(detailBox!.x + detailBox!.width / 2, detailBox!.y + 60);
  await page.mouse.down();
  await page.mouse.move(detailBox!.x - 62, detailBox!.y + 60, { steps: 8 });
  await page.mouse.up();

  const persisted = await page.evaluate(() =>
    JSON.parse(localStorage.getItem("tasks:workspace:v1") ?? "{}"),
  );
  expect(persisted).toMatchObject({ schemaVersion: 1 });
  expect(persisted.detailWidth).toBeGreaterThan(460);

  await page.setViewportSize({ width: 900, height: 900 });
  await expect(detailHandle).toBeHidden();
  await page.setViewportSize({ width: 1440, height: 900 });
  await expect(detailHandle).toHaveAttribute("aria-valuenow", String(persisted.detailWidth));

  await page.locator('[data-slot="dialog-overlay"]').click({ position: { x: 10, y: 10 } });
  await expect(page.getByRole("dialog")).toHaveCount(0);

  await page.reload();
  await expect(page.getByLabel("Search tasks")).toBeVisible();
  await page.getByTestId(`task-row-${resizeKey}`).locator(".task-row-main").click();
  await expect(page.getByRole("separator", { name: "Resize task details" })).toHaveAttribute(
    "aria-valuenow",
    String(persisted.detailWidth),
  );

  await page.locator('[data-slot="dialog-overlay"]').click({ position: { x: 10, y: 10 } });
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByTestId(`task-row-${resizeKey}`).locator(".task-row-main").click();
  await page.getByRole("dialog").getByRole("button", { name: "Edit title" }).click();
  await page.getByRole("dialog").getByLabel("Title").fill("Unsaved resize detail");
  await page.locator('[data-slot="dialog-overlay"]').click({ position: { x: 10, y: 10 } });
  await expect(
    page.getByText("Are you sure you want to close this task? Unsaved changes will be discarded."),
  ).toBeVisible();
});

test("Tasks toolbar keeps every control in one row at 1024 px", async ({ page }) => {
  await page.setViewportSize({ width: 1024, height: 768 });
  await page.goto(TASKS_PAGE);
  const toolbar = page.locator("[data-toolbar]");
  await expect(toolbar.getByLabel("Search tasks")).toBeVisible();
  await expect(toolbar.getByRole("button", { name: "New task" })).toBeVisible();
  const tops = await toolbar.evaluate((element) =>
    [...element.children].map((child) => Math.round(child.getBoundingClientRect().top)),
  );
  expect(tops.length).toBeGreaterThan(3);
  expect(new Set(tops).size).toBe(1);
});

test("Tasks detail keeps its header reachable under a long description", async ({
  page,
  request,
}) => {
  const description = Array.from({ length: 80 }, (_, index) => `- line ${index + 1}`).join("\n");
  let response = await request.post(`${daemonURL}/api/task-queues`, {
    data: { prefix: "SCROLL", name: "Scroll browser" },
  });
  expect(response.ok()).toBe(true);
  response = await request.post(`${daemonURL}/api/tasks`, {
    data: {
      queue: "SCROLL",
      title: "Long description scroll",
      description,
      idempotency_key: "tasks-browser-long-description",
    },
  });
  expect(response.ok()).toBe(true);
  const key = (await response.json()).result.key as string;
  await page.goto(TASKS_PAGE);
  await page.getByTestId(`task-row-${key}`).locator(".task-row-main").click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText("line 80")).toBeVisible();

  // Only the panel scrolls. A dialog that overflows its own overflow:hidden box
  // can be scrolled by focus or scrollIntoView, but never back by the wheel.
  const overflow = await dialog.evaluate((element) => element.scrollHeight - element.clientHeight);
  expect(overflow).toBeLessThanOrEqual(0);
  await dialog.getByText("Comment order").evaluate((element) => element.scrollIntoView());
  expect(await dialog.evaluate((element) => element.scrollTop)).toBe(0);
  const dialogTop = (await dialog.boundingBox())!.y;
  expect((await dialog.locator("header").first().boundingBox())!.y).toBeGreaterThanOrEqual(
    dialogTop,
  );
});

test("Tasks production workspace lists the agent pools of a queue", async ({ page, request }) => {
  const response = await request.post(`${daemonURL}/api/task-queues`, {
    data: { prefix: "FLOW", name: "Pool browser" },
  });
  expect(response.ok()).toBe(true);

  await page.goto(TASKS_PAGE);
  await openManageQueues(page);
  await page.getByRole("button", { name: /^Queue FLOW/ }).click();
  await page.getByRole("tab", { name: "Pools" }).click();
  await expect(
    page
      .getByRole("region", { name: "Agent pools FLOW" })
      .getByRole("button", { name: "Add pool" }),
  ).toBeVisible();
  await assertNoLoadFailedToast(page);
});

test("Tasks production workspace persists PATCH saves, release fields, and the full tree workflow", async ({
  page,
  request,
}) => {
  test.setTimeout(60_000);
  // The assignee is a closed list of the daemon's agents.
  await ensureAgent(request, "worker-e2e");
  await page.goto(TASKS_PAGE);
  await expect(page.getByLabel("Search tasks")).toBeVisible();

  await openManageQueues(page);
  await page.getByRole("button", { name: "New queue" }).click();
  await page.getByLabel("Queue prefix").fill("test");
  await page.getByLabel("New queue name").fill("Tasks E2E");
  await page.getByRole("button", { name: "Add queue" }).click();
  await expect(page.getByRole("heading", { name: "Tasks E2E" })).toBeVisible();

  await page.getByLabel("Queue name TEST").fill("Tasks E2E updated");
  await page.getByLabel("Queue description TEST").fill("Saved through PATCH from a browser origin");
  await page.getByRole("button", { name: "Save TEST" }).click();
  await expect(page.getByText("Queue updated", { exact: true })).toBeVisible();
  await assertNoLoadFailedToast(page);

  await page.getByRole("button", { name: "Close" }).click();
  await page.getByRole("button", { name: "New task" }).click();
  await page.getByLabel("Task queue").selectOption("TEST");
  await page.getByLabel("Task title").fill("Root task");
  await page.getByRole("button", { name: "Create task" }).click();
  // The daemon mints a random key suffix, so the spec asks it which key it made.
  const rootKey = await keyByTitle(request, "Root task");
  await expect(page.getByTestId(`task-row-${rootKey}`)).toBeVisible();
  await page.getByRole("button", { name: "Close task detail" }).click();

  await page.getByRole("button", { name: `Add child to ${rootKey}` }).click();
  await page.getByLabel("Task title").fill("First child");
  await page.getByRole("button", { name: "Create task" }).click();
  const childKey = await keyByTitle(request, "First child");
  await expect(page.getByTestId(`task-row-${childKey}`)).toBeVisible();
  await page.getByRole("button", { name: "Close task detail" }).click();

  await page.getByRole("button", { name: "New task" }).click();
  await page.getByLabel("Task queue").selectOption("TEST");
  await page.getByLabel("Task title").fill("Second root");
  await page.getByRole("button", { name: "Create task" }).click();
  const secondRootKey = await keyByTitle(request, "Second root");
  await expect(page.getByTestId(`task-row-${secondRootKey}`)).toBeVisible();
  await page.getByRole("button", { name: "Close task detail" }).click();

  await page.getByTestId(`task-row-${rootKey}`).locator(".task-row-main").click();
  const detail = page.locator(".task-detail-panel");
  await expect(detail.getByRole("heading", { name: rootKey })).toBeVisible();
  await detail.getByRole("button", { name: "Edit title" }).click();
  await detail.getByLabel("Title").fill("Root task updated");
  await detail
    .getByRole("textbox", { name: "Description" })
    .fill("Edited from the production Tasks form");
  await expandSection(detail, "Details");
  await detail.getByLabel("Status").selectOption("in_progress");
  await detail.getByLabel("Assignee").selectOption("agent:worker-e2e");
  await detail.getByLabel("Manual block reason").fill("waiting on E2E fixture");
  const taskPatchRequest = page.waitForRequest(
    (request) =>
      request.method() === "PATCH" && request.url() === `${daemonURL}/api/tasks/${rootKey}`,
  );
  await detail.getByRole("button", { name: "Save task" }).click();
  expect((await taskPatchRequest).postDataJSON()).toMatchObject({
    title: "Root task updated",
    description: "Edited from the production Tasks form",
    status: "in_progress",
    assignee: "agent:worker-e2e",
    manual_block_reason: "waiting on E2E fixture",
  });
  await expect(page.getByText("Task updated", { exact: true }).last()).toBeVisible();
  await expect(page.getByTestId(`task-row-${rootKey}`)).toContainText("Root task updated");
  await expect(page.getByTestId(`task-row-${rootKey}`)).toContainText("blocked");
  await expect(page.getByTestId(`task-row-${rootKey}`)).toContainText("worker-e2e");
  await assertNoLoadFailedToast(page);

  await detail.getByLabel("Status").selectOption("wait_customer");
  await detail.getByLabel("Pull request").fill("https://github.com/acme/tariboy/pull/43");
  await detail.getByRole("button", { name: "Save task" }).click();
  await expect(page.getByText("Task updated", { exact: true }).last()).toBeVisible();
  expect(await taskFromAPI(request, rootKey)).toMatchObject({
    status: "wait_customer",
    pull_request: "https://github.com/acme/tariboy/pull/43",
  });

  await page.reload();
  await page.getByTestId(`task-row-${rootKey}`).locator(".task-row-main").click();
  await expect(detail.getByLabel("Status")).toHaveValue("wait_customer");
  await expect(detail.getByLabel("Pull request")).toHaveValue(
    "https://github.com/acme/tariboy/pull/43",
  );
  await expandSection(detail, "Details");
  await detail.getByLabel("Pull request").fill("");
  await detail.getByRole("button", { name: "Save task" }).click();
  await expect(page.getByText("Task updated", { exact: true }).last()).toBeVisible();

  await page.reload();
  await page.getByTestId(`task-row-${rootKey}`).locator(".task-row-main").click();
  await expect(detail.getByLabel("Status")).toHaveValue("wait_customer");
  await expect(detail.getByLabel("Pull request")).toHaveValue("");

  const saved = await taskFromAPI(request, rootKey);
  expect(saved).toMatchObject({
    title: "Root task updated",
    description: "Edited from the production Tasks form",
    status: "wait_customer",
    assignee: "agent:worker-e2e",
    manual_block_reason: "waiting on E2E fixture",
    pull_request: "",
  });

  // A customer question is answered through the ordinary comment form.
  await detail.getByLabel("Ask an agent", { exact: true }).selectOption("user:customer");
  await detail.getByLabel("Comment", { exact: true }).fill("Please confirm the browser workflow");
  await detail.getByRole("button", { name: "Send comment" }).click();
  await expect(detail.getByText("Please confirm the browser workflow")).toBeVisible();

  await page.reload();
  await expect(page.getByLabel("Search tasks")).toBeVisible();
  await toggleWaitingForMe(page);
  await expect(page.getByTestId(`task-row-${rootKey}`)).toBeVisible();
  await page.getByTestId(`task-row-${rootKey}`).locator(".task-row-main").click();
  await detail.getByLabel("Comment", { exact: true }).fill("Confirmed from the customer");
  await detail.getByRole("button", { name: "Send comment" }).click();
  await expect(detail.getByText("Confirmed from the customer")).toBeVisible();

  await detail.getByRole("button", { name: "Close task detail" }).click();
  await toggleWaitingForMe(page);
  await page.getByTestId(`task-row-${rootKey}`).locator(".task-row-main").click();
  await expandSection(detail, "Details");
  await detail.getByLabel("Relation type").selectOption("related");
  await detail.getByLabel("Related task key").fill(secondRootKey);
  await detail.getByRole("button", { name: "Add relation" }).click();
  // The dependency row spells its type and key in separate cells.
  const relationRow = detail.getByRole("button", { name: `Remove relation to ${secondRootKey}` });
  await expect(relationRow).toBeVisible();
  await expect(detail.getByText("related", { exact: true })).toBeVisible();
  await relationRow.click();
  await expect(relationRow).toHaveCount(0);
  await detail.getByRole("button", { name: "Close task detail" }).click();

  const criticalRoot = await createTask(request, { title: "Critical root", priority: "P0" });
  const secondCriticalRoot = await createTask(request, {
    title: "Second critical root",
    priority: "P0",
  });
  const lowRoot = await createTask(request, { title: "Low root", priority: "P3" });
  const criticalChild = await createTask(request, {
    title: "Critical child",
    priority: "P0",
    parent_key: rootKey,
  });
  const lowChild = await createTask(request, {
    title: "Low child",
    priority: "P3",
    parent_key: rootKey,
  });
  const expandRoot = page.getByRole("button", { name: `Expand ${rootKey}` });
  if (await expandRoot.isVisible()) await expandRoot.click();
  await expect(page.getByTestId(`task-row-${lowChild.key}`)).toBeVisible({ timeout: 10_000 });

  let ordered = await visibleTaskKeys(page);
  expect(ordered.indexOf(criticalRoot.key)).toBeLessThan(ordered.indexOf(rootKey));
  expect(ordered.indexOf(rootKey)).toBeLessThan(ordered.indexOf(lowRoot.key));
  expect(ordered.indexOf(criticalChild.key)).toBeLessThan(ordered.indexOf(childKey));
  expect(ordered.indexOf(childKey)).toBeLessThan(ordered.indexOf(lowChild.key));
  await expect(
    page.getByTestId(`task-row-${criticalRoot.key}`).getByLabel("P0 Critical"),
  ).toHaveText("P0");
  await expect(page.getByTestId(`task-row-${lowRoot.key}`).getByLabel("P3 Low")).toHaveText("P3");

  await pointerDrag(
    page,
    page.getByRole("button", { name: `Move ${secondCriticalRoot.key}` }),
    page.getByTestId(`drop-before-${criticalRoot.key}`),
  );
  await expect
    .poll(async () => {
      const second = await taskFromAPI(request, secondCriticalRoot.key);
      const first = await taskFromAPI(request, criticalRoot.key);
      return second.position < first.position;
    })
    .toBe(true);

  const lowPosition = (await taskFromAPI(request, lowRoot.key)).position;
  await pointerDrag(
    page,
    page.getByRole("button", { name: `Move ${lowRoot.key}` }),
    page.getByTestId(`drop-before-${criticalRoot.key}`),
  );
  await expect
    .poll(async () => (await taskFromAPI(request, lowRoot.key)).position)
    .toBe(lowPosition);
  ordered = await visibleTaskKeys(page);
  expect(ordered.indexOf(criticalRoot.key)).toBeLessThan(ordered.indexOf(lowRoot.key));

  await pointerDrag(
    page,
    page.getByRole("button", { name: `Move ${lowRoot.key}` }),
    page.getByTestId(`drop-inside-${rootKey}`),
  );
  await expect.poll(async () => (await taskFromAPI(request, lowRoot.key)).parent_key).toBe(rootKey);
  ordered = await visibleTaskKeys(page);
  expect(ordered.indexOf(childKey)).toBeLessThan(ordered.indexOf(lowChild.key));
  expect(ordered.indexOf(childKey)).toBeLessThan(ordered.indexOf(lowRoot.key));

  await pointerDrag(
    page,
    page.getByRole("button", { name: `Move ${secondRootKey}` }),
    page.getByTestId(`drop-inside-${rootKey}`),
  );
  await expect
    .poll(async () => (await taskFromAPI(request, secondRootKey)).parent_key)
    .toBe(rootKey);
  await expect(page.getByTestId(`task-row-${secondRootKey}`)).toBeVisible();

  await pointerDrag(
    page,
    page.getByRole("button", { name: `Move ${secondRootKey}` }),
    page.getByTestId(`drop-before-${childKey}`),
  );
  await expect
    .poll(async () => {
      const moved = await taskFromAPI(request, secondRootKey);
      const first = await taskFromAPI(request, childKey);
      return moved.position < first.position;
    })
    .toBe(true);

  await pickAssignee(page, "My tasks");
  await openFiltersSub(page, /^Assignee/);
  await expect(page.getByRole("menuitemradio", { name: "My tasks" })).toHaveAttribute(
    "aria-checked",
    "true",
  );
  await page.getByRole("menuitemradio", { name: "Anyone", exact: true }).click();
  await page.getByLabel("Search tasks").fill("Root task updated");
  await expect(page.getByTestId(`task-row-${rootKey}`)).toBeVisible();
  await expect(page.getByTestId(`task-row-${secondRootKey}`)).toHaveCount(0);
  await page.getByLabel("Search tasks").fill("");

  const createResponse = await request.post(`${daemonURL}/api/tasks`, {
    data: {
      queue: "TEST",
      title: "Realtime injected task",
      idempotency_key: "tasks-browser-e2e-realtime",
    },
  });
  expect(createResponse.ok()).toBe(true);
  await expect(page.getByText("Realtime injected task")).toBeVisible({ timeout: 10_000 });

  await page.reload();
  await expect(page.getByText("Realtime injected task")).toBeVisible();
  await expect(page.getByText("Root task updated")).toBeVisible();
  await assertNoLoadFailedToast(page);
});

const approvalWorkflow = `schema_version: 1
name: wfui
workflow_version: 0.1.0
initial_status: approval
statuses:
  - id: approval
    owner: customer
    instructions: ./statuses/approval.md
    transitions:
      - { on: approved, to: done }
  - id: done
    terminal: true
`;

test("Tasks production workspace drives a workflow queue: bind, approve, secrets", async ({
  page,
  request,
}) => {
  test.setTimeout(90_000);
  const started = Date.now();
  const step = (name: string) =>
    console.log(`[workflow-ui] ${((Date.now() - started) / 1000).toFixed(1)}s ${name}`);

  // Step 1: publish the fixture image through the daemon's build route, then
  // bind the queue from the queue settings.
  const source = mkdtempSync(path.join(tmpdir(), "tasks-e2e-workflow-"));
  mkdirSync(path.join(source, "statuses"));
  writeFileSync(path.join(source, "Workflowfile.yaml"), approvalWorkflow);
  writeFileSync(
    path.join(source, "statuses", "approval.md"),
    "Approve the work when it looks right.\n",
  );
  let response = await request.post(`${daemonURL}/api/workflow-images/build`, {
    data: { path: source, wait: true },
  });
  rmSync(source, { recursive: true, force: true });
  expect(response.ok(), "step 1: the fixture workflow image must build").toBe(true);
  response = await request.post(`${daemonURL}/api/task-queues`, {
    data: { prefix: "WFUI", name: "Workflow browser" },
  });
  expect(response.ok()).toBe(true);
  response = await request.post(`${daemonURL}/api/task-queues`, {
    data: { prefix: "WFLEX", name: "Flexible browser" },
  });
  expect(response.ok()).toBe(true);

  await page.goto(TASKS_PAGE);
  await expect(page.getByLabel("Search tasks")).toBeVisible();
  await openManageQueues(page);
  await page.getByRole("button", { name: /^Queue WFUI/ }).click();
  await page.getByRole("tab", { name: "Workflow" }).click();
  const settings = page.getByRole("region", { name: "Workflow WFUI" });
  await expect(settings.getByText("No workflow")).toBeVisible();
  await settings.getByLabel("Workflow image").selectOption("wfui:latest");
  await settings.getByRole("button", { name: "Bind", exact: true }).click();
  await expect(
    settings.getByTestId("binding"),
    "step 1: the binding shows after Bind",
  ).toContainText("wfui");
  step("1 published and bound");

  // Step 2: a task in the bound queue.
  await page.getByRole("button", { name: "Close", exact: true }).click();
  await page.getByRole("button", { name: "New task" }).click();
  await page.getByLabel("Task queue").selectOption("WFUI");
  await page.getByLabel("Task title").fill("Approve the plan");
  await page.getByRole("button", { name: "Create task" }).click();
  const key = await keyByTitle(request, "Approve the plan");
  expect((await taskFromAPI(request, key)).status).toBe("approval");
  step("2 task created");

  // Step 3: the list shows the label and the waiting indicator.
  await page.getByRole("button", { name: "Close task detail" }).click();
  const row = page.getByTestId(`task-row-${key}`);
  await expect(row, "step 3: the row shows the workflow status label").toContainText("Approval");
  await expect(row, "step 3: the row shows who is awaited").toContainText("customer");
  step("3 list label and indicator");

  // Step 4: the drawer shows the workflow panel and no status or assignee control.
  await row.locator(".task-row-main").click();
  const detail = page.locator(".task-detail-panel");
  // The initial status is a start, not a request: no card, the panel decides.
  await expect(detail.getByRole("region", { name: "Needs you" })).toHaveCount(0);
  await expandSection(detail, "Workflow");
  await expect(detail.getByText("wfui@0.1.0"), "step 4: the panel header").toBeVisible();
  await expect(detail.getByText("status approval")).toBeVisible();
  await expect(detail.getByRole("button", { name: "approved → done" })).toBeEnabled();
  await expect(
    detail.getByRole("combobox", { name: "Status", exact: true }),
    "step 4: no status select",
  ).toHaveCount(0);
  await expect(
    detail.getByRole("combobox", { name: "Assignee", exact: true }),
    "step 4: no assignee control",
  ).toHaveCount(0);
  step("4 drawer panel");

  // Step 5: approve; the task closes.
  await detail.getByRole("button", { name: "approved → done" }).click();
  await expect(
    detail.getByText("status done"),
    "step 5: the panel shows the terminal status",
  ).toBeVisible({ timeout: 15_000 });
  await expect.poll(async () => (await taskFromAPI(request, key)).status).toBe("done");
  await detail.getByRole("button", { name: "Close task detail" }).click();
  await page.getByRole("radio", { name: "Done", exact: true }).click();
  await expect(
    page.getByTestId(`task-row-${key}`),
    "step 5: the closed task is listed",
  ).toContainText("Done");
  step("5 approved and closed");

  // Step 5b: the customer moves one task by hand and cancels another. The
  // confirmation opens above the detail sheet, so its buttons take the click.
  const createWorkflowTask = async (title: string) => {
    const created = await request.post(`${daemonURL}/api/tasks`, {
      data: { queue: "WFUI", title, idempotency_key: `tasks-browser-${title}` },
    });
    expect(created.ok()).toBe(true);
    return (await created.json()).result.key as string;
  };
  const moveKey = await createWorkflowTask("Move by hand");
  const cancelKey = await createWorkflowTask("Cancel by hand");
  await page.getByRole("radio", { name: "Active", exact: true }).click();
  await page.getByTestId(`task-row-${moveKey}`).locator(".task-row-main").click();
  await expandSection(detail, "Workflow");
  await detail.getByRole("button", { name: "Move to status…" }).click();
  await detail.getByLabel("Target status").selectOption("done");
  await detail.getByLabel("Reason", { exact: true }).fill("closed by hand");
  await detail.getByRole("button", { name: "Move", exact: true }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Move task" })
    .click({ timeout: 5_000 });
  await expect
    .poll(async () => (await taskFromAPI(request, moveKey)).status, "step 5b: the move lands")
    .toBe("done");
  await detail.getByRole("button", { name: "Close task detail" }).click();
  await page.getByTestId(`task-row-${cancelKey}`).locator(".task-row-main").click();
  await expandSection(detail, "Workflow");
  await detail.getByRole("button", { name: "Cancel task" }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Cancel task" })
    .click({ timeout: 5_000 });
  await expect
    .poll(
      async () => ((await taskFromAPI(request, cancelKey)) as { category?: string }).category,
      "step 5b: the cancel lands",
    )
    .toBe("cancelled");
  await detail.getByRole("button", { name: "Close task detail" }).click();
  step("5b moved and cancelled by hand");

  // Step 6: queue settings show the binding and manage secrets.
  await openManageQueues(page);
  await page.getByRole("button", { name: /^Queue WFUI/ }).click();
  await page.getByRole("tab", { name: "Workflow" }).click();
  const queueSettings = page.getByRole("region", { name: "Workflow WFUI" });
  await expect(queueSettings.getByTestId("binding")).toContainText(/wfui\s+0\.1\.0\s+[0-9a-f]{12}/);
  await page.getByRole("tab", { name: "Environment" }).click();
  const secrets = page.getByRole("region", { name: "Secrets WFUI" });
  await expect(secrets.getByText("No secrets.")).toBeVisible();
  await secrets.getByLabel("Secret key").fill("GH_TOKEN");
  await secrets.getByLabel("Secret value").fill("ghp_browser-secret-value");
  await secrets.getByRole("button", { name: "Set", exact: true }).click();
  await expect(
    secrets.getByText("GH_TOKEN", { exact: true }),
    "step 6: the key is listed",
  ).toBeVisible();
  await expect(page.locator("body"), "step 6: no value on the page").not.toContainText(
    "ghp_browser-secret-value",
  );
  await expect(secrets.getByLabel("Secret value")).toHaveValue("");
  // After a reload the key is read back from the daemon, and the value never is.
  await page.reload();
  await expect(page.getByLabel("Search tasks")).toBeVisible();
  await openManageQueues(page);
  await page.getByRole("button", { name: /^Queue WFUI/ }).click();
  await page.getByRole("tab", { name: "Environment" }).click();
  const reloaded = page.getByRole("region", { name: "Secrets WFUI" });
  await expect(
    reloaded.getByText("GH_TOKEN", { exact: true }),
    "step 6: the key is listed after a reload",
  ).toBeVisible();
  expect(await page.content(), "step 6: no value in the reloaded page").not.toContain(
    "ghp_browser-secret-value",
  );
  await reloaded.getByRole("button", { name: "Remove GH_TOKEN" }).click();
  await page.getByRole("alertdialog").getByRole("button", { name: "Remove secret" }).click();
  await expect(reloaded.getByText("No secrets."), "step 6: the removed key is gone").toBeVisible();
  step("6 binding and secrets");

  // Step 7: a flexible queue keeps its status select and its assignee control.
  await page.getByRole("button", { name: "Close", exact: true }).click();
  response = await request.post(`${daemonURL}/api/tasks`, {
    data: {
      queue: "WFLEX",
      title: "Flexible stays flexible",
      idempotency_key: "tasks-browser-wfui-flex",
    },
  });
  expect(response.ok()).toBe(true);
  const flexKey = (await response.json()).result.key as string;
  await page.getByRole("radio", { name: "Active", exact: true }).click();
  await page.getByTestId(`task-row-${flexKey}`).locator(".task-row-main").click();
  const flexDetail = page.locator(".task-detail-panel");
  await expandSection(flexDetail, "Details");
  await expect(
    flexDetail.getByRole("combobox", { name: "Status", exact: true }),
    "step 7: flexible task keeps its status select",
  ).toBeVisible();
  await expect(
    flexDetail.getByRole("combobox", { name: "Assignee", exact: true }),
    "step 7: flexible task keeps its assignee control",
  ).toBeVisible();
  // The header menu opens above the detail sheet, so its item takes the click.
  await flexDetail.getByRole("button", { name: "Task actions" }).click();
  await page.getByRole("menuitem", { name: "Move to another server…" }).click({ timeout: 5_000 });
  await expect(
    page.getByRole("dialog", { name: "Move to another server" }),
    "step 7: the menu item opens its dialog",
  ).toBeVisible();
  step("7 flexible unchanged");
});

// A workflow with a pool status, a customer approval that can send the task
// back, and a script status whose watch fails at once and pauses the task.
const problemWorkflow = `schema_version: 1
name: wfui2
workflow_version: 0.1.0
initial_status: approval
limits: { script_failures: 1 }
statuses:
  - id: draft
    owner: { pool: writers }
    instructions: ./statuses/draft.md
    transitions:
      - on: drafted
        to: approval
        checks:
          - script: ./scripts/check.sh
            description: The draft has a heading.
  - id: approval
    owner: customer
    instructions: ./statuses/approval.md
    transitions:
      - { on: approved, to: probe }
      - { on: rework, to: draft }
  - id: probe
    owner: script
    watch:
      script: ./scripts/fail.sh
      every: 1s
      description: Probes the deployment.
    transitions:
      - { on: probed, to: done }
  - id: done
    terminal: true
`;

test("Tasks production workspace explains a workflow task's problem, maps its steps, and returns it to the agent", async ({
  page,
  request,
}) => {
  test.setTimeout(90_000);
  const started = Date.now();
  const step = (name: string) =>
    console.log(`[workflow-problem-ui] ${((Date.now() - started) / 1000).toFixed(1)}s ${name}`);

  // Step 1: publish the image with its scripts, give the pool an agent, and bind the queue.
  const source = mkdtempSync(path.join(tmpdir(), "tasks-e2e-workflow-problem-"));
  mkdirSync(path.join(source, "statuses"));
  mkdirSync(path.join(source, "scripts"));
  writeFileSync(path.join(source, "Workflowfile.yaml"), problemWorkflow);
  writeFileSync(path.join(source, "statuses", "draft.md"), "Write the draft.\n");
  writeFileSync(
    path.join(source, "statuses", "approval.md"),
    "Approve the draft, or send it back.\n",
  );
  writeFileSync(path.join(source, "scripts", "check.sh"), "#!/bin/sh\nexit 0\n");
  writeFileSync(
    path.join(source, "scripts", "fail.sh"),
    "#!/bin/sh\necho 'the probe answered 502' >&2\nexit 1\n",
  );
  chmodSync(path.join(source, "scripts", "check.sh"), 0o755);
  chmodSync(path.join(source, "scripts", "fail.sh"), 0o755);
  let response = await request.post(`${daemonURL}/api/workflow-images/build`, {
    data: { path: source, wait: true },
  });
  rmSync(source, { recursive: true, force: true });
  expect(response.ok(), "step 1: the fixture workflow image must build").toBe(true);
  response = await request.post(`${daemonURL}/api/task-queues`, {
    data: { prefix: "WFPR", name: "Workflow problems" },
  });
  expect(response.ok()).toBe(true);
  const agent = await request.post(`${daemonURL}/api/agents`, {
    data: { name: "wfpr-writer", image: "bare:latest", cwd: "/tmp" },
  });
  expect((await agent.json()).ok, "step 1: a pool needs an existing agent").toBe(true);
  response = await request.patch(`${daemonURL}/api/task-queues/WFPR/pools/writers`, {
    data: { agents: ["wfpr-writer"], revision: 0, idempotency_key: "wfpr-pool" },
  });
  expect(response.ok(), "step 1: the pool binds").toBe(true);
  response = await request.put(`${daemonURL}/api/task-queues/WFPR/workflow`, {
    data: { ref: "wfui2:latest", revision: 0 },
  });
  expect(response.ok(), "step 1: the queue binds the image").toBe(true);
  response = await request.post(`${daemonURL}/api/tasks`, {
    data: { queue: "WFPR", title: "Probe the release", idempotency_key: "tasks-browser-wfpr" },
  });
  expect(response.ok()).toBe(true);
  const key = (await response.json()).result.key as string;
  expect((await taskFromAPI(request, key)).status).toBe("approval");
  step("1 published, bound, task created");

  // Step 2: the row counts the step; the panel maps the workflow and opens a step.
  await page.goto(TASKS_PAGE);
  await expect(page.getByLabel("Search tasks")).toBeVisible();
  const row = page.getByTestId(`task-row-${key}`);
  await expect(row.getByTitle("Step 2 of 4"), "step 2: the row counts the step").toHaveText("2/4");
  await row.locator(".task-row-main").click();
  const detail = page.locator(".task-detail-panel");
  await expandSection(detail, "Workflow");
  await expect(detail.getByText("wfui2@0.1.0")).toBeVisible();
  await expect(detail.getByText("step 2 of 4")).toBeVisible();
  const steps = detail.getByRole("list", { name: "Workflow steps" });
  await expect(steps.getByRole("button")).toHaveCount(4);
  await expect(steps.getByRole("button", { name: /^approval/ })).toHaveAttribute(
    "aria-current",
    "step",
  );
  await steps.getByRole("button", { name: /^draft/ }).click();
  await expect(
    detail.getByTestId("step-detail"),
    "step 2: a step opens to its gates",
  ).toContainText("The draft has a heading.");
  await expect(detail.getByTestId("step-detail")).toContainText("owned by pool writers");
  step("2 step counter and stepper");

  // Step 3: return the task to the agent through the declared outcome.
  await detail.getByRole("button", { name: "Return to agent…" }).click();
  await expect(detail.getByLabel("Return to status")).toHaveValue("draft");
  await expect(detail.getByText("declares rework")).toBeVisible();
  await detail.getByLabel("Message to the agent").fill("Please add a heading");
  await detail.getByRole("button", { name: "Return to agent", exact: true }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Return to agent" })
    .click({ timeout: 5_000 });
  await expect
    .poll(async () => (await taskFromAPI(request, key)).status, "step 3: the outcome lands")
    .toBe("draft");
  response = await request.get(`${daemonURL}/api/tasks/${key}/workflow`);
  const view = (await response.json()).result as {
    visits: { status: string; message?: string; left_at?: string }[];
  };
  expect(
    view.visits.find((visit) => visit.status === "approval" && visit.left_at)?.message,
    "step 3: the message is the transition message",
  ).toBe("Please add a heading");
  // The pool's agent is stopped, so nobody takes the task: the panel says so.
  await expect(
    detail.getByText("Waiting for an agent"),
    "step 3: the problem block explains the open pool status",
  ).toBeVisible();
  await expect(detail.getByText("step 1 of 4")).toBeVisible();
  await expect(row.getByTitle("Step 1 of 4")).toHaveText("1/4");
  step("3 returned by outcome");

  // Step 4: a move into the script status; its watch fails and pauses the task.
  await detail.getByRole("button", { name: "Move to status…" }).click();
  await detail.getByLabel("Target status").selectOption("probe");
  await detail.getByLabel("Reason", { exact: true }).fill("probe it");
  await detail.getByRole("button", { name: "Move", exact: true }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Move task" })
    .click({ timeout: 5_000 });
  await expect
    .poll(
      async () =>
        ((await taskFromAPI(request, key)) as { workflow_paused_reason?: string })
          .workflow_paused_reason,
      { timeout: 20_000, message: "step 4: the failed watch pauses the task" },
    )
    .toBe("script_failures");
  await expect(
    row.getByText("paused", { exact: true }),
    "step 4: the row shows the pause",
  ).toBeVisible();
  await expect(
    detail.getByText(/Scripts failed too many times/),
    "step 4: the banner says why",
  ).toBeVisible();
  const runs = detail.getByText("Script runs").locator("xpath=ancestor::section[1]");
  await expect(
    runs.locator("li[data-tone='danger']").first(),
    "step 4: the failed run reads as one",
  ).toContainText("failure");
  step("4 watch failed and paused");

  // Step 5: return the paused task to the agent by a move; the pause ends.
  await detail.getByRole("button", { name: "Return to agent…" }).click();
  await expect(detail.getByLabel("Return to status")).toHaveValue("draft");
  await expect(detail.getByText("operator move")).toBeVisible();
  await detail
    .getByLabel("Message to the agent")
    .fill("The probe answers 502; fix the deployment first");
  await detail.getByRole("button", { name: "Return to agent", exact: true }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Return to agent" })
    .click({ timeout: 5_000 });
  await expect
    .poll(async () => (await taskFromAPI(request, key)).status, "step 5: the move lands")
    .toBe("draft");
  expect(
    ((await taskFromAPI(request, key)) as { workflow_paused_reason?: string })
      .workflow_paused_reason ?? "",
    "step 5: the pause is over",
  ).toBe("");
  // The Workflow section follows workflow_attention: it opened with the pause
  // and closes, even under the user, when the drawer reads that the pause
  // ended. Open it again until it stays open.
  await expect(async () => {
    await expandSection(detail, "Workflow");
    await expect(detail.getByText("Waiting for an agent")).toBeVisible({ timeout: 1_000 });
  }).toPass();
  step("5 returned by move");
});

const WORK_AGENT = "work-tab-agent";
const workPage = (query = "") =>
  `/tests/tasks-fixture.html#/agents/local/${WORK_AGENT}/work${query}`;

// The agent is created stopped, so the composer offers "Start & give". The
// suite never starts an agent: the start request is answered here with a
// refusal, and the task stays created as the composer promises.
async function stubAgentStart(page: Page) {
  const starts: string[] = [];
  await page.route(`${daemonURL}/api/agents/${WORK_AGENT}/start`, async (route) => {
    starts.push(route.request().method());
    await route.fulfill({
      status: 409,
      json: {
        ok: false,
        error: { code: "e2e_no_start", message: "the browser suite starts no agent" },
      },
    });
  });
  return starts;
}

async function giveTask(page: Page, title: string) {
  const composer = page.getByRole("textbox", { name: `New task for ${WORK_AGENT}` });
  await expect(composer).toBeVisible();
  await expect(composer).toHaveAttribute("placeholder", `Give ${WORK_AGENT} a task…`);
  await composer.fill(title);
  await composer.press("Enter");
  await expect(composer).toHaveValue("");
}

test("Agent Work tab gives a task in a flexible queue to the agent", async ({ page, request }) => {
  await ensureAgent(request, WORK_AGENT);
  const starts = await stubAgentStart(page);
  await page.goto(workPage());
  await expect(page.getByRole("heading", { name: WORK_AGENT })).toBeVisible();
  // The Work tab is the composer and the sections, not the Tasks toolbar.
  await expect(page.getByLabel("Search tasks")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Start & give" })).toBeVisible();

  await giveTask(page, "Write the release notes from the Work tab");
  const key = await keyByTitle(request, "Write the release notes from the Work tab");
  expect(await taskFromAPI(request, key)).toMatchObject({
    assignee: `agent:${WORK_AGENT}`,
    priority: "P2",
    status: "open",
  });
  // A stopped agent is started after the create; the stubbed refusal says so.
  await expect.poll(() => starts).toEqual(["POST"]);
  await expect(page.getByRole("alert")).toContainText(
    `${key} is created, but the agent did not start`,
  );

  const queued = page.getByRole("region", { name: /^Queued \(\d+\)$/ });
  const row = queued.getByRole("button", { name: new RegExp(`^${key}`) });
  await expect(row).toContainText("Write the release notes from the Work tab");
  await expect(row).toContainText("Open");
  await expect(page.getByRole("region", { name: /^In progress \(\d+\)$/ })).toBeVisible();
  await expect(page.getByRole("searchbox", { name: "Search done tasks" })).toBeVisible();

  // A row opens the task drawer.
  await row.click();
  await expect(page.getByRole("dialog", { name: key })).toBeVisible();
});

// A plan written in a pool status, then approved by the customer.
const planWorkflow = `schema_version: 1
name: wfplan
workflow_version: 0.1.0
initial_status: draft
artifacts:
  - { name: plan, description: The plan to approve. }
statuses:
  - id: draft
    owner: { pool: planners }
    instructions: ./statuses/draft.md
    transitions:
      - { on: planned, to: approval, requires: [plan] }
  - id: approval
    owner: customer
    instructions: ./statuses/approval.md
    transitions:
      - { on: approved, to: build }
      - { on: rework, to: draft }
  - id: build
    owner: { pool: planners }
    instructions: ./statuses/build.md
    transitions:
      - { on: built, to: done }
  - id: done
    terminal: true
`;

async function bindPlanQueue(request: APIRequestContext) {
  const source = mkdtempSync(path.join(tmpdir(), "tasks-e2e-workflow-plan-"));
  mkdirSync(path.join(source, "statuses"));
  writeFileSync(path.join(source, "Workflowfile.yaml"), planWorkflow);
  for (const status of ["draft", "approval", "build"]) {
    writeFileSync(path.join(source, "statuses", `${status}.md`), `Do the ${status} step.\n`);
  }
  let response = await request.post(`${daemonURL}/api/workflow-images/build`, {
    data: { path: source, wait: true },
  });
  rmSync(source, { recursive: true, force: true });
  expect(response.ok(), "the plan workflow image must build").toBe(true);
  response = await request.post(`${daemonURL}/api/task-queues`, {
    data: { prefix: "WPLAN", name: "Plan browser" },
  });
  expect(response.ok()).toBe(true);
  response = await request.patch(`${daemonURL}/api/task-queues/WPLAN/pools/planners`, {
    data: { agents: [WORK_AGENT], revision: 0, idempotency_key: "wplan-pool" },
  });
  expect(response.ok(), "the pool binds").toBe(true);
  response = await request.put(`${daemonURL}/api/task-queues/WPLAN/workflow`, {
    data: { ref: "wfplan:latest", revision: 0 },
  });
  expect(response.ok(), "the queue binds the image").toBe(true);
}

// A bare agent never holds a pool status, so the daemon cannot hand the plan
// step to it: the plan is set and the task moved by the customer instead.
async function readyForApproval(request: APIRequestContext, key: string) {
  let response = await request.put(`${daemonURL}/api/tasks/${key}/artifacts/plan`, {
    data: { value: "## Plan\n\n1. Parse the changelog\n2. Publish the notes\n" },
  });
  expect(response.ok(), "the customer sets the plan").toBe(true);
  response = await request.post(`${daemonURL}/api/tasks/${key}/workflow/move`, {
    data: { to: "approval", reason: "the plan is ready" },
  });
  expect(response.ok(), "the task moves to approval").toBe(true);
}

test("Agent Work tab dispatches a bound-queue task to its workflow and approves its plan", async ({
  page,
  request,
}) => {
  test.setTimeout(60_000);
  await ensureAgent(request, WORK_AGENT);
  await bindPlanQueue(request);
  await stubAgentStart(page);
  await page.goto(workPage());
  await expect(page.getByRole("heading", { name: WORK_AGENT })).toBeVisible();

  // Step 1: pick the bound queue in Details; the composer says who takes it.
  await page.getByRole("button", { name: "Details", exact: true }).click();
  // The label wraps the select, so its text includes the chosen option: match by role.
  await page.getByRole("combobox", { name: "Queue", exact: true }).selectOption("WPLAN");
  await expect(page.getByRole("combobox", { name: "Priority", exact: true })).toHaveValue("P2");
  await expect(
    page.getByText("Dispatched by the wfplan workflow; a pool member takes it"),
  ).toBeVisible();
  await giveTask(page, "Plan the release notes");
  const key = await keyByTitle(request, "Plan the release notes");
  const created = await taskFromAPI(request, key);
  expect(created.assignee, "step 1: a bound queue takes no assignee").toBe("");
  expect(created.status).toBe("draft");
  const remembered = await page.evaluate(() =>
    JSON.parse(localStorage.getItem("tasks:new-task-queue-by-agent:v1") ?? "{}"),
  );
  expect(Object.values(remembered)).toContainEqual({ [WORK_AGENT]: "WPLAN" });

  // Step 2: the dispatched line opens the task.
  const dispatched = page.getByRole("button", { name: `${key} dispatched by the wfplan workflow` });
  await dispatched.click();
  await expect(page.getByRole("dialog", { name: key })).toBeVisible();

  // Step 3: in the customer status the drawer pins the card with the plan. The
  // task is not the agent's (see readyForApproval), so the card is checked in
  // the drawer, not under Needs you.
  await readyForApproval(request, key);
  await page.goto(workPage(`?task=${key}`));
  const drawer = page.getByRole("dialog", { name: key });
  const card = drawer.getByRole("region", { name: "Workflow decision" });
  await expect(card).toContainText("Waiting for your decision");
  await card.getByRole("button", { name: "View plan" }).click();
  const plan = detailsSection(drawer, "plan");
  await expect(plan).toHaveAttribute("open", "");
  await expect(plan.getByRole("heading", { name: "Plan" })).toBeVisible();
  await expect(plan.getByText("Publish the notes")).toBeVisible();
  await expect(card.getByRole("button", { name: "rework → draft" })).toBeEnabled();

  // Step 4: approve with a message; the task moves on.
  await card.getByLabel("Outcome message").fill("Go ahead");
  await card.getByRole("button", { name: "approved → build" }).click();
  await expect.poll(async () => (await taskFromAPI(request, key)).status).toBe("build");
  const view = (await (await request.get(`${daemonURL}/api/tasks/${key}/workflow`)).json())
    .result as { visits: { status: string; message?: string; left_at?: string }[] };
  expect(
    view.visits.find((visit) => visit.status === "approval" && visit.left_at)?.message,
    "step 4: the message is the transition message",
  ).toBe("Go ahead");
  await expect(card).toContainText("The agent continues");
});
