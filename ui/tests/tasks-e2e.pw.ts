import { expect, test, type APIRequestContext, type Page } from "playwright/test";

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

async function visibleTaskKeys(page: Page) {
  return page.locator("[data-testid^='task-row-']").evaluateAll((rows) =>
    rows.map((row) => row.getAttribute("data-testid")?.replace("task-row-", "")),
  );
}

async function pointerDrag(page: Page, source: ReturnType<Page["getByRole"]>, target: ReturnType<Page["getByTestId"]>) {
  const sourceBox = await source.boundingBox();
  const targetBox = await target.boundingBox();
  expect(sourceBox).not.toBeNull();
  expect(targetBox).not.toBeNull();
  const startX = sourceBox!.x + sourceBox!.width / 2;
  const startY = sourceBox!.y + sourceBox!.height / 2;
  await page.mouse.move(startX, startY);
  await page.mouse.down();
  await page.mouse.move(startX + 8, startY, { steps: 2 });
  await page.mouse.move(targetBox!.x + targetBox!.width / 2, targetBox!.y + targetBox!.height / 2, { steps: 20 });
  // dnd-kit derives the drop destination from React pointer-move state. Do not
  // release the pointer until that state has observed the intended zone.
  const targetID = await target.getAttribute("data-testid");
  if (targetID?.startsWith("drop-inside-")) {
    await expect(target.locator("..")).toHaveClass(/is-drop-target/);
  } else {
    await expect(target).toHaveClass(/is-over/);
  }
  await page.mouse.up();
}

async function assertNoLoadFailedToast(page: Page) {
  await expect(page.getByText(/network error: Load failed/i)).toHaveCount(0);
}

test("Tasks production workspace resizes and restores both side panels", async ({ page }) => {
  await page.goto("/tests/tasks-fixture.html#/servers/local/tasks");
  const navigationHandle = page.getByRole("separator", { name: "Resize task navigation" });
  const detailHandle = page.getByRole("separator", { name: "Resize task details" });
  await expect(navigationHandle).toBeVisible();
  await expect(detailHandle).toBeVisible();

  const navigationBox = await navigationHandle.boundingBox();
  const detailBox = await detailHandle.boundingBox();
  expect(navigationBox).not.toBeNull();
  expect(detailBox).not.toBeNull();
  await page.mouse.move(navigationBox!.x + navigationBox!.width / 2, navigationBox!.y + 60);
  await page.mouse.down();
  await page.mouse.move(navigationBox!.x + 74, navigationBox!.y + 60, { steps: 8 });
  await page.mouse.up();
  await page.mouse.move(detailBox!.x + detailBox!.width / 2, detailBox!.y + 60);
  await page.mouse.down();
  await page.mouse.move(detailBox!.x - 62, detailBox!.y + 60, { steps: 8 });
  await page.mouse.up();

  const persisted = await page.evaluate(() => JSON.parse(localStorage.getItem("tasks:workspace:v1") ?? "{}"));
  expect(persisted).toMatchObject({ schemaVersion: 1 });
  expect(persisted.navigationWidth).toBeGreaterThan(260);
  expect(persisted.detailWidth).toBeGreaterThan(460);

  await page.setViewportSize({ width: 900, height: 900 });
  await expect(navigationHandle).toBeHidden();
  await expect(detailHandle).toBeHidden();
  await page.setViewportSize({ width: 1440, height: 900 });
  await expect(navigationHandle).toHaveAttribute("aria-valuenow", String(persisted.navigationWidth));
  await expect(detailHandle).toHaveAttribute("aria-valuenow", String(persisted.detailWidth));

  await page.reload();
  await expect(page.getByRole("separator", { name: "Resize task navigation" }))
    .toHaveAttribute("aria-valuenow", String(persisted.navigationWidth));
  await expect(page.getByRole("separator", { name: "Resize task details" }))
    .toHaveAttribute("aria-valuenow", String(persisted.detailWidth));
});

test("Tasks production workspace publishes and selects a workflow version", async ({ page, request }) => {
  const call = async (method: "post" | "patch" | "put", path: string, data: unknown) => {
    const response = await request[method](`${daemonURL}${path}`, { data })
    expect(response.ok()).toBe(true)
    return (await response.json()).result
  }
  await call("post", "/api/task-queues", { prefix: "FLOW", name: "Workflow browser" })
  const definition = {
    name: "browser-flow", version: 1, initial_status: "work",
    statuses: [
      { id: "work", requirements: [{ id: "implementation", pool: "workers", dispatch: "claim_one", inputs: [], produces: ["result"], outcomes: ["done"] }], transitions: [{ when: "implementation.done", to: "done" }] },
      { id: "done", requirements: [], transitions: [], terminal: true },
    ],
  }

  await page.goto("/tests/tasks-fixture.html#/servers/local/tasks")
  await page.getByRole("button", { name: "Queues" }).click()
  await page.getByText("Create definition (JSON)").click()
  await page.getByLabel("Workflow definition FLOW").fill(JSON.stringify(definition))
  await page.getByRole("button", { name: "Validate and publish" }).click()
  await expect(page.getByText("Workflow published", { exact: true })).toBeVisible()
  await page.getByLabel("Workflow name FLOW").fill("browser-flow")
  await page.getByRole("button", { name: "Load versions" }).click()
  await expect(page.getByLabel("Published workflow version FLOW")).toContainText("browser-flow@1")
  await expect(page.getByText("Legacy queue (no workflow)")).toBeVisible()
  await assertNoLoadFailedToast(page)
})

