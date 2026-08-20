import { expect, test } from "./fixture";

test("the real Desktop titlebar requests native dragging and keeps its sidebar control clickable", async ({ desktop }) => {
  await expect.poll(() => desktop.execute<boolean>(`
    return typeof window.__TAURI_INTERNALS__ === "object"
      && document.querySelector("header") !== null
      && document.visibilityState === "visible"
      && window.innerWidth > 0
      && window.innerHeight > 0;
  `)).toBe(true);

  await desktop.findElement(
    "css selector",
    "header[data-tauri-drag-region]",
  );
  const sidebarToggle = await desktop.findElement(
    "css selector",
    'header button[aria-label$="agents"]',
  );

  const initialLabel = await desktop.execute<string | null>(`
    const toggle = document.querySelector('header button[aria-label$="agents"]');
    return toggle instanceof HTMLButtonElement
      && !toggle.hasAttribute("data-tauri-drag-region")
      ? toggle.getAttribute("aria-label")
      : null;
  `);
  expect(["Hide agents", "Show agents"]).toContain(initialLabel);

  expect(await desktop.execute<boolean>(`
    const header = document.querySelector('[data-testid="app-titlebar"]');
    const links = header ? [...header.querySelectorAll("a")] : [];
    return links.length === 1
      && links[0].textContent.trim() === "Workspace"
      && links[0].getAttribute("href").endsWith("/workspace")
      && !links[0].hasAttribute("data-tauri-drag-region")
      && !["Agents", "Tasks", "Images", "Settings"].some(
        (label) => links.some((link) => link.textContent.trim() === label),
      );
  `)).toBe(true);

  expect(await desktop.execute<boolean>(`
    const spacer = document.querySelector("header > [aria-hidden].flex-1");
    if (!(spacer instanceof HTMLElement)) return false;
    return spacer.dispatchEvent(new MouseEvent("mousedown", {
      bubbles: true,
      cancelable: true,
      composed: true,
      button: 0,
      detail: 1,
    }));
  `)).toBe(false);

  await desktop.execute(`
    window.__titlebarDragIPC = "pending";
    window.__TAURI_INTERNALS__.invoke("plugin:window|start_dragging")
      .then(() => { window.__titlebarDragIPC = "resolved"; })
      .catch((error) => { window.__titlebarDragIPC = "rejected: " + String(error); });
    return true;
  `);
  await expect.poll(() => desktop.execute<string>(
    "return window.__titlebarDragIPC || 'missing';",
  )).toBe("resolved");

  await desktop.elementClick(sidebarToggle);
  await expect.poll(() => desktop.execute<string | null>(
    "return document.querySelector('header button[aria-label$=\"agents\"]')?.getAttribute('aria-label') ?? null;",
  )).toBe(initialLabel === "Hide agents" ? "Show agents" : "Hide agents");
});
