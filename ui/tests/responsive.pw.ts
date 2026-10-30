import { expect, test, type APIRequestContext, type Page } from "playwright/test";

const daemonURL = "http://127.0.0.1:4176";
const fixture = "/tests/tasks-fixture.html#";
const AGENT = "responsive-agent-with-a-long-name";
// The width below which the agent list is a drawer behind the menu button.
const COMPACT = 768;
const SETTINGS_PAGES = [
  "/settings/hosts",
  "/settings/teams",
  "/settings/usage",
  "/settings/agent-defaults",
  "/settings/integrations",
  "/settings/maintenance",
];

async function ensureAgent(request: APIRequestContext) {
  const existing = await request.get(`${daemonURL}/api/agents/${AGENT}`);
  if (existing.ok()) return;
  const response = await request.post(`${daemonURL}/api/agents`, {
    data: { name: AGENT, image: "bare:latest", cwd: "/tmp" },
  });
  const envelope = await response.json();
  expect(envelope.ok, JSON.stringify(envelope)).toBe(true);
}

// Every visible element that ends past the right edge of the window and is not
// inside a horizontal scroller of its own: content cut off or pushing the page.
async function overflowing(page: Page) {
  return page.evaluate(() => {
    const limit = window.innerWidth + 1;
    const scrollsX = (element: Element) => {
      for (let node = element.parentElement; node; node = node.parentElement) {
        const overflowX = getComputedStyle(node).overflowX;
        if (overflowX === "auto" || overflowX === "scroll") return true;
      }
      return false;
    };
    const found: string[] = [];
    for (const element of document.body.querySelectorAll("*")) {
      const rect = element.getBoundingClientRect();
      if (rect.width <= 1 || rect.height <= 1 || rect.right <= limit) continue;
      const style = getComputedStyle(element);
      if (style.visibility === "hidden" || style.display === "none") continue;
      if (scrollsX(element)) continue;
      const label =
        `${element.tagName.toLowerCase()}${element.id ? `#${element.id}` : ""}` +
        `[${(element.getAttribute("data-testid") ?? element.className?.toString() ?? "").slice(0, 60)}]` +
        ` right=${Math.round(rect.right)} "${(element.getAttribute("aria-label") ?? element.textContent ?? "").trim().slice(0, 40)}"`;
      found.push(label);
    }
    return found.slice(0, 10);
  });
}

async function expectFits(page: Page, route: string) {
  await page.goto(`${fixture}${route}`);
  await page.waitForLoadState("networkidle");
  // RESPONSIVE_SHOTS=dir keeps a picture of each page for a person to look at.
  if (process.env.RESPONSIVE_SHOTS) {
    await page.screenshot({
      path: `${process.env.RESPONSIVE_SHOTS}/${test.info().project.name}${route.replace(/[^a-z0-9]+/gi, "_")}.png`,
    });
  }
  expect(await overflowing(page), `overflow at ${route}`).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
}

// A task with a long title, so the task table and the task detail have rows.
async function ensureTask(request: APIRequestContext) {
  // An existing queue answers with an error; the task create below is the check.
  await request.post(`${daemonURL}/api/task-queues`, {
    data: { prefix: "MOB", name: "Mobile layout" },
  });
  const response = await request.post(`${daemonURL}/api/tasks`, {
    data: {
      queue: "MOB",
      idempotency_key: "responsive-task",
      priority: "P1",
      title:
        "Check that the whole interface works on phones and tablets without sideways scrolling",
      description: "A [link](https://example.com) and a long line: " + "word ".repeat(80),
      assignee: `agent:${AGENT}`,
    },
  });
  const envelope = await response.json();
  expect(envelope.ok, JSON.stringify(envelope)).toBe(true);
  return envelope.result.key as string;
}

let taskKey = "";
test.beforeEach(async ({ request }) => {
  await ensureAgent(request);
  taskKey = await ensureTask(request);
});

test("task detail fits the window", async ({ page }) => {
  await expectFits(page, `/tasks?task=${taskKey}`);
  await expect(page.getByText(taskKey).first()).toBeVisible();
});

// The tops of the direct children of every visible [data-toolbar]: a toolbar
// that wrapped has children on two rows.
async function toolbarRows(page: Page) {
  return page.evaluate(() =>
    [...document.querySelectorAll("[data-toolbar]")]
      .filter((bar) => bar.getBoundingClientRect().height > 0)
      .map((bar) => {
        const tops = [...bar.children]
          .map((child) => child.getBoundingClientRect())
          .filter((rect) => rect.width > 0 && rect.height > 0)
          .map((rect) => Math.round(rect.top + rect.height / 2));
        return { label: bar.getAttribute("aria-label") ?? "", rows: new Set(tops).size };
      }),
  );
}

// At 1024 px and wider, toolbars collapse controls into a menu before they wrap.
for (const route of [
  "/tasks",
  "/inbox",
  "/library/images",
  "/library/workflows",
  "/library/sources",
]) {
  test(`${route} toolbar stays one row on a desktop`, async ({ page }) => {
    test.skip(page.viewportSize()!.width < 1024, "phones and tablets may wrap");
    await page.goto(`${fixture}${route}`);
    await page.waitForLoadState("networkidle");
    const bars = await toolbarRows(page);
    expect(bars.length).toBeGreaterThan(0);
    for (const bar of bars) expect(bar.rows, `${route} ${bar.label}`).toBe(1);
  });
}

