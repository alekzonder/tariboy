import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AgentNameContext } from "@/lib/agent";
import { AliasEditor } from "./AliasEditor";

beforeEach(() => {
  vi.stubGlobal("fetch", vi.fn().mockImplementation((_url: string, init?: RequestInit) => {
    const isPost = init?.method === "POST";
    const body = isPost
      ? { name: "foo", alias: JSON.parse(init!.body as string).value }
      : { name: "foo", alias: "" };
    return Promise.resolve({
      ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result: body }),
    } as Response);
  }));
});
afterEach(() => vi.restoreAllMocks());

describe("AliasEditor", () => {
  it("shows the agent name and lets you add an alias", async () => {
    render(<AgentNameContext.Provider value="foo"><AliasEditor /></AgentNameContext.Provider>);
    await waitFor(() => expect(screen.getByText(/Agent:/)).toBeInTheDocument());
    await userEvent.click(screen.getByText("+ alias"));
    const input = screen.getByPlaceholderText("alias");
    await userEvent.type(input, "Nice");
    await userEvent.click(screen.getByText("Save"));
    await waitFor(() => expect(screen.getByText("Nice")).toBeInTheDocument());
  });
});
