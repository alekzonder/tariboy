import { it, expect, vi, afterEach } from "vitest";
import { render, screen, waitFor } from "@/test/queryRender";
import { MemoryRouter } from "react-router-dom";
import Home from "./Home";

afterEach(() => vi.restoreAllMocks());

it("renders agent cards from the API", async () => {
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string) => {
    const body = path.includes("/agents")
      ? { ok: true, result: { agents: [{ name: "alpha", image: "img:1", state: "running", harness: "claude", group: null }], count: 1 } }
      : { ok: true, result: { version: "1.0", pid: 5, started_at: "", uptime_seconds: 0, base_dir: "", schema_version: 1 } };
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify(body) } as Response);
  }));
  render(<MemoryRouter><Home /></MemoryRouter>);
  await waitFor(() => expect(screen.getByText("alpha")).toBeInTheDocument());
  expect(screen.getByText("running")).toBeInTheDocument();
  expect(screen.queryByText("loop")).not.toBeInTheDocument();
});
