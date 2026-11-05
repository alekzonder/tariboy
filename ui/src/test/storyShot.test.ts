import { mkdirSync, mkdtempSync, rmSync, utimesSync, writeFileSync } from "node:fs";
import type { AddressInfo } from "node:net";
import type { Server } from "node:http";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { expect, it } from "vitest";

// ui/scripts/story-shot.mjs has no type declarations, so the test types the
// five exports it uses.
type StoryShot = {
  parseArgs(argv: string[]): { id: string; theme: string; width: number; rebuild: boolean };
  storyUrl(origin: string, id: string, theme: string): string;
  shotPath(uiDir: string, id: string, theme: string, width: number): string;
  serveStatic(dir: string): Promise<Server>;
  slotCommand(env: Record<string, string | undefined>, argv: string[]): string[] | null;
};
const script = resolve(__dirname, "../../scripts/story-shot.mjs");
const load = () => import(/* @vite-ignore */ script) as Promise<StoryShot>;

it("parses the id, the theme and the width", async () => {
  const { parseArgs } = await load();
  expect(parseArgs(["a--b", "dark", "360"])).toEqual({
    id: "a--b",
    theme: "dark",
    width: 360,
    rebuild: false,
  });
  expect(parseArgs(["a--b", "light", "1024", "--rebuild"]).rebuild).toBe(true);
});

it("rejects a missing id", async () => {
  const { parseArgs } = await load();
  expect(() => parseArgs([])).toThrow(/usage/i);
});

it("rejects a theme other than light or dark", async () => {
  const { parseArgs } = await load();
  expect(() => parseArgs(["a--b", "system", "360"])).toThrow(/theme/);
});

it("rejects a width that is not a positive integer", async () => {
  const { parseArgs } = await load();
  for (const width of ["0", "-5", "3.5", "wide", ""])
    expect(() => parseArgs(["a--b", "light", width])).toThrow(/width/);
});

it("builds the iframe URL with the theme global", async () => {
  const { storyUrl } = await load();
  expect(storyUrl("http://127.0.0.1:41234", "mockups-home--width360", "dark")).toBe(
    "http://127.0.0.1:41234/iframe.html?id=mockups-home--width360&viewMode=story&globals=theme:dark",
  );
});

it("writes the PNG under .story-shots", async () => {
  const { shotPath } = await load();
  expect(shotPath("/x/ui", "a--b", "light", 360)).toBe("/x/ui/.story-shots/a--b.light.360.png");
});

it("serves only on loopback", async () => {
  const { serveStatic } = await load();
  const dir = mkdtempSync(join(tmpdir(), "story-shot-"));
  writeFileSync(join(dir, "hello.txt"), "hello");
  const server = await serveStatic(dir);
  try {
    const address = server.address() as AddressInfo;
    expect(address.address).toBe("127.0.0.1");
    expect(address.port).toBeGreaterThan(0);
    const response = await fetch(`http://127.0.0.1:${address.port}/hello.txt`);
    expect(await response.text()).toBe("hello");
    expect((await fetch(`http://127.0.0.1:${address.port}/../secret`)).status).toBe(404);
  } finally {
    await new Promise((done) => server.close(done));
    rmSync(dir, { recursive: true, force: true });
  }
});

it("takes the host check slot unless it already holds it", async () => {
  const { slotCommand } = await load();
  const argv = ["a--b", "light", "360"];
  const command = slotCommand({}, argv);
  expect(command?.[0]).toBe("flock");
  expect(command).toContain("/tmp/tariboy-check.lock");
  expect(command?.slice(-argv.length)).toEqual(argv);
  expect(slotCommand({ TARIBOY_CHECK_LOCK: "/run/x.lock" }, argv)).toContain("/run/x.lock");
  expect(slotCommand({ TARIBOY_CHECK_LOCK_HELD: "1" }, argv)).toBeNull();
});

// buildIsStale: the static build is stale when it is missing or an input file
// of the Storybook build is newer than storybook-static/index.json.
type Stale = { buildIsStale(uiDir: string): string | null };
const staleFixture = (touch: (dir: string, at: Date) => void) => {
  const dir = mkdtempSync(join(tmpdir(), "story-stale-"));
  const old = new Date(Date.now() - 60_000);
  const mk = (rel: string, at: Date) => {
    const file = join(dir, rel);
    mkdirSync(dirname(file), { recursive: true });
    writeFileSync(file, "x");
    utimesSync(file, at, at);
  };
  for (const rel of ["src/a.ts", ".storybook/main.ts", "vite.storybook.config.ts", "README.md"])
    mk(rel, old);
  mk("storybook-static/index.json", new Date(Date.now() - 30_000));
  touch(dir, new Date());
  return { dir, mk, cleanup: () => rmSync(dir, { recursive: true, force: true }) };
};

it("reports a missing build as stale", async () => {
  const { buildIsStale } = (await load()) as unknown as Stale;
  const dir = mkdtempSync(join(tmpdir(), "story-stale-"));
  try {
    expect(buildIsStale(dir)).toMatch(/missing/);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

it("reports a build newer than every input as fresh", async () => {
  const { buildIsStale } = (await load()) as unknown as Stale;
  const f = staleFixture(() => {});
  try {
    expect(buildIsStale(f.dir)).toBeNull();
  } finally {
    f.cleanup();
  }
});

it.each(["src/a.ts", "src/new/deep.tsx", ".storybook/main.ts", "vite.storybook.config.ts"])(
  "reports a build older than %s as stale",
  async (rel) => {
    const { buildIsStale } = (await load()) as unknown as Stale;
    const f = staleFixture(() => {});
    try {
      f.mk(rel, new Date());
      expect(buildIsStale(f.dir)).toContain(rel);
    } finally {
      f.cleanup();
    }
  },
);

it("ignores a newer file that is not an input", async () => {
  const { buildIsStale } = (await load()) as unknown as Stale;
  const f = staleFixture(() => {});
  try {
    f.mk("README.md", new Date());
    expect(buildIsStale(f.dir)).toBeNull();
  } finally {
    f.cleanup();
  }
});
