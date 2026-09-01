import { expect, test } from "playwright/test";

test("portable image export, source states, and import retag work in a real browser", async ({ page }) => {
  await page.goto("/tests/images-fixture.html");
  await expect(page.getByText("/srv/images/reviewer")).toBeVisible();
  await expect(page.getByText("Source CWD unavailable — imported artifact").first()).toBeVisible();
  await expect(page.getByText("Source CWD unavailable — /srv/images/missing")).toBeVisible();

  const downloadPromise = page.waitForEvent("download");
  await page.getByRole("button", { name: "Export reviewer:v3" }).click();
  expect((await downloadPromise).suggestedFilename()).toBe("reviewer-v3.tariboy-image.tar.gz");
  await expect(page.getByText("image reviewer:v3 saved to file reviewer-v3.tariboy-image.tar.gz"))
    .toBeVisible();

  await page.getByLabel("Import image archive").setInputFiles({
    name: "reviewer-v3.tariboy-image.tar.gz",
    mimeType: "application/gzip",
    buffer: Buffer.from("portable-image"),
  });
  await expect(page.getByLabel("Import name")).toHaveValue("reviewer");
  await expect(page.getByLabel("Import tag")).toHaveValue("v3");
  await page.getByLabel("Import name").fill("reviewer-copy");
  await page.getByLabel("Import tag").fill("v4");
  await page.getByRole("button", { name: "Import image", exact: true }).click();
  await expect.poll(() => page.evaluate(() => (window as Window & { __imageApplyRef?: string }).__imageApplyRef)).toBe("reviewer-copy:v4");

  await page.goto("/tests/images-fixture.html?mode=build");
  await page.setViewportSize({ width: 1200, height: 300 });
  await page.getByLabel("Image source directory").fill("/srv/images/browser-built");
  await page.getByLabel("Image name").fill("browser-built");
  await expect(page.getByLabel("Image tag")).toHaveValue("latest");
  await page.getByRole("button", { name: "Validate" }).click();
  const template = page.getByLabel("Validated image template");
  await expect(template).toContainText("identity");
  await expect(template).toContainText("$CURRENT_VERSION_STORE/prompts/iteration-finish.md");
  await expect(template).toContainText("context");
  const imagesWorkspace = page.getByRole("heading", { name: "Images" }).locator("../..");
  await imagesWorkspace.hover();
  await page.mouse.wheel(0, 600);
  await expect.poll(() => imagesWorkspace.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  await page.getByRole("button", { name: "Close" }).click();
  await expect(template).toHaveCount(0);
  await page.getByRole("button", { name: "Build", exact: true }).click();
  await expect.poll(() => page.evaluate(() => (window as Window & { __imageBuild?: unknown }).__imageBuild)).toEqual({
    path: "/srv/images/browser-built", name: "browser-built",
  });

  await page.addInitScript(() => {
    const state = window as Window & { __imageVSCode?: unknown; __TAURI_INTERNALS__?: { invoke: (cmd: string, args: unknown) => Promise<null> } };
    state.__TAURI_INTERNALS__ = { invoke: async (cmd, args) => { state.__imageVSCode = { cmd, args }; return null; } };
  });
  await page.goto("/tests/images-fixture.html?mode=detail");
  await expect(page.getByText("/srv/images/reviewer")).toBeVisible();
  await page.getByRole("button", { name: "Open in VS Code" }).click();
  await expect.poll(() => page.evaluate(() => (window as Window & { __imageVSCode?: unknown }).__imageVSCode)).toEqual({
    cmd: "open_host_path_in_vscode", args: { hostId: "", path: "/srv/images/reviewer" },
  });
  await page.getByRole("link", { name: "Template" }).click();
  await expect(page.getByText("identity")).toBeVisible();
  await expect(page.getByText("context")).toBeVisible();

  await page.goto("/tests/images-fixture.html?mode=agent");
  const currentBasic = page.getByText("Current:").locator("..").getByRole("link", { name: "basic:latest" });
  await expect(currentBasic).toHaveAttribute("href", "/servers/local/images/basic/latest");
  const imageSelector = page.getByRole("combobox", { name: "Agent image" });
  await imageSelector.click();
  const builtOption = page.getByRole("option", { name: "browser-built:latest" });
  await expect(builtOption).toBeVisible();
  await builtOption.dispatchEvent("pointerdown", { pointerType: "mouse", button: 0 });
  await builtOption.dispatchEvent("pointerup", { pointerType: "mouse", button: 0 });
  await expect(imageSelector).toContainText("browser-built:latest");
  await page.getByRole("button", { name: "Use next iteration" }).click();
  await expect(page.getByText("Pending:").locator("..")).toContainText("browser-built:latest");
  await page.getByRole("button", { name: "Simulate next iteration" }).click();
  await expect(page.getByText("Current:").locator("..")).toContainText("browser-built:latest");
  await expect(page.getByText("Pending:")).toHaveCount(0);
});

test("transfers one source archive to ready non-source servers with target-bound requests", async ({ page }) => {
  await page.goto("/tests/images-fixture.html?mode=transfer");
  await page.getByRole("button", { name: "Upload to servers reviewer:v3" }).click();

  await page.getByRole("button", { name: "All servers" }).click();
  await expect(page.getByLabel("Transfer to This daemon (local)")).toBeChecked();
  await expect(page.getByLabel("Transfer to Ready target")).toBeChecked();
  await expect(page.getByLabel("Transfer to Already present target")).toBeChecked();
  await expect(page.getByLabel("Transfer to Conflict target")).toBeChecked();
  await expect(page.getByLabel("Transfer to Source server")).toHaveCount(0);
  await expect(page.getByLabel("Transfer to Unavailable target")).toHaveCount(0);

  await page.getByRole("button", { name: "Start transfer" }).click();
  await expect(page.getByText("Ready target: Completed")).toBeVisible();
  await expect(page.getByText("Already present target: Already present")).toBeVisible();
  await expect(page.getByText("Conflict target: Failed — target ref conflicts")).toBeVisible();
  await expect(page.getByLabel("Retag and retry for Conflict target")).toHaveValue("reviewer:v3");

  await expect.poll(() => page.evaluate(() => (window as Window & {
    __imageTransferRequests?: Array<{ method: string; url: string }>;
  }).__imageTransferRequests ?? [])).toEqual([
    { method: "GET", url: "https://source.tariboy.test/api/images/reviewer%3Av3/export" },
    { method: "POST", url: `${new URL(page.url()).origin}/api/image-imports` },
    { method: "POST", url: `${new URL(page.url()).origin}/api/image-imports/local-import/apply` },
    { method: "POST", url: "https://conflict.tariboy.test/api/image-imports" },
    { method: "POST", url: "https://conflict.tariboy.test/api/image-imports/conflict-import/apply" },
    { method: "POST", url: "https://ready.tariboy.test/api/image-imports" },
    { method: "POST", url: "https://ready.tariboy.test/api/image-imports/ready-import/apply" },
    { method: "POST", url: "https://present.tariboy.test/api/image-imports" },
    { method: "POST", url: "https://present.tariboy.test/api/image-imports/present-import/apply" },
  ]);
});

test("cancels every unstarted selected destination without requesting it", async ({ page }) => {
  await page.goto("/tests/images-fixture.html?mode=transfer-cancel");
  await page.getByRole("button", { name: "Upload to servers reviewer:v3" }).click();
  await page.getByRole("button", { name: "All servers" }).click();
  await expect(page.getByLabel("Transfer to In-flight target")).toBeChecked();
  await expect(page.getByLabel("Transfer to Cancelled target A")).toBeChecked();
  await expect(page.getByLabel("Transfer to Cancelled target B")).toBeChecked();

  await page.getByRole("button", { name: "Start transfer" }).click();
  await expect(page.getByText("In-flight target: Importing")).toBeVisible();
  await page.getByRole("button", { name: "Cancel transfer" }).click();
  await page.evaluate(() => (window as Window & { __finishImageTransfer?: () => void }).__finishImageTransfer?.());

  await expect(page.getByText("Cancelled target A: Cancelled")).toBeVisible();
  await expect(page.getByText("Cancelled target B: Cancelled")).toBeVisible();
  await expect.poll(() => page.evaluate(() => (window as Window & {
    __imageTransferRequests?: Array<{ method: string; url: string }>;
  }).__imageTransferRequests ?? [])).not.toContainEqual(
    expect.objectContaining({ url: expect.stringContaining("cancelled-a.tariboy.test") }),
  );
  await expect.poll(() => page.evaluate(() => (window as Window & {
    __imageTransferRequests?: Array<{ method: string; url: string }>;
  }).__imageTransferRequests ?? [])).not.toContainEqual(
    expect.objectContaining({ url: expect.stringContaining("cancelled-b.tariboy.test") }),
  );
});
