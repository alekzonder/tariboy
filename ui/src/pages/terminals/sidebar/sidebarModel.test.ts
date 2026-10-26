import { describe, expect, it } from "vitest";
import type { HostAgents } from "@/lib/aggregate";
import {
  agentKey,
  allAgents,
  filterHosts,
  groupSections,
  groupTeams,
  hostAgents,
  rankAgents,
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

  it("puts agents with an unread question above the rest and pinned above both", () => {
    const rows = allAgents(hosts);
    const { pinned, rest } = rankAgents(
      rows,
      new Set([agentKey("prod", "delta")]),
      new Map([[agentKey("prod", "gamma"), 1]]),
    );
    expect(pinned.map((row) => row.agent.name)).toEqual(["delta"]);
    expect(rest.map((row) => row.agent.name)).toEqual(["gamma", "beta", "alpha"]);
  });

  it("keeps the saved order inside a band", () => {
    const { rest } = rankAgents(allAgents(hosts), new Set(), new Map());
    expect(rest.map((row) => row.agent.name)).toEqual(["beta", "alpha", "gamma", "delta"]);
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
