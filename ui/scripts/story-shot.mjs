#!/usr/bin/env node
// One PNG of one story, in a theme and a width, for the designer:
//   cd ui && npm run story:shot -- <story-id> <light|dark> <width> [--rebuild]
// It serves the static Storybook build on loopback and opens the story in
// Playwright chromium. It never reaches a daemon: Storybook has no /api proxy.
import { spawnSync } from "node:child_process";
import { existsSync, mkdirSync, readdirSync, readFileSync, realpathSync, statSync } from "node:fs";
import http from "node:http";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const uiDir = path.resolve(here, "..");
const USAGE = "usage: npm run story:shot -- <story-id> <light|dark> <width> [--rebuild]";

export function parseArgs(argv) {
  const rebuild = argv.includes("--rebuild");
  const [id, theme, width] = argv.filter((arg) => arg !== "--rebuild");
  if (!id) throw new Error(USAGE);
  if (theme !== "light" && theme !== "dark")
    throw new Error(`theme must be light or dark, got ${JSON.stringify(theme)}\n${USAGE}`);
  if (!/^[1-9]\d*$/.test(width ?? ""))
    throw new Error(`width must be a positive integer, got ${JSON.stringify(width)}\n${USAGE}`);
  return { id, theme, width: Number(width), rebuild };
}

export function storyUrl(origin, id, theme) {
  return `${origin}/iframe.html?id=${encodeURIComponent(id)}&viewMode=story&globals=theme:${theme}`;
}

export function shotPath(dir, id, theme, width) {
  return path.join(dir, ".story-shots", `${id}.${theme}.${width}.png`);
}

const TYPES = {
  ".html": "text/html",
  ".js": "text/javascript",
  ".mjs": "text/javascript",
  ".css": "text/css",
  ".json": "application/json",
  ".svg": "image/svg+xml",
  ".woff2": "font/woff2",
  ".woff": "font/woff",
  ".png": "image/png",
};

export function serveStatic(dir) {
  const root = path.resolve(dir);
  const server = http.createServer((req, res) => {
    const pathname = decodeURIComponent(new URL(req.url ?? "/", "http://x").pathname);
    const file = path.join(root, pathname === "/" ? "index.html" : pathname);
    if (!file.startsWith(root + path.sep) || !existsSync(file) || !statSync(file).isFile()) {
      res.writeHead(404).end("not found");
      return;
    }
    res.writeHead(200, { "content-type": TYPES[path.extname(file)] ?? "application/octet-stream" });
    res.end(readFileSync(file));
  });
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => resolve(server));
  });
}

// The host runs one check at a time (CHECK_SLOT of the Makefile). The script
// runs itself again under the same flock; the child marks that it holds it.
export function slotCommand(env, argv) {
  if (env.TARIBOY_CHECK_LOCK_HELD) return null;
  const lock = env.TARIBOY_CHECK_LOCK ?? "/tmp/tariboy-check.lock";
  return ["flock", "-w", "3600", lock, process.execPath, fileURLToPath(import.meta.url), ...argv];
}

function takeSlot(argv) {
  const command = slotCommand(process.env, argv);
  if (!command) return;
  const run = spawnSync(command[0], command.slice(1), {
    stdio: "inherit",
    env: { ...process.env, TARIBOY_CHECK_LOCK_HELD: "1" },
  });
  // shortcut: a host without flock runs unlocked (as the Makefile does), upgrade if one needs the slot there
  if (run.error?.code === "ENOENT") return;
  process.exit(run.status ?? 1);
}

// Everything main.ts and vite.storybook.config.ts read.
const INPUT_DIRS = ["src", ".storybook"];
const INPUT_FILES = [
  "vite.storybook.config.ts",
  "vite.fsAllow.ts",
  "package.json",
  "package-lock.json",
  "tsconfig.json",
  "tsconfig.app.json",
  "tsconfig.storybook.json",
];

function* walk(dir) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const file = path.join(dir, entry.name);
    if (entry.isDirectory()) yield* walk(file);
    else yield file;
  }
}

// Returns why the build must be redone, or null when it is fresh.
// shortcut: compares mtimes, so a file restored with an old mtime (cp -p) is missed, use --rebuild then
export function buildIsStale(dir) {
  const index = path.join(dir, "storybook-static", "index.json");
  if (!existsSync(index)) return "storybook-static is missing";
  const built = statSync(index).mtimeMs;
  const inputs = [
    ...INPUT_DIRS.filter((d) => existsSync(path.join(dir, d))).flatMap((d) => [
      ...walk(path.join(dir, d)),
    ]),
    ...INPUT_FILES.map((f) => path.join(dir, f)).filter((f) => existsSync(f)),
  ];
  const newer = inputs.find((file) => statSync(file).mtimeMs > built);
  return newer ? `storybook-static is stale: ${path.relative(dir, newer)} is newer` : null;
}

function ensureBuild(staticDir, rebuild) {
  const reason = rebuild ? "rebuild requested" : buildIsStale(uiDir);
  if (!reason) {
    console.error("storybook-static is fresh");
    return;
  }
  console.error(reason);
  const build = spawnSync(
    "npm",
    ["run", "build-storybook", "--", "--output-dir", "storybook-static", "--quiet"],
    { cwd: uiDir, stdio: "inherit" },
  );
  if (build.status !== 0) throw new Error("build-storybook failed");
}

function ensureStory(staticDir, id) {
  const index = JSON.parse(readFileSync(path.join(staticDir, "index.json"), "utf8"));
  if (index.entries?.[id]) return;
  const known = Object.keys(index.entries ?? {}).join("\n  ");
  throw new Error(`no story with id ${id}. Known ids:\n  ${known}`);
}

async function shoot({ id, theme, width }, staticDir) {
  const { chromium } = await import("playwright");
  const server = await serveStatic(staticDir);
  const browser = await chromium.launch();
  try {
    const origin = `http://127.0.0.1:${server.address().port}`;
    const page = await browser.newPage({ viewport: { width, height: 800 } });
    await page.goto(storyUrl(origin, id, theme));
    await page.waitForSelector("body.sb-show-main, body.sb-show-errordisplay");
    if (await page.locator("body.sb-show-errordisplay").count())
      throw new Error(`story ${id} failed to render`);
    await page.evaluate(() => document.fonts.ready);
    // Wait for finite transitions (the theme switch) so the frame is the end state.
    // Infinite animations (a spinner) are skipped, or the wait would never end.
    await page.evaluate(async () => {
      const finite = document
        .getAnimations()
        .filter((a) => Number.isFinite(a.effect?.getComputedTiming().endTime ?? Infinity));
      await Promise.race([
        Promise.allSettled(finite.map((a) => a.finished)),
        new Promise((done) => setTimeout(done, 2000)),
      ]);
      await new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done)));
    });
    const file = shotPath(uiDir, id, theme, width);
    mkdirSync(path.dirname(file), { recursive: true });
    await page.screenshot({ path: file, fullPage: true });
    return file;
  } finally {
    await browser.close();
    await new Promise((done) => server.close(done));
  }
}

async function main() {
  const argv = process.argv.slice(2);
  const args = parseArgs(argv);
  takeSlot(argv);
  const staticDir = path.join(uiDir, "storybook-static");
  ensureBuild(staticDir, args.rebuild);
  ensureStory(staticDir, args.id);
  console.log(await shoot(args, staticDir));
}

if (
  process.argv[1] &&
  realpathSync(process.argv[1]) === realpathSync(fileURLToPath(import.meta.url))
) {
  main().catch((error) => {
    console.error(error.message);
    process.exit(1);
  });
}
