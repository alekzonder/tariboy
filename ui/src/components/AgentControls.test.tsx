import { it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { AgentControls } from "./AgentControls";

afterEach(() => vi.restoreAllMocks());

it("posts a lifecycle action and can exec with a prompt", async () => {
  const calls: Array<{ method: string; path: string; body: unknown }> = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string, init?: RequestInit) => {
    calls.push({ method: init?.method ?? "GET", path, body: init?.body ? JSON.parse(init.body as string) : undefined });
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result: {} }) } as Response);
  }));
  render(<AgentControls name="alpha" />);

  fireEvent.click(screen.getByText("Start"));
  await waitFor(() => expect(calls.some((c) => c.path === "/api/agents/alpha/start" && c.method === "POST")).toBe(true));

  fireEvent.change(screen.getByPlaceholderText(/one-shot exec/i), { target: { value: "go" } });
  fireEvent.click(screen.getByText("Exec"));
  await waitFor(() =>
    expect(calls.some((c) => c.path === "/api/agents/alpha/exec" && (c.body as { prompt?: string })?.prompt === "go")).toBe(true),
  );
});