test("Tasks production workspace persists PATCH saves and exercises the full tree workflow", async ({
  page,
  request,
}) => {
  await page.goto("/tests/tasks-fixture.html#/servers/local/tasks");
  await expect(page.getByRole("heading", { name: "Tasks", exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Queues" }).click();
  await page.getByRole("button", { name: "New queue" }).click();
  await page.getByLabel("Queue prefix").fill("test");
  await page.getByLabel("New queue name").fill("Tasks E2E");
  await page.getByRole("button", { name: "Create queue" }).click();
  await expect(page.getByRole("heading", { name: "Tasks E2E" })).toBeVisible();

  await page.getByLabel("Queue name TEST").fill("Tasks E2E updated");
  await page.getByLabel("Queue description TEST").fill("Saved through PATCH from a browser origin");
  await page.getByRole("button", { name: "Save TEST" }).click();
  await expect(page.getByText("Queue updated", { exact: true })).toBeVisible();
  await assertNoLoadFailedToast(page);

  await page.getByRole("button", { name: "All tasks" }).click();
  await page.getByRole("button", { name: "New task" }).click();
  await page.getByLabel("Task queue").selectOption("TEST");
  await page.getByLabel("Task title").fill("Root task");
  await page.getByRole("button", { name: "Create task" }).click();
  await expect(page.getByTestId("task-row-TEST-1")).toBeVisible();

  await page.getByRole("button", { name: "Add child to TEST-1" }).click();
  await page.getByLabel("Task title").fill("First child");
  await page.getByRole("button", { name: "Create task" }).click();
  await expect(page.getByTestId("task-row-TEST-2")).toBeVisible();

  await page.getByRole("button", { name: "New task" }).click();
  await page.getByLabel("Task queue").selectOption("TEST");
  await page.getByLabel("Task title").fill("Second root");
  await page.getByRole("button", { name: "Create task" }).click();
  await expect(page.getByTestId("task-row-TEST-3")).toBeVisible();

  await page.getByTestId("task-row-TEST-1").locator(".task-row-main").click();
  const detail = page.locator(".task-detail-panel");
  await expect(detail.getByRole("heading", { name: "TEST-1" })).toBeVisible();
  await detail.getByLabel("Title").fill("Root task updated");
  await detail.getByLabel("Description").fill("Edited from the production Tasks form");
  await detail.getByLabel("Status").selectOption("in_progress");
  await detail.getByLabel("Assignee").fill("worker-e2e");
  await detail.getByLabel("Manual block reason").fill("waiting on E2E fixture");
  const taskPatchRequest = page.waitForRequest((request) =>
    request.method() === "PATCH" && request.url() === `${daemonURL}/api/tasks/TEST-1`);
  await detail.getByRole("button", { name: "Save task" }).click();
  expect((await taskPatchRequest).postDataJSON()).toMatchObject({
    title: "Root task updated",
    description: "Edited from the production Tasks form",
    status: "in_progress",
    assignee: "worker-e2e",
    manual_block_reason: "waiting on E2E fixture",
  });
  await expect(page.getByText("Task updated", { exact: true })).toBeVisible();
  await expect(page.getByTestId("task-row-TEST-1")).toContainText("Root task updated");
  await expect(page.getByTestId("task-row-TEST-1")).toContainText("blocked");
  await expect(page.getByTestId("task-row-TEST-1")).toContainText("worker-e2e");
  await assertNoLoadFailedToast(page);

  const saved = await taskFromAPI(request, "TEST-1");
  expect(saved).toMatchObject({
    title: "Root task updated",
    description: "Edited from the production Tasks form",
    status: "in_progress",
    assignee: "agent:worker-e2e",
    manual_block_reason: "waiting on E2E fixture",
  });

  await detail.getByLabel("Ask", { exact: true }).selectOption({ index: 1 });
  await detail.getByLabel("Comment", { exact: true }).fill("Please confirm the browser workflow");
  await detail.getByRole("button", { name: "Send comment" }).click();
  await expect(detail.getByText(/Waiting for user:/)).toBeVisible();
  await expect(detail.getByText("Please confirm the browser workflow")).toBeVisible();

  await page.reload();
  await expect(page.getByRole("heading", { name: "Tasks", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Notifications" }).click();
  await expect(page.getByText("Inbox zero.")).toBeVisible();

  await page.getByRole("button", { name: "Waiting for me" }).click();
  await expect(page.getByTestId("task-row-TEST-1")).toBeVisible();
  await page.getByTestId("task-row-TEST-1").locator(".task-row-main").click();
  await detail.getByLabel("Comment", { exact: true }).fill("Confirmed from the customer");
  await detail.getByRole("button", { name: "Send comment" }).click();
  await expect(detail.getByText(/Waiting for user:/)).toHaveCount(0);
  await expect(detail.getByText("Confirmed from the customer")).toBeVisible();

  await page.getByRole("button", { name: "All tasks" }).click();
  await page.getByTestId("task-row-TEST-1").locator(".task-row-main").click();
  await detail.getByLabel("Relation type").selectOption("related");
  await detail.getByLabel("Related task key").fill("TEST-3");
  await detail.getByRole("button", { name: "Add relation" }).click();
  await expect(detail.getByText("related TEST-3")).toBeVisible();
  await detail.getByRole("button", { name: "Remove relation to TEST-3" }).click();
  await expect(detail.getByText("related TEST-3")).toHaveCount(0);

  const criticalRoot = await createTask(request, { title: "Critical root", priority: "P0" });
  const secondCriticalRoot = await createTask(request, { title: "Second critical root", priority: "P0" });
  const lowRoot = await createTask(request, { title: "Low root", priority: "P3" });
  const criticalChild = await createTask(request, {
    title: "Critical child",
    priority: "P0",
    parent_key: "TEST-1",
  });
  const lowChild = await createTask(request, {
    title: "Low child",
    priority: "P3",
    parent_key: "TEST-1",
  });
  const expandRoot = page.getByRole("button", { name: "Expand TEST-1" });
  if (await expandRoot.isVisible()) await expandRoot.click();
  await expect(page.getByTestId(`task-row-${lowChild.key}`)).toBeVisible({ timeout: 10_000 });

  let ordered = await visibleTaskKeys(page);
  expect(ordered.indexOf(criticalRoot.key)).toBeLessThan(ordered.indexOf("TEST-1"));
  expect(ordered.indexOf("TEST-1")).toBeLessThan(ordered.indexOf(lowRoot.key));
  expect(ordered.indexOf(criticalChild.key)).toBeLessThan(ordered.indexOf("TEST-2"));
  expect(ordered.indexOf("TEST-2")).toBeLessThan(ordered.indexOf(lowChild.key));
  await expect(page.getByTestId(`task-row-${criticalRoot.key}`).getByLabel("P0 Critical")).toHaveText("P0");
  await expect(page.getByTestId(`task-row-${lowRoot.key}`).getByLabel("P3 Low")).toHaveText("P3");

  await pointerDrag(page, page.getByRole("button", { name: `Move ${secondCriticalRoot.key}` }), page.getByTestId(`drop-before-${criticalRoot.key}`));
  await expect.poll(async () => {
    const second = await taskFromAPI(request, secondCriticalRoot.key);
    const first = await taskFromAPI(request, criticalRoot.key);
    return second.position < first.position;
  }).toBe(true);

  const lowPosition = (await taskFromAPI(request, lowRoot.key)).position;
  await pointerDrag(page, page.getByRole("button", { name: `Move ${lowRoot.key}` }), page.getByTestId(`drop-before-${criticalRoot.key}`));
  await expect.poll(async () => (await taskFromAPI(request, lowRoot.key)).position).toBe(lowPosition);
  ordered = await visibleTaskKeys(page);
  expect(ordered.indexOf(criticalRoot.key)).toBeLessThan(ordered.indexOf(lowRoot.key));

  await pointerDrag(page, page.getByRole("button", { name: `Move ${lowRoot.key}` }), page.getByTestId("drop-inside-TEST-1"));
  await expect.poll(async () => (await taskFromAPI(request, lowRoot.key)).parent_key).toBe("TEST-1");
  ordered = await visibleTaskKeys(page);
  expect(ordered.indexOf("TEST-2")).toBeLessThan(ordered.indexOf(lowChild.key));
  expect(ordered.indexOf("TEST-2")).toBeLessThan(ordered.indexOf(lowRoot.key));

  await pointerDrag(page, page.getByRole("button", { name: "Move TEST-3" }), page.getByTestId("drop-inside-TEST-1"));
  await expect.poll(async () => (await taskFromAPI(request, "TEST-3")).parent_key).toBe("TEST-1");
  await expect(page.getByTestId("task-row-TEST-3")).toBeVisible();

  await pointerDrag(page, page.getByRole("button", { name: "Move TEST-3" }), page.getByTestId("drop-before-TEST-2"));
  await expect.poll(async () => {
    const moved = await taskFromAPI(request, "TEST-3");
    const first = await taskFromAPI(request, "TEST-2");
    return moved.position < first.position;
  }).toBe(true);

  await page.getByRole("button", { name: "My tasks" }).click();
  await expect(page.getByRole("heading", { name: "My tasks" })).toBeVisible();
  await page.getByRole("button", { name: "All tasks" }).click();
  await page.getByLabel("Search tasks").fill("Root task updated");
  await expect(page.getByTestId("task-row-TEST-1")).toBeVisible();
  await expect(page.getByTestId("task-row-TEST-3")).toHaveCount(0);
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
