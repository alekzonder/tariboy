import { expect, test, type Locator, type Page } from "playwright/test";

// Real-browser geometry coverage; intentionally named *.pw.ts so Vitest does
// not collect it as a unit test. Every drag uses the same pointer events as a
// person; synthetic DragEvent/DataTransfer dispatch would hide WebView bugs.
async function pointerDrag(
  page: Page,
  source: Locator,
  clientX: number,
  clientY: number,
  expectedDock?: "left" | "right" | "top" | "bottom",
) {
  const sourceBox = await source.boundingBox();
  expect(sourceBox).not.toBeNull();
  const startX = sourceBox!.x + sourceBox!.width / 2;
  const startY = sourceBox!.y + sourceBox!.height / 2;
  await page.mouse.move(startX, startY);
  await page.mouse.down();
  await page.mouse.move(startX + 8, startY, { steps: 2 });
  await page.mouse.move(clientX, clientY, { steps: 20 });
  await expect(page.getByTestId("workspace-drop-preview")).toBeVisible();
  if (expectedDock) {
    await expect(page.getByTestId("workspace-drop-preview")).toHaveAttribute(
      "data-dock",
      expectedDock,
    );
  }
  await page.mouse.up();
  await expect(page.getByTestId("workspace-drop-preview")).toHaveCount(0);
}

test("real pointer drags create nested splits, move panes, and preserve resize", async ({ page }) => {
  await page.goto("/tests/workspace-fixture.html");
  const tabsets = page.locator(".flexlayout__tabset");
  const workspace = page.getByTestId("terminal-workspace");
  const workspaceBox = await workspace.boundingBox();
  expect(workspaceBox).not.toBeNull();

  await pointerDrag(
    page,
    page.getByRole("button", { name: "Open alpha" }),
    workspaceBox!.x + workspaceBox!.width / 2,
    workspaceBox!.y + workspaceBox!.height / 2,
  );
  await expect(page.getByRole("tab", { name: "alpha" })).toBeVisible();
  await expect(tabsets).toHaveCount(1);

  const alphaPane = tabsets.filter({ has: page.getByRole("tab", { name: "alpha" }) });
  const alphaBox = await alphaPane.boundingBox();
  expect(alphaBox).not.toBeNull();
  await pointerDrag(
    page,
    page.getByRole("button", { name: "Open beta" }),
    alphaBox!.x + alphaBox!.width - 3,
    alphaBox!.y + alphaBox!.height / 2,
    "right",
  );

  await expect(page.getByRole("tab", { name: "beta" })).toBeVisible();
  await expect(tabsets).toHaveCount(2);
  const beforeLeft = await tabsets.nth(0).boundingBox();
  const beforeRight = await tabsets.nth(1).boundingBox();
  expect(beforeLeft).not.toBeNull();
  expect(beforeRight).not.toBeNull();
  expect(beforeLeft!.width).toBeGreaterThan(200);
  expect(beforeRight!.width).toBeGreaterThan(200);
  expect(beforeLeft!.x).toBeLessThan(beforeRight!.x);

  const nestedAlphaBox = await alphaPane.boundingBox();
  expect(nestedAlphaBox).not.toBeNull();
  await pointerDrag(
    page,
    page.getByRole("button", { name: "Open gamma" }),
    nestedAlphaBox!.x + nestedAlphaBox!.width / 2,
    nestedAlphaBox!.y + nestedAlphaBox!.height - 3,
    "bottom",
  );
  await expect(page.getByRole("tab", { name: "gamma" })).toBeVisible();
  await expect(tabsets).toHaveCount(3);
  const gammaPane = tabsets.filter({ has: page.getByRole("tab", { name: "gamma" }) });
  const gammaBox = await gammaPane.boundingBox();
  const splitAlphaBox = await alphaPane.boundingBox();
  expect(gammaBox).not.toBeNull();
  expect(splitAlphaBox).not.toBeNull();
  expect(gammaBox!.y).toBeGreaterThan(splitAlphaBox!.y);

  const betaPane = tabsets.filter({ has: page.getByRole("tab", { name: "beta" }) });
  const resizeBeforeAlpha = await alphaPane.boundingBox();
  const resizeBeforeBeta = await betaPane.boundingBox();
  expect(resizeBeforeAlpha).not.toBeNull();
  expect(resizeBeforeBeta).not.toBeNull();
  const splitterBox = await page.locator(".flexlayout__splitter_horz").evaluateAll(
    (elements) => elements
      .map((element) => element.getBoundingClientRect().toJSON())
      .find((rect) =>
        rect.width > 0
        && rect.height > 0
        && rect.top >= 0
        && rect.left >= 0
        && rect.bottom <= window.innerHeight
        && rect.right <= window.innerWidth),
  );
  expect(splitterBox).toBeDefined();
  await page.mouse.move(
    splitterBox!.x + splitterBox!.width / 2,
    splitterBox!.y + splitterBox!.height / 2,
  );
  await page.mouse.down();
  await page.mouse.move(
    splitterBox!.x + 80,
    splitterBox!.y + splitterBox!.height / 2,
    { steps: 10 },
  );
  await page.mouse.up();

  await expect.poll(async () => {
    const after = await alphaPane.boundingBox();
    return Math.abs(after!.width - resizeBeforeAlpha!.width);
  }).toBeGreaterThan(30);
  await expect.poll(async () => {
    const after = await betaPane.boundingBox();
    return Math.abs(after!.width - resizeBeforeBeta!.width);
  }).toBeGreaterThan(30);

  const movedAlphaTarget = await alphaPane.boundingBox();
  expect(movedAlphaTarget).not.toBeNull();
  await pointerDrag(
    page,
    page.getByRole("tab", { name: "beta" }).getByTestId("workspace-drag-beta"),
    movedAlphaTarget!.x + movedAlphaTarget!.width / 2,
    movedAlphaTarget!.y + 3,
    "top",
  );

  const movedAlpha = await alphaPane.boundingBox();
  const movedBeta = await betaPane.boundingBox();
  expect(movedAlpha).not.toBeNull();
  expect(movedBeta).not.toBeNull();
  // Assert the vertical stack itself, not merely that something shifted: a
  // displacement assertion is also satisfied by the resize above, which is how a
  // silent no-op on this step stayed green for so long. beta was dropped on
  // alpha's top edge, so it must end up entirely above alpha and in the same
  // column (their horizontal extents overlap).
  expect(movedBeta!.y + movedBeta!.height).toBeLessThanOrEqual(movedAlpha!.y + 1);
  const columnOverlap = Math.min(movedBeta!.x + movedBeta!.width, movedAlpha!.x + movedAlpha!.width)
    - Math.max(movedBeta!.x, movedAlpha!.x);
  expect(columnOverlap).toBeGreaterThan(Math.min(movedBeta!.width, movedAlpha!.width) / 2);
  await expect(tabsets).toHaveCount(3);
  for (const pane of [alphaPane, betaPane, gammaPane]) {
    await expect(pane.getByRole("tab")).toHaveCount(1);
  }
});
