import { readFileSync, readdirSync } from "node:fs";
import { join, resolve } from "node:path";
import { transformWithOxc } from "vite";
import { expect, it } from "vitest";

// src/types/ holds types only, so a story or a preview can import a response
// type without loading the API client. A file there must compile to no
// JavaScript. oxc removes only `import type`/`export type` under
// onlyRemoveTypeImports, which is what verbatimModuleSyntax does in tsc.
const src = resolve(__dirname, "..");
const typesDir = join(src, "types");
const apiDir = join(src, "lib/api");

const domains = [
  "agents",
  "audit",
  "daemon",
  "files",
  "groups",
  "harnesses",
  "imageSources",
  "images",
  "iterations",
  "mail",
  "metrics",
  "onboarding",
  "operations",
  "plugins",
  "scripts",
  "stores",
  "tasks",
  "usage",
  "workflows",
];

function typeFiles(): string[] {
  try {
    return readdirSync(typesDir).filter((name) => name.endsWith(".ts"));
  } catch {
    return [];
  }
}

it("src/types holds a file per domain and an index, and every file compiles to no JavaScript", async () => {
  const files = typeFiles();
  expect(files.length).toBeGreaterThanOrEqual(domains.length + 1);
  for (const file of files) {
    const out = await transformWithOxc(readFileSync(join(typesDir, file), "utf8"), file, {
      lang: "ts",
      typescript: { onlyRemoveTypeImports: true },
    });
    const js = out.code.replace(/^export \{\};$/gm, "").trim();
    expect({ file, js }).toEqual({ file, js: "" });
  }
});

it.each(domains)("lib/api/%s.ts re-exports its types from @/types", (domain) => {
  const text = readFileSync(join(apiDir, `${domain}.ts`), "utf8");
  expect(text).toContain(`from "@/types/${domain}"`);
});
