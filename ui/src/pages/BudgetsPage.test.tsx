import { it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import BudgetsPage from "./BudgetsPage";

afterEach(() => vi.restoreAllMocks());

it("shows status rows and posts a new budget with the limit-usd key", async () => {
  const posts: Array<{ path: string; body: Record<string, unknown> }> = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string, init?: RequestInit) => {
    if (init?.method === "POST") posts.push({ path, body: JSON.parse(init.body as string) });
    const body = path.includes("/status")
      ? { ok: true, result: { budgets: [{ scope: "global", limit_usd: 10, spent_usd: 3.5, mode: "warn", over: false }], count: 1 } }
      : { ok: true, result: {} };
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify(body) } as Response);
  }));
  render(<BudgetsPage />);

  await waitFor(() => expect(screen.getByText("global")).toBeInTheDocument());
  fireEvent.change(screen.getByPlaceholderText("limit USD"), { target: { value: "25" } });
  fireEvent.click(screen.getByText("Save"));
  await waitFor(() => expect(posts.some((p) => p.path === "/api/budgets" && p.body["limit-usd"] === "25")).toBe(true));
});
