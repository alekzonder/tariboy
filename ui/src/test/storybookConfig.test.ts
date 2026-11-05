import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, it } from "vitest";
import main from "../../.storybook/main";

// Storybook must never reach a daemon (spec 9.1). The app's vite.config.ts
// proxies /api to 127.0.0.1:9990, so Storybook reads its own Vite file.

it("points Storybook at the Vite file without a proxy", () => {
  const framework = main.framework as {
    options: { builder: { viteConfigPath: string } };
  };
  expect(framework.options.builder.viteConfigPath).toBe("vite.storybook.config.ts");
  expect("viteFinal" in main).toBe(false);
});

it("has no /api proxy and no live listener in the Storybook Vite config", () => {
  // A source scan: neither loadConfigFromFile (rolldown rejects plugin objects
  // of another realm under vmThreads) nor an import (@tailwindcss/vite refuses
  // to load inside Vitest) can evaluate the file here.
  const source = readFileSync(resolve(__dirname, "../../vite.storybook.config.ts"), "utf8")
    .split("\n")
    .filter((line) => !line.trim().startsWith("//"))
    .join("\n");
  expect(source).not.toMatch(/proxy/);
  expect(source).not.toContain("/api");
  expect(source).not.toContain("127.0.0.1:9990");
  expect(source).not.toMatch(/\btest\s*:/);
  // Without it the page polls "This daemon (local)" at the relative /api.
  expect(source).toMatch(/define:\s*\{\s*"import\.meta\.env\.VITE_TARIBOY_NO_LOCAL_HOST":\s*'"1"'/);
});
