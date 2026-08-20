import { expect, test, waitForMainWindow } from "./fixture";
import type { W3CClient } from "./w3c";

async function bodyText(desktop: W3CClient): Promise<string> {
  return desktop.execute<string>("return document.body ? document.body.innerText : '';");
}

test.use({
  usageSeed: {
    groups: [
      { name: "alpha", lead: "alice-alpha", agent: "alice-alpha" },
      { name: "beta", lead: "bob-beta", agent: "bob-beta" },
    ],
    requests: [
      { id: "alpha-new", ts: "2026-08-18T12:00:00Z", agent: "alice-alpha", model: "alpha-model-new", inputTokens: 120, outputTokens: 60, costUSD: 0.008, group: "alpha" },
      { id: "alpha-old", ts: "2026-08-17T12:00:00Z", agent: "alice-alpha", model: "alpha-model-old", inputTokens: 80, outputTokens: 40, costUSD: 0.004, group: "alpha" },
      { id: "beta", ts: "2026-08-18T11:00:00Z", agent: "bob-beta", model: "beta-model", inputTokens: 70, outputTokens: 30, costUSD: 0.003, group: "beta" },
      { id: "ungrouped", ts: "2026-08-18T10:00:00Z", agent: "legacy-ungrouped", model: "ungrouped-model", inputTokens: 20, outputTokens: 10, costUSD: 0.001 },
    ],
  },
});

test("filters request-time group snapshots in the production Desktop Usage view", async ({ desktop }) => {
  await waitForMainWindow(desktop);

  await desktop.elementClick(await desktop.findElement(
    "xpath",
    "//button[@aria-label='Open server This daemon (local)']",
  ));
  await expect.poll(() => desktop.execute<string>("return window.location.hash;")).toBe("#/servers/local/tasks");
  const settingsLink = await expect.poll(async () => {
    try {
      return await desktop.findElement("xpath", "//a[normalize-space(.)='Settings']");
    } catch {
      return null;
    }
  }).not.toBeNull().then(() => desktop.findElement("xpath", "//a[normalize-space(.)='Settings']"));
  await desktop.elementClick(settingsLink);
  await expect.poll(() => desktop.execute<string>("return window.location.hash;")).toBe("#/servers/local/settings");
  const advancedLink = await expect.poll(async () => {
    try {
      return await desktop.findElement("xpath", "//a[normalize-space(.)='Advanced']");
    } catch {
      return null;
    }
  }).not.toBeNull().then(() => desktop.findElement("xpath", "//a[normalize-space(.)='Advanced']"));
  await desktop.elementClick(advancedLink);
  await expect.poll(() => desktop.execute<string>("return window.location.hash;")).toBe("#/servers/local/settings/advanced");
  const usageLink = await expect.poll(async () => {
    try {
      return await desktop.findElement("xpath", "//a[normalize-space(.)='Usage']");
    } catch {
      return null;
    }
  }).not.toBeNull().then(() => desktop.findElement("xpath", "//a[normalize-space(.)='Usage']"));
  await desktop.elementClick(usageLink);
  await expect.poll(() => desktop.execute<string>("return window.location.hash;")).toBe("#/servers/local/settings/advanced/usage");

  await expect.poll(() => bodyText(desktop)).toContain("Usage by agent");
  await expect.poll(() => desktop.execute<boolean>(`
    return [...document.querySelectorAll('select[aria-label="Group"] option')]
      .some((option) => option.textContent?.trim() === "alpha");
  `)).toBe(true);

  await desktop.execute(`
    const select = document.querySelector('select[aria-label="Group"]');
    select.value = "alpha";
    select.dispatchEvent(new Event("change", { bubbles: true }));
    return true;
  `);

  await expect.poll(() => desktop.execute<string>(`
    return document.querySelector('[aria-label="Usage summary"]')?.textContent || '';
  `)).toMatch(/Requests\s*2/);
  await expect.poll(() => desktop.execute<string>(`
    return document.querySelector('[aria-labelledby="usage-aggregates-heading"]')?.textContent || '';
  `)).toContain("alice-alpha");

  const filtered = await desktop.execute<string>(`
    const aggregate = document.querySelector('[aria-labelledby="usage-aggregates-heading"]')?.textContent || '';
    const recent = document.querySelector('[aria-labelledby="recent-requests-heading"]')?.textContent || '';
    return aggregate + recent;
  `);
  expect(filtered).toContain("alpha-model-new");
  expect(filtered).toContain("alpha-model-old");
  expect(filtered).not.toContain("bob-beta");
  expect(filtered).not.toContain("legacy-ungrouped");
  expect(filtered).not.toContain("beta-model");
  expect(filtered).not.toContain("ungrouped-model");
  expect(filtered).not.toContain("Ungrouped");
});
