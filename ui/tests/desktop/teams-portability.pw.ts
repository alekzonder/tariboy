import { resolve } from "node:path";
import { expect, test } from "./fixture";
import type { W3CClient } from "./w3c";

const sourceDir = resolve(process.cwd(), "../internal/builtinimages/source");

async function openPath(desktop: W3CClient, path: string): Promise<void> {
  await desktop.execute(`window.location.hash = ${JSON.stringify(`#${path}`)};`);
}

async function bodyText(desktop: W3CClient): Promise<string> {
  return desktop.execute<string>("return document.body ? document.body.innerText : '';");
}

test("real Desktop creates, separates, copies, exports, previews, and imports a team", async ({ desktop }) => {
  await desktop.execute(`window.__desktopApiBase = "";
    const originalFetch = window.fetch.bind(window);
    window.fetch = (input, init) => {
      const url = typeof input === "string" ? input : input.url;
      const marker = url.indexOf("/api/");
      if (marker > 0) window.__desktopApiBase = url.slice(0, marker);
      return originalFetch(input, init);
    };`);
  await openPath(desktop, "/servers/local/settings/advanced/groups");
  await expect.poll(() => bodyText(desktop)).toContain("New group (wizard)");

  const created = await desktop.execute<boolean>(`const input = document.querySelector('input[placeholder="group name"]');
    const button = [...document.querySelectorAll('button')].find((candidate) => candidate.textContent?.trim() === "Create");
    if (!(input instanceof HTMLInputElement) || !(button instanceof HTMLButtonElement)) return false;
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set.call(input, "desktop-team");
    input.dispatchEvent(new Event("input", { bubbles: true }));
    button.click();
    return true;`);
  expect(created).toBe(true);
  await expect.poll(() => bodyText(desktop)).toContain("desktop-team");

  await desktop.execute(`window.__desktopSetup = "running";
    (async () => {
      const call = async (method, path, body) => {
        const response = await fetch(window.__desktopApiBase + path, { method, headers: body === undefined ? undefined : { "content-type": "application/json" }, body: body === undefined ? undefined : JSON.stringify(body) });
        const envelope = await response.json();
        if (!response.ok || !envelope.ok) throw new Error(path + ": " + JSON.stringify(envelope.error));
        return envelope.result;
      };
      await call("POST", "/api/images/build", { name: "portable-image", tag: "v1", path: ${JSON.stringify(sourceDir)} });
      await call("POST", "/api/agents", { image: "portable-image:v1", name: "desktop-worker", group: "desktop-team", harness: "codex", loop: false });
      window.__desktopSetup = "complete";
    })().catch((error) => { window.__desktopSetup = "failed: " + String(error); });`);
  await expect.poll(() => desktop.execute<string>("return window.__desktopSetup || '';"), { timeout: 60_000 })
    .toBe("complete");

  await openPath(desktop, "/");
  await expect.poll(() => bodyText(desktop)).toContain("Teams");
  await expect.poll(() => bodyText(desktop)).toContain("desktop-team");
  await expect.poll(() => bodyText(desktop)).toContain("Individual agents");

  await openPath(desktop, "/servers/local/settings/advanced/groups?team=desktop-team");
  await expect.poll(() => bodyText(desktop)).toContain("Copy YAML");
  const copyStarted = await desktop.execute<boolean>(`window.__desktopCopiedYAML = "";
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText: async (text) => { window.__desktopCopiedYAML = text; } } });
    const button = [...document.querySelectorAll('button')].find((candidate) => candidate.textContent?.trim() === "Copy YAML");
    if (!(button instanceof HTMLButtonElement)) return false;
    button.click();
    return true;`);
  expect(copyStarted).toBe(true);
  await expect.poll(() => desktop.execute<string>("return window.__desktopCopiedYAML || '';"))
    .toContain("desktop-team");

  const exportStarted = await desktop.execute<boolean>(`window.__desktopTeamArchive = null;
    window.__desktopTeamDownloadName = "";
    const originalCreateObjectURL = URL.createObjectURL.bind(URL);
    URL.createObjectURL = (blob) => { window.__desktopTeamArchive = blob; return originalCreateObjectURL(blob); };
    HTMLAnchorElement.prototype.click = function () { window.__desktopTeamDownloadName = this.download; };
    const button = [...document.querySelectorAll('button')].find((candidate) => candidate.textContent?.trim() === "Export archive");
    if (!(button instanceof HTMLButtonElement)) return false;
    button.click();
    return true;`);
  expect(exportStarted).toBe(true);
  await expect.poll(() => desktop.execute<number>("return window.__desktopTeamArchive ? window.__desktopTeamArchive.size : 0;"))
    .toBeGreaterThan(0);
  await expect.poll(() => desktop.execute<string>("return window.__desktopTeamDownloadName || '';"))
    .toBe("desktop-team.tar.gz");

  await desktop.execute(`window.__desktopCleanup = "running";
    (async () => {
      const call = async (method, path) => {
        const response = await fetch(window.__desktopApiBase + path, { method });
        const envelope = await response.json();
        if (!response.ok || !envelope.ok) throw new Error(path + ": " + JSON.stringify(envelope.error));
      };
      await call("DELETE", "/api/agents/desktop-worker?force=true&purge=true");
      await call("DELETE", "/api/groups/desktop-team");
      window.__desktopCleanup = "complete";
    })().catch((error) => { window.__desktopCleanup = "failed: " + String(error); });`);
  await expect.poll(() => desktop.execute<string>("return window.__desktopCleanup || '';"), { timeout: 60_000 })
    .toBe("complete");

  const uploadStarted = await desktop.execute<boolean>(`const input = document.querySelector('input[aria-label="Import team archive"]');
    if (!(input instanceof HTMLInputElement) || !window.__desktopTeamArchive) return false;
    const transfer = new DataTransfer();
    transfer.items.add(new File([window.__desktopTeamArchive], "desktop-team.tar.gz", { type: "application/gzip" }));
    input.files = transfer.files;
    input.dispatchEvent(new Event("change", { bubbles: true }));
    return true;`);
  expect(uploadStarted).toBe(true);
  await expect.poll(() => bodyText(desktop)).toContain("Images: none");
  expect(await bodyText(desktop)).not.toContain("Portable Desktop source");

  const applyStarted = await desktop.execute<boolean>(`const yaml = document.querySelector('textarea[aria-label="Imported team compose YAML"]');
    const button = document.querySelector('button[aria-label="Confirm and import team"]');
    if (!(yaml instanceof HTMLTextAreaElement) || !(button instanceof HTMLButtonElement)) return false;
    button.click();
    return true;`);
  expect(applyStarted).toBe(true);
  await expect.poll(() => bodyText(desktop), { timeout: 60_000 }).toContain("Import status: complete");

  await openPath(desktop, "/servers/local/settings/advanced/groups?team=desktop-team");
  await expect.poll(() => bodyText(desktop)).toContain("desktop-worker");
  await openPath(desktop, "/servers/local/images?tab=built");
  await expect.poll(() => bodyText(desktop)).toContain("portable-image");
});
