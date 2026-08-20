import { resolve } from "node:path";
import { expect, test, waitForMainWindow } from "./fixture";
import type { W3CClient } from "./w3c";

const sourceDir = resolve(process.cwd(), "../internal/builtinimages/source");

async function bodyText(desktop: W3CClient): Promise<string> {
  return desktop.execute<string>("return document.body ? document.body.innerText : '';");
}

test("builds a transparent image from its original directory and assigns it to an existing agent", async ({ desktop }) => {
  await waitForMainWindow(desktop);
  await desktop.execute(`window.__imageSetup = "pending";
    window.__TAURI_INTERNALS__.invoke("daemon_status").then(async (status) => {
      const response = await fetch(status.base_url + "/api/agents", {
        method: "POST", headers: { "content-type": "application/json" },
        body: JSON.stringify({ image: "basic:latest", name: "image-select-e2e", harness: "stub", loop: false }),
      });
      const payload = await response.json();
      if (!response.ok || !payload.ok) throw new Error(payload?.error?.message || "agent create failed");
      window.__imageSetup = "ready";
    }).catch((error) => { window.__imageSetup = "failed: " + String(error); });`);
  await expect.poll(() => desktop.execute<string>("return window.__imageSetup || '';"), { timeout: 60_000 }).toBe("ready");

  await desktop.execute(`window.location.hash = "#/servers/local/images";`);
  await expect.poll(() => bodyText(desktop)).toContain("Build from directory");
  await desktop.elementSendKeys(await desktop.findElement("css selector", 'input[aria-label="Image source directory"]'), sourceDir);
  await desktop.elementSendKeys(await desktop.findElement("css selector", 'input[aria-label="Image name"]'), "transparent-e2e");
  await desktop.elementClick(await desktop.findElement("xpath", "//button[normalize-space(.)='Build']"));
  await expect.poll(() => bodyText(desktop), { timeout: 60_000 }).toContain("transparent-e2e:latest");

  await desktop.execute(`window.location.hash = "#/servers/local/images/transparent-e2e/latest";`);
  await expect.poll(() => bodyText(desktop)).toContain(sourceDir);
  await expect.poll(async () => {
    try {
      return await desktop.elementText(await desktop.findElement("xpath", "//button[normalize-space(.)='Open in VS Code']"));
    } catch {
      return "";
    }
  }).toBe("Open in VS Code");

  await desktop.execute(`window.location.hash = "#/servers/local/images?tab=built";`);
  await expect.poll(() => bodyText(desktop)).toContain("transparent-e2e:latest");
  const sourceImagesRoute = await desktop.execute<string>("return window.location.hash;");
  const exportStarted = await desktop.execute<boolean>(`window.__desktopImageArchive = null;
    window.__desktopImageDownloadName = "";
    const originalCreateObjectURL = URL.createObjectURL.bind(URL);
    URL.createObjectURL = (blob) => { window.__desktopImageArchive = blob; return originalCreateObjectURL(blob); };
    HTMLAnchorElement.prototype.click = function () { window.__desktopImageDownloadName = this.download; };
    const button = document.querySelector('button[aria-label="Export transparent-e2e:latest"]');
    if (!(button instanceof HTMLButtonElement)) return false;
    button.click();
    return true;`);
  expect(exportStarted).toBe(true);
  await expect.poll(() => desktop.execute<number>("return window.__desktopImageArchive ? window.__desktopImageArchive.size : 0;"))
    .toBeGreaterThan(0);
  await expect.poll(() => desktop.execute<string>("return window.__desktopImageDownloadName || '';"))
    .toBe("transparent-e2e-latest.tariboy-image.tar.gz");
  await expect.poll(() => bodyText(desktop)).toContain(
    "image transparent-e2e:latest saved to file transparent-e2e-latest.tariboy-image.tar.gz",
  );
  const uploadStarted = await desktop.execute<boolean>(`const input = document.querySelector('input[aria-label="Import image archive"]');
    if (!(input instanceof HTMLInputElement) || !window.__desktopImageArchive) return false;
    const transfer = new DataTransfer();
    transfer.items.add(new File([window.__desktopImageArchive], "transparent-e2e-latest.tariboy-image.tar.gz", { type: "application/gzip" }));
    input.files = transfer.files;
    input.dispatchEvent(new Event("change", { bubbles: true }));
    return true;`);
  expect(uploadStarted).toBe(true);
  await expect.poll(() => bodyText(desktop)).toContain("Import image");
  await desktop.elementClick(await desktop.findElement("xpath", "//button[normalize-space(.)='Import image']"));
  await expect.poll(() => bodyText(desktop)).toContain("image imported");
  await expect.poll(() => desktop.execute<string>("return window.location.hash;")).toBe(sourceImagesRoute);

  await desktop.execute(`window.location.hash = "#/servers/local/images/transparent-e2e/latest/template";`);
  await expect.poll(() => bodyText(desktop)).toContain("Template");
  await expect.poll(() => bodyText(desktop)).toContain("identity");
  await expect.poll(() => bodyText(desktop)).toContain("$CURRENT_VERSION_STORE/skills/whoami/prompt.md");

  await desktop.execute(`window.location.hash = "#/agents/local/image-select-e2e/configuration";`);
  await expect.poll(() => bodyText(desktop)).toContain("Agent image");
  await expect.poll(() => desktop.execute<string>(`return [...document.querySelectorAll("a")]
    .find((candidate) => candidate.textContent?.trim() === "basic:latest")?.getAttribute("href") || "";`))
    .toBe("#/servers/local/images/basic/latest");
  const selector = await desktop.findElement("css selector", 'button[aria-label="Agent image"]');
  await desktop.elementClick(selector);
  const option = await expect.poll(async () => {
    try {
      return await desktop.findElement("xpath", "//*[@role='option' and normalize-space(.)='transparent-e2e:latest']");
    } catch {
      return null;
    }
  }).not.toBeNull().then(() => desktop.findElement("xpath", "//*[@role='option' and normalize-space(.)='transparent-e2e:latest']"));
  await desktop.elementClick(option);
  await desktop.elementClick(await desktop.findElement("xpath", "//button[normalize-space(.)='Use next iteration']"));
  await expect.poll(() => bodyText(desktop)).toContain("Pending: transparent-e2e:latest");

  await desktop.execute(`window.__imageActivation = "pending";
    window.__TAURI_INTERNALS__.invoke("daemon_status").then(async (status) => {
      const exec = await fetch(status.base_url + "/api/agents/image-select-e2e/exec", {
        method: "POST", headers: { "content-type": "application/json" },
        body: JSON.stringify({ prompt: "verify activated image" }),
      });
      if (!exec.ok) throw new Error("iteration launch failed");
      const deadline = Date.now() + 60000;
      while (Date.now() < deadline) {
        const [assignmentResponse, iterationsResponse] = await Promise.all([
          fetch(status.base_url + "/api/agents/image-select-e2e/image"),
          fetch(status.base_url + "/api/agents/image-select-e2e/iterations"),
        ]);
        const assignment = await assignmentResponse.json();
        const iterations = await iterationsResponse.json();
        const rows = iterations?.result?.iterations || [];
        const latest = rows[rows.length - 1];
        const detailResponse = latest ? await fetch(status.base_url + "/api/agents/image-select-e2e/iterations/" + encodeURIComponent(latest.id)) : null;
        const detail = detailResponse ? await detailResponse.json() : null;
        if (assignment?.result?.current?.ref === "transparent-e2e:latest" &&
            detail?.result?.image_ref === "transparent-e2e:latest" && detail?.result?.prompt_template_sha256) {
          window.__imageActivation = "ready";
          return;
        }
        await new Promise((resolve) => setTimeout(resolve, 250));
      }
      throw new Error("activated image snapshot not observed");
    }).catch((error) => { window.__imageActivation = "failed: " + String(error); });`);
  await expect.poll(() => desktop.execute<string>("return window.__imageActivation || '';"), { timeout: 70_000 }).toBe("ready");
});
