import { beforeEach, describe, expect, it } from "vitest";
import {
  SIDEBAR_AGENTS_KEY,
  SIDEBAR_PINNED_KEY,
  readAgentsOpen,
  readPinnedAgents,
  writeAgentsOpen,
  writePinnedAgents,
} from "./sidebarPrefs";

beforeEach(() => localStorage.clear());

describe("agent list preference", () => {
  it("opens by default and survives a round trip", () => {
    expect(readAgentsOpen()).toBe(true);
    writeAgentsOpen(false);
    expect(readAgentsOpen()).toBe(false);
    writeAgentsOpen(true);
    expect(readAgentsOpen()).toBe(true);
  });

  it("opens for an unknown value and ignores the former tab key", () => {
    localStorage.setItem(SIDEBAR_AGENTS_KEY, "__gone__");
    localStorage.setItem("terminals:sidebar-tab:v1", "agents");
    expect(readAgentsOpen()).toBe(true);
  });
});

describe("pinned agents", () => {
  it("round-trips the pinned keys", () => {
    writePinnedAgents(new Set(['["","alpha"]']));
    expect([...readPinnedAgents()]).toEqual(['["","alpha"]']);
  });

  it("ignores junk and an unknown version", () => {
    localStorage.setItem(SIDEBAR_PINNED_KEY, "not json");
    expect(readPinnedAgents().size).toBe(0);
    localStorage.setItem(SIDEBAR_PINNED_KEY, JSON.stringify({ version: 2, keys: ["x"] }));
    expect(readPinnedAgents().size).toBe(0);
    localStorage.setItem(SIDEBAR_PINNED_KEY, JSON.stringify({ version: 1, keys: ["x", 7] }));
    expect([...readPinnedAgents()]).toEqual(["x"]);
  });
});
