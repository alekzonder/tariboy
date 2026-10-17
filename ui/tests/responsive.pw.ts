import { expect, test, type APIRequestContext, type Page } from "playwright/test";

const daemonURL = "http://127.0.0.1:4176";
const fixture = "/tests/tasks-fixture.html#";
const AGENT = "responsive-agent-with-a-long-name";
// The width below which the agent list is a drawer behind the menu button.
const COMPACT = 1024;

async function ensureAgent(request: APIRequestContext) {
  const existing = await request.get(`${daemonURL}/api/agents/${AGENT}`);
  if (existing.ok()) return;
  const response = await request.post(`${daemonURL}/api/agents`, {
    data: { name: AGENT, image: "bare:latest", cwd: "/tmp", loop: false },
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
      const label = `${element.tagName.toLowerCase()}${element.id ? `#${element.id}` : ""}`
        + `[${(element.getAttribute("data-testid") ?? element.className?.toString() ?? "").slice(0, 60)}]`
        + ` right=${Math.round(rect.right)} "${(element.textContent ?? "").trim().slice(0, 40)}"`;
      found.push(label);
    }
    return found.slice(0, 10);
  });
}

async function expectFits(page: Page, route: string) {
  await page.goto(`${fixture}${route}`);
  await page.waitForLoadState("networkidle");
  expect(await overflowing(page), `overflow at ${route}`).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
}

test.beforeEach(async ({ request }) => {
  await ensureAgent(request);
});

for (const route of [
  "/",
  "/?view=all",
  "/servers/local/tasks",
  "/servers/local/images",
  "/servers/local/workflows",
  "/servers/local/stores",
  "/servers/local/settings",
  "/servers/local/settings/hosts",
  "/servers/local/settings/advanced",
  "/app-settings",
]) {
  test(`${route} fits the window`, async ({ page }) => {
    await expectFits(page, route);
  });
}

for (const tab of ["chat", "tasks", "autopilot", "activity", "configuration", "advanced"]) {
  test(`agent ${tab} tab fits the window`, async ({ page }) => {
    await expectFits(page, `/agents/local/${AGENT}/${tab}`);
    await expect(page.getByRole("heading", { name: AGENT })).toBeVisible();
  });
}

test("agent header controls never cover the agent's details", async ({ page }) => {
  await page.goto(`${fixture}/agents/local/${AGENT}/chat`);
  const heading = page.getByRole("heading", { name: AGENT });
  await expect(heading).toBeVisible();
  const controls = page.getByTestId("agent-controls");
  const controlsBox = await controls.boundingBox();
  expect(controlsBox).not.toBeNull();
  for (const detail of [heading, page.getByTestId("agent-image"), page.getByTestId("agent-cwd")]) {
    const box = await detail.boundingBox();
    expect(box).not.toBeNull();
    const overlaps = box!.x < controlsBox!.x + controlsBox!.width && controlsBox!.x < box!.x + box!.width
      && box!.y < controlsBox!.y + controlsBox!.height && controlsBox!.y < box!.y + box!.height;
    expect(overlaps, `controls overlap ${await detail.textContent()}`).toBe(false);
  }
});

test("the agent list is a drawer on compact screens and a column on wide ones", async ({ page }) => {
  const width = page.viewportSize()!.width;
  await page.goto(`${fixture}/agents/local/${AGENT}/chat`);
  await expect(page.getByRole("heading", { name: AGENT })).toBeVisible();
  if (width >= COMPACT) {
    await expect(page.getByTestId("agents-sidebar")).toBeVisible();
    await expect(page.getByRole("button", { name: "Open menu" })).toHaveCount(0);
    return;
  }
  await expect(page.getByTestId("agents-sidebar")).toHaveCount(0);
  await page.getByRole("button", { name: "Open menu" }).click();
  const drawer = page.getByRole("dialog", { name: "Agents" });
  await expect(drawer).toBeVisible();
  const box = await drawer.boundingBox();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(width);
  await drawer.getByRole("tab", { name: "Agents" }).click();
  await drawer.getByText(AGENT, { exact: true }).click();
  await expect(drawer).toHaveCount(0);
  await expect(page).toHaveURL(new RegExp(`/agents/local/${AGENT}/chat`));
});
