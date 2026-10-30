import { describe, expect, it } from "vitest";
import type { DaemonStatus, OnboardingStatus } from "@/lib/api";
import { imageSummary } from "@/test/fixtures/apiFixtures";
import {
  bannerMessage,
  checklistRows,
  imageChoices,
  onboardingSettled,
  pickHarness,
  starterWaitReason,
  suggestAgentName,
} from "./firstRun";

const harnesses = (found: string[]) =>
  ["claude", "codex", "opencode", "cursor"].map((type) => ({
    type,
    found: found.includes(type),
    install: `install ${type}`,
  }));

const status = (
  store: string,
  starter: string,
  found: string[] = ["claude"],
  errors: { store?: string; starter?: string } = {},
): OnboardingStatus => ({
  agents: 0,
  default_store: {
    state: store,
    name: "tariboy",
    source: "https://example.test/store.git",
    error: errors.store,
  },
  starter_image: { state: starter, error: errors.starter },
  harnesses: harnesses(found),
});

describe("checklistRows", () => {
  it("reports every row done on a ready host", () => {
    expect(checklistRows(status("registered", "ready"))).toEqual([
      { key: "catalog", state: "done", text: "Tariboy catalog connected" },
      { key: "starter", state: "done", text: 'Starter image "basic" ready' },
      {
        key: "tool",
        state: "done",
        text: "Coding tool: Claude Code found. Codex, OpenCode, Cursor not found.",
        install: harnesses(["claude"]).slice(1),
      },
    ]);
  });

  it("treats an existing basic image as ready", () => {
    expect(checklistRows(status("registered", "skipped"))[1].state).toBe("done");
  });

  it("shows the working rows while the daemon connects and builds", () => {
    const rows = checklistRows(status("pending", "pending"));
    expect(rows.map((row) => [row.state, row.text])).toEqual([
      ["working", "Connecting the Tariboy catalog…"],
      ["working", "Building the starter image…"],
      ["done", "Coding tool: Claude Code found. Codex, OpenCode, Cursor not found."],
    ]);
  });

  it("keeps the raw error of a failed row for Details", () => {
    const rows = checklistRows(
      status("failed", "pending", ["claude"], { store: "git clone: network down" }),
    );
    // The starter image waits for the catalog, so only the catalog row explains it.
    expect(rows.map((row) => row.key)).toEqual(["catalog", "tool"]);
    expect(rows[0]).toEqual({
      key: "catalog",
      state: "failed",
      text: "Could not connect the Tariboy catalog",
      error: "git clone: network down",
    });
    const starter = checklistRows(
      status("registered", "failed", ["claude"], { starter: "npx: not found" }),
    )[1];
    expect(starter).toMatchObject({ state: "failed", error: "npx: not found" });
    expect(starter.text).toBe('Could not build the starter image "basic"');
  });

  it("points a removed catalog at the Stores page and drops the waiting starter row", () => {
    const rows = checklistRows(status("removed", "pending"));
    expect(rows.map((row) => row.key)).toEqual(["catalog", "tool"]);
    expect(rows[0]).toMatchObject({
      state: "warn",
      text: "Tariboy catalog removed. Add it again on the Stores page.",
    });
  });

  it("warns when no coding tool is installed", () => {
    const tool = checklistRows(status("registered", "ready", []))[2];
    expect(tool.state).toBe("warn");
    expect(tool.text).toBe(
      "Coding tool: none found. Install Claude Code, Codex, OpenCode, Cursor.",
    );
    expect(tool.install).toHaveLength(4);
  });
});

describe("onboardingSettled", () => {
  it("polls only while a row can still change by itself", () => {
    expect(onboardingSettled(status("pending", "pending"))).toBe(false);
    expect(onboardingSettled(status("registered", "pending"))).toBe(false);
    expect(onboardingSettled(status("registered", "building"))).toBe(false);
    expect(onboardingSettled(status("registered", "ready"))).toBe(true);
    expect(onboardingSettled(status("failed", "pending"))).toBe(true);
    expect(onboardingSettled(status("registered", "failed"))).toBe(true);
    expect(onboardingSettled(status("removed", "pending"))).toBe(true);
  });
});

describe("starterWaitReason", () => {
  it("names the unfinished row the starter image waits for", () => {
    expect(starterWaitReason(status("pending", "pending"))).toBe(
      "Waiting for the Tariboy catalog…",
    );
    expect(starterWaitReason(status("registered", "building"))).toBe(
      "Waiting for the starter image…",
    );
    expect(starterWaitReason(status("registered", "ready"))).toBeNull();
    expect(starterWaitReason(status("failed", "pending"))).toBeNull();
    expect(starterWaitReason(null)).toBeNull();
  });
});

