import { it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { AgentNameContext } from "@/lib/agent";
import AgentContext from "./AgentContext";

afterEach(() => vi.restoreAllMocks());

function stubFetch(calls: Array<{ path: string; method?: string; body: unknown }>) {
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string, init?: RequestInit) => {
    if (init?.method) calls.push({ path, method: init.method, body: init?.body ? JSON.parse(init.body as string) : undefined });
    let result: unknown = {};
    if (path.endsWith("/context")) result = { name: "alpha", context: "some context" };
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result }) } as Response);
  }));
}

it("loads and saves the context panel", async () => {
  const calls: Array<{ path: string; method?: string; body: unknown }> = [];
  stubFetch(calls);
  render(<AgentNameContext.Provider value="alpha"><AgentContext /></AgentNameContext.Provider>);

  await waitFor(() => expect(screen.getByDisplayValue("some context")).toBeInTheDocument());
  fireEvent.change(screen.getByDisplayValue("some context"), { target: { value: "updated context" } });
  fireEvent.click(screen.getByText("Save context"));
  await waitFor(() =>
    expect(calls.some((c) => c.path === "/api/agents/alpha/context" && c.method === "POST" &&
      (c.body as { text?: string })?.text === "updated context")).toBe(true),
  );
});
