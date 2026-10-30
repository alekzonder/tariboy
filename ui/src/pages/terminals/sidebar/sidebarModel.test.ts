import { describe, expect, it } from "vitest";
import type { HostAgents } from "@/lib/aggregate";
import {
  agentKey,
  allAgents,
  filterHosts,
  groupSections,
  groupTeams,
  hostAgents,
  hostsLine,
  sidebarLayout,
} from "./sidebarModel";
import type { AgentRow } from "@/lib/api";
import { agentRow } from "@/test/fixtures/apiFixtures";

const agent = (name: string, group = ""): AgentRow =>
  agentRow({ name, image: "img:latest", state: "running", harness: "claude", group });

const hosts: HostAgents[] = [
  {
    host: { id: "", label: "This daemon (local)" },
    agents: [agent("alpha", "release"), agent("beta")],
    groups: [{ name: "release", lead: "alpha", members: 1 }],
    sidebarOrder: { version: 1, groups: [], agents: ["beta", "alpha"] },
  },
  {
    host: { id: "prod", label: "prod" },
    agents: [agent("gamma", "release"), agent("delta")],
    groups: [{ name: "idle-team", lead: "", members: 0 }],
  },
];

describe("sidebar agent list", () => {
  it("lists agents from every server, each server in its saved order", () => {
    expect(allAgents(hosts).map((row) => `${row.hostId}/${row.agent.name}`)).toEqual([
      "/beta",
      "/alpha",
      "prod/gamma",
      "prod/delta",
    ]);
  });
});

describe("sidebar layout", () => {
  const names = (rows: Array<{ agent: { name: string } }>) => rows.map((row) => row.agent.name);

  it("shows one host's teams and agents without a host heading", () => {
    const layout = sidebarLayout(hosts.slice(0, 1), new Set());
    expect(layout.byHost).toBe(false);
    expect(layout.pinned).toEqual([]);
    expect(layout.sections).toHaveLength(1);
    expect([...layout.sections[0].teams.keys()]).toEqual(["release"]);
    expect(names(layout.sections[0].individuals)).toEqual(["beta"]);
  });

  it("groups two hosts under their headings and shows a pinned agent only on top", () => {
    const layout = sidebarLayout(
      hosts,
      new Set([agentKey("prod", "delta"), agentKey("", "alpha")]),
    );
    expect(layout.byHost).toBe(true);
    expect(names(layout.pinned)).toEqual(["alpha", "delta"]);
    expect(layout.sections.map((section) => section.host.host.id)).toEqual(["", "prod"]);
    expect(names(layout.sections[0].teams.get("release") ?? [])).toEqual([]);
    expect(names(layout.sections[1].individuals)).toEqual([]);
    expect(names(layout.sections[1].teams.get("release") ?? [])).toEqual(["gamma"]);
    expect([...layout.sections[1].teams.keys()]).toEqual(["idle-team", "release"]);
  });
});

describe("hosts line", () => {
  it.each([
    [[], "No hosts"],
    [[hosts[0]], "Local · ready"],
    [[{ ...hosts[1], error: "down" }], "prod · unreachable"],
    [hosts, "2 hosts · ready"],
    [[hosts[0], { ...hosts[1], error: "down" }], "2 hosts · 1 unreachable"],
  ] as Array<[HostAgents[], string]>)("reads %#", (list, text) => {
    expect(hostsLine(list)).toBe(text);
  });
});

describe("sidebar group sections", () => {
  it("collects a group across servers and keeps a declared empty group", () => {
    const sections = groupSections(hosts, allAgents(hosts));
    expect(sections.map((section) => section.name)).toEqual(["idle-team", "release"]);
    expect(sections[1].agents.map((row) => `${row.hostId}/${row.agent.name}`)).toEqual([
      "/alpha",
      "prod/gamma",
    ]);
    expect(sections[0].agents).toEqual([]);
  });
});

describe("server teams", () => {
  it("lists declared and used teams in the saved order, members in their own order", () => {
    const host: HostAgents = {
      ...hosts[0],
      agents: [agent("alpha", "release"), agent("beta"), agent("omega", "ops")],
      sidebarOrder: { version: 1, groups: ["ops"], agents: [] },
    };
    const teams = groupTeams(host, hostAgents(host));
    expect([...teams.keys()]).toEqual(["ops", "release"]);
    expect(teams.get("release")?.map((row) => row.agent.name)).toEqual(["alpha"]);
    expect(teams.get("ops")?.map((row) => row.agent.name)).toEqual(["omega"]);
  });

  it("keeps a declared team with no members", () => {
    const teams = groupTeams(hosts[1], hostAgents(hosts[1]));
    expect([...teams.entries()].map(([name, rows]) => [name, rows.length])).toEqual([
      ["idle-team", 0],
      ["release", 1],
    ]);
  });
});

describe("sidebar search", () => {
  it("keeps matching agents, and a server whose label matches keeps its own", () => {
    expect(filterHosts(hosts, "amm").map((host) => host.agents.map((a) => a.name))).toEqual([
      ["gamma"],
    ]);
    expect(filterHosts(hosts, "prod").map((host) => host.host.id)).toEqual(["prod"]);
    expect(filterHosts(hosts, "  ")).toEqual(hosts);
  });
});
