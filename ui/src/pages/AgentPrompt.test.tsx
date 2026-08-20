import { it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { AgentNameContext } from "@/lib/agent";
import AgentPrompt from "./AgentPrompt";

afterEach(() => vi.restoreAllMocks());

function stubFetch(
  calls: Array<{ path: string; method?: string; body: unknown }>,
  promptResult: unknown = {
    name: "alpha",
    prompt: "assembled prompt text",
    layers: [{ name: "system", sha256: "abc123def456" }],
  },
) {
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string, init?: RequestInit) => {
    if (init?.method) calls.push({ path, method: init.method, body: init?.body ? JSON.parse(init.body as string) : undefined });
    let result: unknown = {};
    if (path.endsWith("/prompt")) result = promptResult;
    else if (path.endsWith("/user-prompt")) result = { name: "alpha", user_prompt: "hi" };
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result }) } as Response);
  }));
}

it("renders the assembled prompt preview with layers", async () => {
  const calls: Array<{ path: string; method?: string; body: unknown }> = [];
  stubFetch(calls);
  render(<AgentNameContext.Provider value="alpha"><AgentPrompt /></AgentNameContext.Provider>);

  await waitFor(() => expect(screen.getByDisplayValue("assembled prompt text")).toBeInTheDocument());
  expect(screen.getByDisplayValue("assembled prompt text")).toHaveAttribute("readonly");
  expect(screen.getByText(/system.*abc123def456/)).toBeInTheDocument();
});

it("renders schema v2 file layers and runtime placeholders in declared order", async () => {
  const calls: Array<{ path: string; method?: string; body: unknown }> = [];
  stubFetch(calls, {
    name: "alpha",
    prompt: "assembled schema v2 prompt",
    layers: [
      {
        kind: "file",
        source: "$STORE/skills/whoami/prompt.md",
        archive_path: "prompt/layers/000-prompt.md",
        sha256: "abcdef0123456789",
      },
      { kind: "runtime", runtime: "identity" },
    ],
  });
  render(<AgentNameContext.Provider value="alpha"><AgentPrompt /></AgentNameContext.Provider>);

  await waitFor(() => expect(screen.getByDisplayValue("assembled schema v2 prompt")).toBeInTheDocument());
  const rows = screen.getAllByRole("listitem");
  expect(rows).toHaveLength(2);
  expect(within(rows[0]).getByText(/\$STORE\/skills\/whoami\/prompt\.md.*abcdef012345/)).toBeInTheDocument();
  expect(within(rows[1]).getByText(/runtime.*identity/i)).toBeInTheDocument();
});

it("saves the user prompt via POST /user-prompt", async () => {
  const calls: Array<{ path: string; method?: string; body: unknown }> = [];
  stubFetch(calls);
  render(<AgentNameContext.Provider value="alpha"><AgentPrompt /></AgentNameContext.Provider>);

  await waitFor(() => expect(screen.getByDisplayValue("hi")).toBeInTheDocument());
  fireEvent.change(screen.getByDisplayValue("hi"), { target: { value: "new prompt" } });
  fireEvent.click(screen.getByText("Save prompt"));
  await waitFor(() =>
    expect(calls.some((c) => c.path === "/api/agents/alpha/user-prompt" && c.method === "POST" &&
      (c.body as { text?: string })?.text === "new prompt")).toBe(true),
  );
});

it("clears the user prompt via the Clear button", async () => {
  const calls: Array<{ path: string; method?: string; body: unknown }> = [];
  stubFetch(calls);
  render(<AgentNameContext.Provider value="alpha"><AgentPrompt /></AgentNameContext.Provider>);

  await waitFor(() => expect(screen.getByDisplayValue("hi")).toBeInTheDocument());
  fireEvent.click(screen.getByText("Clear"));
  await waitFor(() =>
    expect(calls.some((c) => c.path === "/api/agents/alpha/user-prompt" && c.method === "POST" &&
      (c.body as { text?: string })?.text === "")).toBe(true),
  );
});
