import { beforeEach, describe, expect, it } from "vitest";
import {
  SIDEBAR_PINNED_KEY,
  SIDEBAR_TAB_KEY,
  readPinnedAgents,
  readSidebarTab,
  writePinnedAgents,
  writeSidebarTab,
} from "./sidebarPrefs";

beforeEach(() => localStorage.clear());

describe("sidebar tab preference", () => {
  it("defaults to servers and survives a round trip", () => {
    expect(readSidebarTab()).toBe("servers");
    writeSidebarTab("agents");
    expect(readSidebarTab()).toBe("agents");
  });

  it("falls back to servers for a value that is not a tab", () => {
    localStorage.setItem(SIDEBAR_TAB_KEY, "__gone__");
    expect(readSidebarTab()).toBe("servers");
  });

  it("falls back to servers for the hidden groups tab", () => {
    localStorage.setItem(SIDEBAR_TAB_KEY, "groups");
    expect(readSidebarTab()).toBe("servers");
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
