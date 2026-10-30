import { describe, expect, it } from "vitest";
import type { HostAgents } from "@/lib/aggregate";
import type { DaemonMeta } from "@/lib/daemons";
import {
  agentPath,
  buildCreateHosts,
  buildWorkspaceHosts,
  fallbackHostLabel,
  hostLabelFor,
  mergeVisibleServerOrder,
  parseServerOrder,
  rankHosts,
  routeHost,
  withLastSuccessful,
} from "./terminalsHosts";

const host = (id: string, patch: Partial<HostAgents> = {}): HostAgents => ({
  host: { id, label: `label-${id}` },
  agents: [],
  ...patch,
});
const daemon = (id: string, patch: Partial<DaemonMeta> = {}): DaemonMeta => ({
  id,
  label: `daemon-${id}`,
  baseURL: `https://${id}`,
  ...patch,
});

describe("buildCreateHosts", () => {
  it("lists the local daemon, aggregate hosts and registry hosts once each", () => {
    const hosts = buildCreateHosts(
      [host("a", { error: "down" })],
      [daemon("a"), daemon("b")],
      true,
    );
    expect(hosts).toEqual([
      { id: "", label: "This daemon (local)", revision: "pending\u0000\u0000", agents: [] },
      { id: "a", label: "label-a", revision: "down\u0000\u0000https://a", agents: [] },
      { id: "b", label: "daemon-b", revision: "pending\u0000\u0000https://b", agents: [] },
    ]);
  });

  it("leaves the local daemon out when there is none", () => {
    expect(buildCreateHosts([host("a")], [], false)).toEqual([
      { id: "a", label: "label-a", revision: "reachable\u0000\u0000", agents: [] },
    ]);
  });

  it("names the agents a host already has, for a unique suggested name", () => {
    const agents = [{ name: "app-basic" }] as HostAgents["agents"];
    expect(buildCreateHosts([host("a", { agents })], [], false)[0].agents).toEqual(["app-basic"]);
  });
});

describe("buildWorkspaceHosts", () => {
  it("is ready only when reachable and connected", () => {
    const created = buildCreateHosts([host("a"), host("b", { error: "x" })], [], false);
    expect(
      buildWorkspaceHosts(
        [...created, { id: "c", label: "c", revision: "", agents: [] }],
        [host("a"), host("b", { error: "x" })],
        [daemon("a", { state: "connecting" })],
      ),
    ).toEqual([
      { id: "a", label: "label-a", ready: false },
      { id: "b", label: "label-b", ready: false },
      { id: "c", label: "c", ready: false },
    ]);
    expect(buildWorkspaceHosts(created.slice(0, 1), [host("a")], [])[0].ready).toBe(true);
  });
});

describe("withLastSuccessful", () => {
  it("shows a failing host's last good agents with the current error", () => {
    const good = host("a", { agents: [{ name: "x" } as HostAgents["agents"][number]] });
    const result = withLastSuccessful(
      [host("a", { error: "down" }), host("b")],
      new Map([["a", good]]),
    );
    expect(result[0]).toEqual({ ...good, error: "down" });
    expect(result[1]).toEqual(host("b"));
  });
});

describe("routeHost", () => {
  it("prefers the live host, then the last good snapshot, then the failing live host", () => {
    const live = host("a");
    const failing = host("a", { error: "down" });
    const snapshot = host("a", { host: { id: "a", label: "old" } });
    expect(routeHost(undefined, [live], new Map())).toEqual({});
    expect(routeHost("a", [live], new Map([["a", snapshot]])).selectedHost).toBe(live);
    expect(routeHost("a", [failing], new Map([["a", snapshot]]))).toEqual({
      liveSelectedHost: failing,
      selectedHost: snapshot,
    });
    expect(routeHost("a", [failing], new Map()).selectedHost).toBe(failing);
  });
});

describe("server order", () => {
  it("reads only a version 1 list of ids", () => {
    expect(parseServerOrder({ version: 1, ids: ["a", 2, "b"] })).toEqual(["a", "b"]);
    expect(parseServerOrder({ version: 2, ids: ["a"] })).toEqual([]);
    expect(parseServerOrder(null)).toEqual([]);
  });

  it("puts the visible hosts back into the slots they held", () => {
    expect(mergeVisibleServerOrder(["a", "x", "b", "c"], ["c", "a", "b"])).toEqual([
      "c",
      "x",
      "a",
      "b",
    ]);
  });

  it("ranks hosts by the saved order and applies pending sidebar orders", () => {
    const order = { version: 1 as const, groups: ["g"], agents: [] as string[] };
    const ranked = rankHosts(
      [host("a"), host("b"), host("c")],
      ["c", "a"],
      new Map([["a", order]]),
    );
    expect(ranked.map((entry) => entry.host.id)).toEqual(["c", "a", "b"]);
    expect(ranked[1].sidebarOrder).toBe(order);
  });
});

describe("paths and labels", () => {
  it("builds an agent path with an optional query", () => {
    expect(agentPath("", "a b", "chat")).toBe("/agents/local/a%20b/chat");
    expect(agentPath("prod", "x", "tasks", "?view=all")).toBe("/agents/prod/x/tasks?view=all");
  });

  it("names the local daemon and otherwise falls back to the id", () => {
    expect(fallbackHostLabel("")).toBe("This daemon (local)");
    expect(fallbackHostLabel("prod")).toBe("prod");
  });

  it("labels the route host from the aggregate, then the registry, then its id", () => {
    expect(hostLabelFor("a", host("a"), [daemon("a")])).toBe("label-a");
    expect(hostLabelFor("a", undefined, [daemon("a")])).toBe("daemon-a");
    expect(hostLabelFor("", undefined, [])).toBe("This daemon (local)");
  });
});
