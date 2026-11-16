import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { ApiProvider } from "@/components/ApiProvider";
import { agentRow } from "@/fixtures/agents";
import { RunningAgentsContext, runningAgentsByHost, useAgentRunning } from "./runningAgents";

const remote = { id: "remote-1", label: "Remote", baseURL: "http://127.0.0.1:1", token: "t" };

describe("runningAgentsByHost", () => {
  it("keeps only the running agents of each host", () => {
    const map = runningAgentsByHost([
      {
        host: { id: "", label: "Local" },
        agents: [
          agentRow({ name: "builder", state: "running" }),
          agentRow({ name: "idle", state: "stopped" }),
          agentRow({ name: "broken", state: "error" }),
        ],
      },
      { host: { id: "remote-1", label: "Remote" }, agents: [], error: "offline" },
    ]);
    expect([...(map.get("") ?? [])]).toEqual(["builder"]);
    expect(map.get("remote-1")?.size).toBe(0);
  });
});

function wrap(running: Map<string, Set<string>>, target: typeof remote | null) {
  return ({ children }: { children: ReactNode }) => (
    <ApiProvider target={target}>
      <RunningAgentsContext.Provider value={running}>{children}</RunningAgentsContext.Provider>
    </ApiProvider>
  );
}

describe("useAgentRunning", () => {
  const running = new Map([
    ["", new Set(["builder"])],
    ["remote-1", new Set(["reviewer"])],
  ]);

  it("is true for a running agent of the provider's host", () => {
    const { result } = renderHook(() => useAgentRunning("agent:builder"), {
      wrapper: wrap(running, null),
    });
    expect(result.current).toBe(true);
  });

  it("is false for an agent that runs on another host", () => {
    const { result } = renderHook(() => useAgentRunning("agent:builder"), {
      wrapper: wrap(running, remote),
    });
    expect(result.current).toBe(false);
  });

  it("is false for a person and without any provider", () => {
    expect(
      renderHook(() => useAgentRunning("user:builder"), { wrapper: wrap(running, null) }).result
        .current,
    ).toBe(false);
    expect(renderHook(() => useAgentRunning("agent:builder")).result.current).toBe(false);
  });
});
