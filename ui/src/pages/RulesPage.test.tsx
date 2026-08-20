import { it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import RulesPage from "./RulesPage";

afterEach(() => vi.restoreAllMocks());

it("lists rules and removes one by id via DELETE", async () => {
  const dels: string[] = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string, init?: RequestInit) => {
    if (init?.method === "DELETE") dels.push(path);
    const body = path === "/api/proxy-rules" && (!init || init.method === "GET" || init.method === undefined)
      ? { ok: true, result: { rules: [{ id: "r1", priority: 0, scope: "global", model_glob: "", kind: "rate-limit", max_requests: 5, max_tokens: 0, window_s: 60, allow: [], deny: [], route: "", enabled: true }], count: 1 } }
      : { ok: true, result: {} };
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify(body) } as Response);
  }));
  render(<RulesPage />);

  await waitFor(() => expect(screen.getByText("global")).toBeInTheDocument());
  fireEvent.click(screen.getByText("Remove"));
  await waitFor(() => expect(dels).toContain("/api/proxy-rules/r1"));
});