test("Home sections do not overlap on a desktop", async ({ page }) => {
  test.skip(page.viewportSize()!.width < 1024, "phones and tablets stack the sections");
  await page.goto(`${fixture}/`);
  await expect(page.getByRole("heading", { name: "Home" })).toBeVisible();
  await page.waitForLoadState("networkidle");
  const boxes = await page
    .locator("main section")
    .evaluateAll((sections) =>
      sections.map((section) => section.getBoundingClientRect().toJSON() as DOMRect),
    );
  for (const [i, a] of boxes.entries()) {
    for (const b of boxes.slice(i + 1)) {
      const overlap =
        a.left < b.right - 1 &&
        b.left < a.right - 1 &&
        a.top < b.bottom - 1 &&
        b.top < a.bottom - 1;
      const nested =
        (a.left <= b.left && a.right >= b.right && a.top <= b.top && a.bottom >= b.bottom) ||
        (b.left <= a.left && b.right >= a.right && b.top <= a.top && b.bottom >= a.bottom);
      expect(overlap && !nested, "Home sections overlap").toBe(false);
    }
  }
  // Each Today tile holds its label and number without spilling into the next.
  const spilled = await page
    .locator("[data-total]")
    .evaluateAll((tiles) =>
      tiles
        .filter((tile) => tile.scrollWidth > tile.clientWidth + 1)
        .map((tile) => tile.getAttribute("data-total")),
    );
  expect(spilled).toEqual([]);
});

for (const route of [
  "/",
  "/?new=1",
  "/tasks",
  "/inbox",
  "/activity",
  "/library/images",
  "/library/images/bare/latest",
  "/library/workflows",
  "/library/sources",
  ...SETTINGS_PAGES,
  "/settings/maintenance/operations/1",
  "/app-settings",
]) {
  test(`${route} fits the window`, async ({ page }) => {
    await expectFits(page, route);
  });
}

for (const tab of ["work", "console", "activity", "settings", "mailbox", "advanced"]) {
  test(`agent ${tab} tab fits the window`, async ({ page }) => {
    await expectFits(page, `/agents/local/${AGENT}/${tab}`);
    await expect(page.getByRole("heading", { name: AGENT })).toBeVisible();
  });
}

test("agent header controls never cover the agent's details", async ({ page }) => {
  await page.goto(`${fixture}/agents/local/${AGENT}/work`);
  const heading = page.getByRole("heading", { name: AGENT });
  await expect(heading).toBeVisible();
  const controls = page.getByTestId("agent-controls");
  const controlsBox = await controls.boundingBox();
  expect(controlsBox).not.toBeNull();
  for (const detail of [
    heading,
    page.getByTestId("agent-sentence"),
    page.getByTestId("agent-facts"),
  ]) {
    const box = await detail.boundingBox();
    expect(box).not.toBeNull();
    const overlaps =
      box!.x < controlsBox!.x + controlsBox!.width &&
      controlsBox!.x < box!.x + box!.width &&
      box!.y < controlsBox!.y + controlsBox!.height &&
      controlsBox!.y < box!.y + box!.height;
    expect(overlaps, `controls overlap ${await detail.textContent()}`).toBe(false);
  }
});

test("the agent list is a drawer on compact screens and a column on wide ones", async ({
  page,
}) => {
  const width = page.viewportSize()!.width;
  await page.goto(`${fixture}/agents/local/${AGENT}/work`);
  await expect(page.getByRole("heading", { name: AGENT })).toBeVisible();
  if (width >= COMPACT) {
    await expect(page.getByTestId("agents-sidebar")).toBeVisible();
    await expect(page.getByRole("button", { name: "Open menu" })).toHaveCount(0);
    return;
  }
  await expect(page.getByTestId("agents-sidebar")).toHaveCount(0);
  await page.getByRole("button", { name: "Open menu" }).click();
  const drawer = page.getByRole("dialog", { name: "Navigation" });
  await expect(drawer).toBeVisible();
  // No focus in the search field, so a phone keeps its keyboard down.
  await expect(drawer.getByRole("searchbox", { name: "Search agents" })).not.toBeFocused();
  // It slides in; measure where it settles.
  await expect.poll(async () => (await drawer.boundingBox())?.x).toBe(0);
  const box = await drawer.boundingBox();
  expect(box!.x + box!.width).toBeLessThanOrEqual(width);
  await drawer.getByText(AGENT, { exact: true }).click();
  await expect(drawer).toHaveCount(0);
  await expect(page).toHaveURL(new RegExp(`/agents/local/${AGENT}/work`));
});

test("an old Library URL opens its Library page", async ({ page }) => {
  await page.goto(`${fixture}/servers/local/images`);
  await expect(page.getByRole("navigation", { name: "Library" })).toBeVisible();
  expect(new URL(page.url()).hash).toBe("#/library/images?host=local");
});

test("an old Settings URL opens its Settings page", async ({ page }) => {
  await page.goto(`${fixture}/servers/local/settings/advanced/groups`);
  await expect(page.getByRole("heading", { name: "Teams" })).toBeVisible();
  expect(new URL(page.url()).hash).toBe("#/settings/teams?host=local");
});

test("the Settings nav fits without scrolling", async ({ page }) => {
  const width = page.viewportSize()!.width;
  test.skip(width < 1024, "below lg the Settings nav is a sideways strip");
  for (const route of SETTINGS_PAGES) {
    await page.goto(`${fixture}${route}`);
    const nav = page.getByRole("navigation", { name: "Settings" });
    await expect(nav.getByRole("link", { name: "Application" })).toBeVisible();
    const aside = nav.locator("xpath=..");
    expect(
      await aside.evaluate((element) => element.scrollHeight <= element.clientHeight),
      `${route} scrolls its Settings nav`,
    ).toBe(true);
  }
});
