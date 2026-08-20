import { beforeEach, describe, expect, it } from "vitest";
import {
  MAX_WORKSPACE_STATE_BYTES,
  WORKSPACE_STATE_KEY,
  emptyWorkspaceState,
  readWorkspaceState,
  sanitizeWorkspaceState,
  terminalKey,
  writeWorkspaceState,
  type TerminalIdentity,
  type TerminalWorkspaceStateV1,
} from "./workspaceState";

const localAgent: TerminalIdentity = { hostId: "", agentName: "alpha" };
const remoteAgent: TerminalIdentity = { hostId: "remote-a", agentName: "worker" };

function stateWith(...identities: TerminalIdentity[]): TerminalWorkspaceStateV1 {
  return {
    schemaVersion: 1,
    layout: {
      global: {},
      borders: [],
      layout: {
        type: "row",
        weight: 100,
        children: identities.map((identity, index) => ({
          type: "tabset",
          id: `set-${index}`,
          weight: 50,
          selected: 0,
          children: [{
            type: "tab",
            id: `terminal-${index}`,
            name: identity.agentName,
            component: "agent-terminal",
            config: { ...identity },
          }],
        })),
      },
    },
    activeTerminal: identities[0] ? terminalKey(identities[0]) : null,
    sidebar: { width: 380, hidden: true },
  };
}

beforeEach(() => {
  localStorage.clear();
});

describe("terminal workspace persistence", () => {
  it("round-trips a valid identity-only workspace", () => {
    const state = stateWith(localAgent, remoteAgent);

    writeWorkspaceState(state);

    expect(readWorkspaceState()).toEqual(state);
    const persisted = localStorage.getItem(WORKSPACE_STATE_KEY)!;
    expect(persisted).toContain("remote-a");
    expect(persisted).toContain("worker");
    expect(persisted).not.toContain("OPENAI_API_KEY");
    expect(persisted).not.toContain("/secret/workdir");
  });

  it("uses the legacy sidebar width only when no workspace exists", () => {
    localStorage.setItem("terminals:sidebarWidth", "444");

    expect(readWorkspaceState()).toEqual({
      ...emptyWorkspaceState(),
      sidebar: { width: 444, hidden: false },
    });
  });

  it("clamps sidebar width when sanitizing", () => {
    const state = stateWith(localAgent);
    state.sidebar.width = 9000;

    expect(sanitizeWorkspaceState(state)?.sidebar.width).toBe(640);
  });

  it.each([
    ["unknown schema", { ...stateWith(localAgent), schemaVersion: 2 }],
    ["missing root layout", { ...stateWith(localAgent), layout: { global: {}, borders: [] } }],
    ["invalid node type", {
      ...stateWith(localAgent),
      layout: { global: {}, borders: [], layout: { type: "floating", children: [] } },
    }],
    ["missing host identity", (() => {
      const state = stateWith(localAgent);
      const tab = ((state.layout.layout as { children: Array<{ children: unknown[] }> }).children[0].children[0]) as {
        config: Record<string, unknown>
      };
      delete tab.config.hostId;
      return state;
    })()],
    ["wrong terminal component", (() => {
      const state = stateWith(localAgent);
      const tab = ((state.layout.layout as { children: Array<{ children: unknown[] }> }).children[0].children[0]) as {
        component: string
      };
      tab.component = "settings";
      return state;
    })()],
    ["duplicate terminal identity", stateWith(localAgent, localAgent)],
    ["duplicate layout node id", (() => {
      const state = stateWith(localAgent, remoteAgent);
      const children = (state.layout.layout as {
        children: Array<{ id: string }>
      }).children;
      children[1].id = children[0].id;
      return state;
    })()],
    ["terminal directly below row", (() => {
      const state = stateWith(localAgent);
      const root = state.layout.layout as {
        children: Array<{ children: unknown[] }>
      };
      root.children = root.children[0].children as Array<{ children: unknown[] }>;
      return state;
    })()],
    ["empty nested row", (() => {
      const state = stateWith(localAgent);
      (state.layout.layout as { children: unknown[] }).children = [{
        type: "row",
        id: "empty-row",
        children: [],
      }];
      return state;
    })()],
    ["non-finite weight", (() => {
      const state = stateWith(localAgent);
      (state.layout.layout as { weight: number }).weight = Number.POSITIVE_INFINITY;
      return state;
    })()],
    ["negative weight", (() => {
      const state = stateWith(localAgent);
      (state.layout.layout as { weight: number }).weight = -1;
      return state;
    })()],
    ["extreme weight", (() => {
      const state = stateWith(localAgent);
      (state.layout.layout as { weight: number }).weight = 1_000_001;
      return state;
    })()],
  ])("rejects %s", (_name, candidate) => {
    expect(sanitizeWorkspaceState(candidate)).toBeNull();
  });

  it("falls back to an empty workspace for malformed JSON", () => {
    localStorage.setItem(WORKSPACE_STATE_KEY, "{");

    expect(readWorkspaceState()).toEqual(emptyWorkspaceState());
  });

  it("falls back to an empty workspace for oversized JSON", () => {
    const state = stateWith(localAgent);
    localStorage.setItem(
      WORKSPACE_STATE_KEY,
      `${JSON.stringify(state)}${" ".repeat(MAX_WORKSPACE_STATE_BYTES)}`,
    );

    expect(readWorkspaceState()).toEqual(emptyWorkspaceState());
  });

  it("measures the storage limit in UTF-8 bytes rather than JavaScript characters", () => {
    const identities = Array.from({ length: 180 }, (_, index) => ({
      hostId: `remote-${index}`,
      agentName: `${index}-${"😀".repeat(300)}`,
    }));
    const state = stateWith(...identities);
    const serialized = JSON.stringify(state);
    expect(serialized.length).toBeLessThan(MAX_WORKSPACE_STATE_BYTES);
    expect(new TextEncoder().encode(serialized).byteLength).toBeGreaterThan(
      MAX_WORKSPACE_STATE_BYTES,
    );

    localStorage.setItem(WORKSPACE_STATE_KEY, serialized);

    expect(readWorkspaceState()).toEqual(emptyWorkspaceState());
  });

  it("encodes host and agent without delimiter collisions", () => {
    expect(terminalKey({ hostId: "a\u0000b", agentName: "c" }))
      .not.toBe(terminalKey({ hostId: "a", agentName: "b\u0000c" }));
  });
});
