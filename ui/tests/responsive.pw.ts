import { expect, test, type APIRequestContext, type Page } from "playwright/test";
import { metricsReport, metricsSlice, taskMetrics } from "../src/test/fixtures/apiFixtures";

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

async function expectFits(page: Page, route: string, ready?: (page: Page) => Promise<void>) {
  await page.goto(`${fixture}${route}`);
  // A page with a live stream (the Runs log tails its events) never goes
  // network-idle; it names the element that shows it has rendered instead.
  if (ready) await ready(page);
  else await page.waitForLoadState("networkidle");
  // RESPONSIVE_SHOTS=dir keeps a picture of each page for a person to look at.
  if (process.env.RESPONSIVE_SHOTS) {
    const dark = await page.evaluate(() => matchMedia("(prefers-color-scheme: dark)").matches);
    const width = page.viewportSize()?.width;
    await page.screenshot({
      path: `${process.env.RESPONSIVE_SHOTS}/${test.info().project.name}-${width}${dark ? "-dark" : ""}${route.replace(/[^a-z0-9]+/gi, "_")}.png`,
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

// The isolated daemon has no completed tasks: Metrics gets a full report.
const METRICS_REPORT = metricsReport({
  agents: [
    metricsSlice({ name: "responsive-agent-with-a-long-name", version: undefined }),
    metricsSlice({ name: "alice", version: undefined }),
  ],
  images: [
    metricsSlice({ name: "developer", version: "0.23.2" }),
    metricsSlice({ name: "developer", version: "0.23.1" }),
  ],
  workflows: [
    metricsSlice({ name: "development", version: "0.10.1" }),
    metricsSlice({ name: "development", version: "0.10.0" }),
  ],
});
const METRICS_ROUTES = [
  "/metrics",
  "/metrics?view=images",
  "/metrics?view=workflows",
  "/metrics/compare?kind=image&name=developer",
];

async function fakeMetrics(page: Page) {
  await page.route(
    (url) => url.pathname === "/api/metrics",
    (route) => route.fulfill({ json: { ok: true, result: METRICS_REPORT } }),
  );
}

for (const route of METRICS_ROUTES) {
  test(`${route} fits the window with a full report`, async ({ page }) => {
    await fakeMetrics(page);
    await expectFits(page, route, async (p) => {
      // Below 640 px the compare table is hidden and a list of blocks shows.
      await expect(p.getByText("Tasks done").filter({ visible: true }).first()).toBeVisible();
    });
  });
}

test("the Metrics toolbar stays one row on a desktop", async ({ page }) => {
  test.skip(page.viewportSize()!.width < 1024, "phones and tablets may wrap");
  await fakeMetrics(page);
  await page.goto(`${fixture}/metrics`);
  await expect(page.getByText("$12.40").first()).toBeVisible();
  const bars = await toolbarRows(page);
  expect(bars.length).toBeGreaterThan(0);
  for (const bar of bars) expect(bar.rows, bar.label).toBe(1);
});

// The page itself never scrolls sideways, whatever its content does.
async function expectTabPageFits(page: Page) {
  const tabPage = page.getByTestId("tab-page");
  const sizes = await tabPage.evaluate((el) => [el.scrollWidth, el.clientWidth]);
  expect(
    sizes[0],
    `tab-page scrollWidth ${sizes[0]} > clientWidth ${sizes[1]}`,
  ).toBeLessThanOrEqual(sizes[1]);
}

// A second native host with a long name, so the toolbar shows Host.
async function addLongHost(page: Page) {
  await page.addInitScript(() => {
    Object.assign(window, {
      __TEST_HOSTS__: [
        {
          id: "build-server",
          label: "build-server-in-the-far-datacenter-with-a-very-long-name",
          kind: "ssh",
          ssh_alias: "build",
          remote_install_dir: "",
          remote_port: 0,
          https_base_url: "",
          last_daemon_version: "",
          state: "disconnected",
          base_url: "",
          local_port: 0,
          phase: "",
          platform: "",
          arch: "",
          prerequisites: [],
          message: "",
        },
      ],
    });
  });
}

const metricsReady = async (p: Page) => {
  await expect(p.getByText("Tasks done").filter({ visible: true }).first()).toBeVisible();
};

// The narrow cases run once, at the widths the metrics review names.
test.describe("metrics on a narrow window", () => {
  test.beforeEach(() => {
    test.skip(test.info().project.name !== "phone-360", "one run sets its own widths");
  });

  for (const width of [360, 768]) {
    test(`/metrics with two hosts and a filter fits at ${width} px`, async ({ page }) => {
      await page.setViewportSize({ width, height: 800 });
      await addLongHost(page);
      await fakeMetrics(page);
      await expectFits(page, "/metrics?agent=alice", async (p) => {
        await metricsReady(p);
        await expect(p.getByRole("combobox", { name: "Host" })).toBeVisible();
        await expect(p.getByRole("button", { name: "Clear filters" })).toBeVisible();
      });
      await expectTabPageFits(page);
    });

    test(`a long card label shows in full at ${width} px`, async ({ page }) => {
      await page.setViewportSize({ width, height: 800 });
      await fakeMetrics(page);
      await page.goto(`${fixture}/metrics`);
      const label = page.getByText("Rejected advances per task", { exact: true }).first();
      await expect(label).toBeVisible();
      const cut = await label.evaluate((el) => el.scrollWidth > el.clientWidth);
      expect(cut, "the label is cut").toBe(false);
      await expect(label).toBeInViewport({ ratio: 1 });
    });
  }

  test("every period button and the Custom dates show in full at 360 px", async ({ page }) => {
    await page.setViewportSize({ width: 360, height: 800 });
    await fakeMetrics(page);
    await page.goto(`${fixture}/metrics?agent=alice`);
    await metricsReady(page);
    const periods = page.getByRole("group", { name: "Period" });
    await periods.getByRole("button", { name: "Custom" }).click();
    for (const name of ["Quarter", "Custom"])
      await expect(periods.getByRole("button", { name })).toBeInViewport({ ratio: 1 });
    for (const name of ["From", "To"])
      await expect(page.getByLabel(name, { exact: true })).toBeInViewport({ ratio: 1 });
    expect(await overflowing(page)).toEqual([]);
    await expectTabPageFits(page);
  });

  test("/metrics/compare with the same version twice fits at 360 px", async ({ page }) => {
    await page.setViewportSize({ width: 360, height: 800 });
    await fakeMetrics(page);
    await expectFits(page, "/metrics/compare?kind=image&name=developer&a=0.23.2&b=0.23.2", (p) =>
      expect(p.getByText("Choose a different version.")).toBeVisible(),
    );
    await expectTabPageFits(page);
  });

  test.describe("dark", () => {
    test.use({ colorScheme: "dark" });
    for (const route of ["/metrics?agent=alice", "/metrics/compare?kind=image&name=developer"]) {
      test(`${route} fits at 360 px in the dark theme`, async ({ page }) => {
        await page.setViewportSize({ width: 360, height: 800 });
        await addLongHost(page);
        await fakeMetrics(page);
        await expectFits(page, route, metricsReady);
        await expectTabPageFits(page);
      });
    }
  });
});

test("task detail fits the window", async ({ page }) => {
  await expectFits(page, `/tasks?task=${taskKey}`);
  await expect(page.getByText(taskKey).first()).toBeVisible();
});

// The task metrics of a long-lived task: long agent names, five statuses and
// several image versions, so the Agents table is wider than a phone.
const usageOf = (iterations: number, cost: number) => ({
  iterations,
  cost_usd: cost,
  tokens: { input: 1_200_000, output: 90_000, cache_write: 40_000, cache_read: 3_000_000 },
});
const imageRun = (version: string) => ({
  image: "developer-with-a-long-image-name",
  image_version: version,
  image_digest: "",
  image_ref: "",
  ...usageOf(3, 0.62),
  tokens: usageOf(0, 0).tokens,
});
const TASK_METRICS = taskMetrics({
  // The responsive task is an open flexible task.
  status: "open",
  status_time_s: {
    open: 7200,
    in_progress: 52_000,
    wait_customer: 9000,
    done: 1800,
    cancelled: 600,
  },
  agents: [
    {
      agent: AGENT,
      usage: usageOf(9, 3.1),
      images: [imageRun("0.23.2"), imageRun("0.23.1")],
    },
    { agent: "reviewer-agent-with-another-long-name", usage: usageOf(3, 0.4), images: [] },
  ],
  images: [imageRun("0.23.2"), imageRun("0.23.1"), imageRun("unknown")],
  workflows: [{ name: "development", version: "0.10.1", digest: "d" }],
});

async function openTaskMetrics(page: Page) {
  await page.addInitScript(() => localStorage.setItem("tasks:metrics-open:v1", "open"));
  await page.route(
    (url) => url.pathname.startsWith("/api/metrics/tasks/"),
    (route) => route.fulfill({ json: { ok: true, result: TASK_METRICS } }),
  );
}

// The section sits below the description and comments, and the sticky task
// header covers the top of the sheet: bring it to the middle so a screenshot
// shows it.
const taskMetricsReady = async (page: Page) => {
  await expect(page.getByRole("table", { name: "Agents" })).toBeVisible();
  await page
    .getByRole("button", { name: /^Metrics/ })
    .evaluate((element) => element.scrollIntoView({ block: "center" }));
};

// Screens 3 and 4 of the metrics design, in both themes. The default theme is
// `system`, so it follows the color scheme of the page.
for (const colorScheme of ["light", "dark"] as const) {
  test.describe(`metrics of an agent and a task, ${colorScheme}`, () => {
    test.use({ colorScheme });

    test("the agent Metrics view fits the window", async ({ page }) => {
      await fakeMetrics(page);
      await expectFits(page, `/agents/local/${AGENT}/activity?view=metrics`, async (p) => {
        // The tab has a Tasks done card of its own: wait for the view's.
        const view = p.getByRole("region", { name: "Agent metrics" });
        await expect(view.getByRole("table", { name: "development" })).toBeVisible();
        await p
          .getByRole("radiogroup", { name: "More" })
          .evaluate((element) => element.scrollIntoView({ block: "start" }));
      });
    });

    test("the open task Metrics section fits the window", async ({ page }) => {
      await openTaskMetrics(page);
      await expectFits(page, `/tasks?task=${taskKey}`, taskMetricsReady);
    });
  });
}

test("the task Agents table scrolls in its own container with a pinned Agent column", async ({
  page,
}) => {
  test.skip(page.viewportSize()!.width !== 360, "one phone width is enough");
  await openTaskMetrics(page);
  await page.goto(`${fixture}/tasks?task=${taskKey}`);
  await taskMetricsReady(page);
  const table = page.getByRole("table", { name: "Agents" });
  const result = await table.evaluate((element) => {
    const scroller = element.parentElement!;
    const before = { scroll: scroller.scrollWidth, client: scroller.clientWidth };
    scroller.scrollLeft = 120;
    const header = element.querySelector("tbody th")!.getBoundingClientRect();
    return {
      ...before,
      scrolled: scroller.scrollLeft,
      offset: Math.round(header.left - scroller.getBoundingClientRect().left),
      page: document.documentElement.scrollWidth <= window.innerWidth,
    };
  });
  expect(result.scroll).toBeGreaterThan(result.client);
  expect(result.scrolled).toBeGreaterThan(0);
  expect(Math.abs(result.offset)).toBeLessThanOrEqual(1);
  expect(result.page).toBe(true);
});

test("the task Metrics section works on a 320 px sheet", async ({ page }) => {
  test.skip(page.viewportSize()!.width !== 360, "one run at its own size");
  await page.setViewportSize({ width: 320, height: 700 });
  await openTaskMetrics(page);
  await expectFits(page, `/tasks?task=${taskKey}`, taskMetricsReady);
});

async function createTask(
  request: APIRequestContext,
  queue: string,
  name: string,
  parentKey = "",
): Promise<string> {
  const response = await request.post(`${daemonURL}/api/tasks`, {
    data: {
      queue,
      idempotency_key: `responsive-deep-${name}`,
      title: `Deep tree ${name}`,
      parent_key: parentKey,
    },
  });
  const envelope = await response.json();
  expect(envelope.ok, JSON.stringify(envelope)).toBe(true);
  return envelope.result.key as string;
}

// A MOB chain of depths 0–5 and, at depth 6, a child from another queue: its
// row carries a queue marker.
async function ensureDeepTree(request: APIRequestContext) {
  await request.post(`${daemonURL}/api/task-queues`, {
    data: { prefix: "MOBDEEP", name: "Deep tree" },
  });
  const chain = [await createTask(request, "MOB", "0")];
  for (let depth = 1; depth <= 5; depth += 1)
    chain.push(await createTask(request, "MOB", String(depth), chain.at(-1)));
  return { chain, deep: await createTask(request, "MOBDEEP", "6", chain.at(-1)) };
}

test("a deep cross-queue key fits its column", async ({ page, request }) => {
  const { chain, deep } = await ensureDeepTree(request);
  await page.goto(`${fixture}/tasks`);
  for (const key of chain) {
    await expect(page.getByTestId(`task-row-${key}`)).toBeVisible();
    const expand = page.getByRole("button", { name: `Expand ${key}` });
    if (await expand.isVisible()) await expand.click();
  }
  const row = page.getByTestId(`task-row-${deep}`);
  await expect(row).toBeVisible();

  const box = await row.locator("[data-column=key]").evaluate((column) => {
    const rect = (element: Element | Range) => element.getBoundingClientRect();
    const marker = column.querySelector("[role=img]")!;
    const key = column.querySelector(".font-mono")!;
    const firstFour = document.createRange();
    firstFour.setStart(key.firstChild!, 0);
    firstFour.setEnd(key.firstChild!, 4);
    return {
      column: rect(column).toJSON() as DOMRect,
      marker: rect(marker).toJSON() as DOMRect,
      key: rect(key).toJSON() as DOMRect,
      firstFour: rect(firstFour).toJSON() as DOMRect,
    };
  });
  console.log(
    `${test.info().project.name}: Key column ${box.column.left}–${box.column.right}, ` +
      `marker ${box.marker.left}–${box.marker.right}, key at +${box.key.left - box.column.left}px`,
  );
  if (process.env.RESPONSIVE_SHOTS) {
    await row.screenshot({
      path: `${process.env.RESPONSIVE_SHOTS}/${test.info().project.name}_deep_row.png`,
    });
  }

  if (page.viewportSize()!.width < 640) {
    expect(box.column.width).toBe(96);
    expect(box.marker.left).toBeGreaterThanOrEqual(box.column.left);
    expect(box.marker.right).toBeLessThanOrEqual(box.column.right);
    // Four characters and the ellipsis after them (one more character wide).
    const charWidth = box.firstFour.width / 4;
    expect(box.firstFour.left).toBeGreaterThanOrEqual(box.column.left);
    expect(box.firstFour.right + charWidth).toBeLessThanOrEqual(box.key.right + 0.5);
    expect(box.key.right).toBeLessThanOrEqual(box.column.right);
  } else {
    // As before: six levels of 20px, the chevron, the marker and their gaps.
    expect(box.key.left - box.column.left).toBe(20 * 6 + 16 + 6 + 12 + 6);
  }
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
    await expectFits(
      page,
      `/agents/local/${AGENT}/${tab}`,
      tab === "activity"
        ? async (p) => {
            await expect(p.getByRole("radiogroup", { name: "More" })).toBeVisible();
            await expect(p.getByRole("radio", { name: "Runs log" })).toBeChecked();
          }
        : undefined,
    );
    await expect(page.getByRole("heading", { name: AGENT })).toBeVisible();
  });
}

// The design's check: on a 1440×900 window the four groups of Settings are all
// visible without scrolling, with the first-visit hint shown.
test("agent Settings shows four groups above the fold at 1440×900", async ({ page }) => {
  test.skip(test.info().project.name !== "desktop-1024", "one fixed window size is enough");
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto(`${fixture}/agents/local/${AGENT}/settings`);
  const spending = page.getByRole("region", { name: "Spending limit" });
  await expect(spending).toBeVisible();
  for (const name of ["Model", "Working folder", "Autopilot"])
    await expect(page.getByRole("region", { name })).toBeVisible();
  const box = (await spending.boundingBox())!;
  expect(box.y + box.height).toBeLessThanOrEqual(900);
  await expect(page.getByRole("button", { name: /^Advanced/ })).toBeVisible();
});

test("agent Settings with Advanced open fits the window", async ({ page }) => {
  await expectFits(page, `/agents/local/${AGENT}/settings`, async (p) => {
    await p.getByRole("button", { name: /^Advanced/ }).click();
    await expect(p.getByRole("region", { name: "Retention and cleanup" })).toBeVisible();
  });
});

// The Work tab is a composer and task sections; ensureTask gives it a Queued row.
test("agent Work tab composer, details and sections fit the window", async ({ page }) => {
  await expectFits(page, `/agents/local/${AGENT}/work`, async (p) => {
    await expect(p.getByRole("region", { name: /^Queued \([1-9]\d*\)$/ })).toBeVisible();
  });
  const composer = page.getByRole("textbox", { name: `New task for ${AGENT}` });
  await expect(composer).toBeVisible();
  await expect(page.getByRole("region", { name: /^In progress \(\d+\)$/ })).toBeVisible();
  await expect(page.getByRole("searchbox", { name: "Search done tasks" })).toBeVisible();
  await expect(
    page
      .getByRole("region", { name: /^Queued/ })
      .getByRole("button", { name: new RegExp(taskKey) }),
  ).toBeVisible();

  await page.getByRole("button", { name: "Details", exact: true }).click();
  await expect(page.getByRole("combobox", { name: "Queue", exact: true })).toBeVisible();
  await expect(page.getByRole("combobox", { name: "Priority", exact: true })).toBeVisible();
  expect(await overflowing(page), "overflow with the composer details open").toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
});

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
