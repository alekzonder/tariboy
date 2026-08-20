import { expect, test, waitForMainWindow } from "./fixture";

test("selects a harness in the New agent dialog", async ({ desktop }) => {
  await waitForMainWindow(desktop);

  const openDialog = await desktop.findElement(
    "xpath",
    "//main//button[normalize-space(.)='New agent']",
  );
  await expect(desktop.elementText(openDialog)).resolves.toBe("New agent");
  await desktop.elementClick(openDialog);

  const harness = await expect.poll(async () => {
    try {
      return await desktop.findElement(
        "css selector",
        "#create-agent-harness",
      );
    } catch {
      return null;
    }
  }).not.toBeNull().then(async () => desktop.findElement("css selector", "#create-agent-harness"));

  await expect(desktop.elementProperty(harness, "value")).resolves.not.toBe("codex");
  await desktop.elementSendKeys(harness, "codex");
  await expect.poll(() => desktop.elementProperty(harness, "value")).toBe("codex");
});
