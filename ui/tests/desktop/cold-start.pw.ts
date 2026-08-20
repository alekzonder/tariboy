import { expect, test } from "./fixture";

test("cold start opens the real Tauri Desktop workspace", async ({ desktop }) => {
  await expect(desktop.windowHandles()).resolves.toHaveLength(1);
  await expect
    .poll(async () => {
      return desktop.execute<{
        title: string;
        text: string;
        tauri: boolean;
      }>(`return {
        title: document.title,
        text: document.body ? document.body.innerText : "",
        tauri: typeof window.__TAURI_INTERNALS__ === "object"
      }`);
    })
    .toEqual(expect.objectContaining({ tauri: true, text: expect.stringContaining("Agents") }));

  const titlebarNavigation = await desktop.findElement(
    "css selector",
    '[data-testid="app-titlebar"] nav',
  );
  await expect(desktop.elementText(titlebarNavigation)).resolves.toBe("Workspace");

  const sidebar = await desktop.findElement("css selector", "aside");
  await expect(desktop.elementText(sidebar)).resolves.toContain("Agents");
});