describe("imageChoices", () => {
  const images = [
    imageSummary({ name: "zeta", tag: "v1", built_at: "2026-01-01T00:00:00Z" }),
    imageSummary({ name: "bare", tag: "latest", bare: true, built_at: "2026-01-01T00:00:00Z" }),
    imageSummary({ name: "alpha", tag: "old", built_at: "2026-01-01T00:00:00Z" }),
    imageSummary({
      name: "alpha",
      tag: "new",
      built_at: "2026-03-01T00:00:00Z",
      image_version: "1.2.0",
      source: "tariboy/alpha",
    }),
    imageSummary({ name: "basic", tag: "latest", built_at: "2026-02-01T00:00:00Z" }),
    imageSummary({ name: "alpha", tag: "mid", built_at: "2026-02-01T00:00:00Z" }),
  ];

  it("lists one row per name, basic first, bare last, others A-Z", () => {
    expect(imageChoices(images).map((choice) => choice.label)).toEqual([
      "basic",
      "alpha",
      "zeta",
      "Plain terminal (bare)",
    ]);
  });

  it("picks the newest built tag and keeps every tag for the version select", () => {
    const alpha = imageChoices(images).find((choice) => choice.name === "alpha");
    expect(alpha).toMatchObject({
      ref: "alpha:new",
      version: "1.2.0",
      source: "tariboy/alpha",
      tags: ["new", "mid", "old"],
    });
  });

  it("adds a waiting basic row while the daemon still makes it", () => {
    const bareOnly = [images[1]];
    expect(imageChoices(bareOnly, "Waiting for the starter image…")).toEqual([
      expect.objectContaining({ name: "basic", waiting: "Waiting for the starter image…" }),
      expect.objectContaining({ name: "bare" }),
    ]);
    expect(imageChoices(images, "Waiting…").filter((c) => c.name === "basic")).toHaveLength(1);
  });
});

describe("pickHarness", () => {
  it("keeps the image default when it is installed", () => {
    expect(pickHarness("codex", harnesses(["claude", "codex"]))).toBe("codex");
  });

  it("falls back to the first installed tool in detection order", () => {
    expect(pickHarness("codex", harnesses(["cursor", "opencode"]))).toBe("opencode");
    expect(pickHarness("", harnesses(["claude"]))).toBe("claude");
  });

  it("keeps the image default when nothing is installed or nothing is known", () => {
    expect(pickHarness("codex", harnesses([]))).toBe("codex");
    expect(pickHarness("codex", null)).toBe("codex");
  });
});

describe("suggestAgentName", () => {
  it("joins the folder basename and the image name", () => {
    expect(suggestAgentName("~/code/my-app", "basic", [])).toBe("my-app-basic");
    expect(suggestAgentName("/srv/My App/", "basic", [])).toBe("my-app-basic");
  });

  it("uses the image name for a managed folder", () => {
    expect(suggestAgentName("", "basic", [])).toBe("basic");
  });

  it("adds a numeric suffix until the name is free", () => {
    expect(suggestAgentName("/a/app", "basic", ["app-basic", "app-basic-2"])).toBe("app-basic-3");
  });
});

describe("bannerMessage", () => {
  const daemon = (tools: { name: string; purpose: string }[]): DaemonStatus => ({
    base_dir: "",
    http_addr: "",
    pid: 1,
    schema_version: 1,
    started_at: "",
    uptime_seconds: 1,
    version: "",
    state: "error",
    message: "missing required tools: everything",
    missing_tools: tools.map((tool) => ({ ...tool, install: "" })),
  });
  const harness = { name: "claude | codex", purpose: "at least one agent harness" };

  it("shows no banner when only the coding tool is missing", () => {
    expect(bannerMessage(daemon([harness]))).toBeNull();
  });

  it("names only the other missing tools", () => {
    expect(bannerMessage(daemon([{ name: "tmux", purpose: "terminal" }, harness]))).toBe(
      "missing required tools: tmux; install them on the daemon host",
    );
  });

  it("keeps any other daemon error as it is", () => {
    expect(bannerMessage(daemon([{ name: "tmux", purpose: "terminal" }]))).toBe(
      "missing required tools: everything",
    );
    expect(bannerMessage({ ...daemon([]), state: "ready" })).toBeNull();
  });
});
