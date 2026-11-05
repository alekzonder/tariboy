import { mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { expect, it } from "vitest";

// CustomerQuestionNotifications opens the tasks websocket. A story or a mockup
// that imports it would reach a daemon, so no such file may.
const src = resolve(__dirname, "..");
const FORBIDDEN = /from\s+["'][^"']*\/CustomerQuestionNotifications["']/;

function walk(dir: string): string[] {
  if (!existsSync(dir)) return [];
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    return entry.isDirectory() ? walk(path) : [path];
  });
}

function isStoryFile(path: string, root: string): boolean {
  const rel = path.slice(root.length + 1).replaceAll("\\", "/");
  return rel.endsWith(".stories.tsx") || rel.startsWith("mockups/");
}

function findForbiddenImports(root: string): string[] {
  return walk(root)
    .filter((path) => isStoryFile(path, root))
    .filter((path) => FORBIDDEN.test(readFileSync(path, "utf8")))
    .map((path) => path.slice(root.length + 1));
}

it("keeps CustomerQuestionNotifications out of stories and mockups", () => {
  expect(findForbiddenImports(src)).toEqual([]);
});

it("finds the forbidden import in a scratch story and not a similar name", () => {
  const root = mkdtempSync(join(tmpdir(), "story-safety-"));
  try {
    writeFileSync(
      join(root, "Bad.stories.tsx"),
      'import { CustomerQuestionNotifications } from "@/components/CustomerQuestionNotifications";\n',
    );
    writeFileSync(
      join(root, "Good.stories.tsx"),
      'import { x } from "@/components/customerQuestionNotificationsContext";\n',
    );
    expect(findForbiddenImports(root)).toEqual(["Bad.stories.tsx"]);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
