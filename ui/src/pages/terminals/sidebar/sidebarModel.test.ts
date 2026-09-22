import { describe, expect, it } from "vitest";
import type { HostAgents } from "@/lib/aggregate";
import type { AgentSummary } from "@/lib/types";
import { agentKey, allAgents, filterHosts, groupSections, rankAgents } from "./sidebarModel";
import type { ChatSummary } from "@/lib/api";

const chat = (agent: string, last_ts: string, unread = 0, kind = "direct"): ChatSummary => ({
  id: `${kind === "direct" ? "dm" : kind}:${agent}`, kind, title: agent,
  agent, last_ts, unread, last_from: `agent:${agent}`, last_type: "message", last_text: "...",
});

const agent = (name: string, group: string | null = null): AgentSummary => ({
  name,
  image: "img:latest",
  state: "running",
  harness: "claude",
  loop_enabled: false,
  group,
});

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
    expect(allAgents(hosts).map((row) => `${row.hostId}/${row.agent.name}`))
      .toEqual(["/beta", "/alpha", "prod/gamma", "prod/delta"]);
  });

  it("puts agents with an unread question above the rest and pinned above both", () => {
    const rows = allAgents(hosts);
    const { pinned, rest } = rankAgents(
      rows,
      new Set([agentKey("prod", "delta")]),
      new Set([agentKey("prod", "gamma")]),
    );
    expect(pinned.map((row) => row.agent.name)).toEqual(["delta"]);
    expect(rest.map((row) => row.agent.name)).toEqual(["gamma", "beta", "alpha"]);
  });

  it("keeps the saved order inside a band", () => {
    const { rest } = rankAgents(allAgents(hosts), new Set(), new Set());
    expect(rest.map((row) => row.agent.name)).toEqual(["beta", "alpha", "gamma", "delta"]);
  });

  it("orders the remaining agents like a chat list, newest conversation first", () => {
    const chatted: HostAgents[] = [
      { ...hosts[0], chats: [chat("alpha", "2026-09-15T10:00:00Z")] },
      { ...hosts[1], chats: [chat("gamma", "2026-09-15T12:00:00Z", 2)] },
    ];
    const rows = allAgents(chatted);
    expect(rows.find((row) => row.agent.name === "gamma")?.chat?.unread).toBe(2);
    const { rest } = rankAgents(rows, new Set(), new Set());
    // gamma spoke last, alpha before it, and the two silent agents keep the
    // operator's own order behind them.
    expect(rest.map((row) => row.agent.name)).toEqual(["gamma", "alpha", "beta", "delta"]);
  });

  it("folds an agent's three chats into one row: newest activity, all unread", () => {
    const chatted: HostAgents[] = [
      {
        ...hosts[0],
        chats: [
          chat("alpha", "2026-09-15T10:00:00Z", 1),
          chat("alpha", "2026-09-15T13:00:00Z", 2, "tasks"),
          chat("alpha", "2026-09-15T09:00:00Z", 0, "service"),
        ],
      },
      hosts[1],
    ];
    const row = allAgents(chatted).find((item) => item.agent.name === "alpha");
    expect(row?.chat?.unread).toBe(3);
    expect(row?.chat?.last_ts).toBe("2026-09-15T13:00:00Z");
  });

  it("keeps an agent calling for a person above a more recent conversation", () => {
    const chatted: HostAgents[] = [
      { ...hosts[0], chats: [chat("alpha", "2026-09-15T12:00:00Z")] },
      hosts[1],
    ];
    const { rest } = rankAgents(allAgents(chatted), new Set(), new Set([agentKey("prod", "gamma")]));
    expect(rest[0].agent.name).toBe("gamma");
    expect(rest[1].agent.name).toBe("alpha");
  });
});

describe("sidebar group sections", () => {
  it("collects a group across servers and keeps a declared empty group", () => {
    const sections = groupSections(hosts, allAgents(hosts));
    expect(sections.map((section) => section.name)).toEqual(["idle-team", "release"]);
    expect(sections[1].agents.map((row) => `${row.hostId}/${row.agent.name}`))
      .toEqual(["/alpha", "prod/gamma"]);
    expect(sections[0].agents).toEqual([]);
  });
});

describe("sidebar search", () => {
  it("keeps matching agents, and a server whose label matches keeps its own", () => {
    expect(filterHosts(hosts, "amm").map((host) => host.agents.map((a) => a.name)))
      .toEqual([["gamma"]]);
    expect(filterHosts(hosts, "prod").map((host) => host.host.id)).toEqual(["prod"]);
    expect(filterHosts(hosts, "  ")).toEqual(hosts);
  });
});
