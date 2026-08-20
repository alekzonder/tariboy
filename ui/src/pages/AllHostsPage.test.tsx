import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import AllHostsPage from "./AllHostsPage";
import { addDaemon } from "@/lib/daemons";

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});
afterEach(() => vi.restoreAllMocks());

describe("AllHostsPage", () => {
  it("renders agents grouped by host and degrades a failed host", async () => {
    await addDaemon({ label: "prod", baseURL: "https://prod:8765", token: "tp" });
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation((url: string) => {
        const localAgents = url.startsWith("/api/agents");
        const localGroups = url.startsWith("/api/groups");
        const local = localAgents || localGroups;
        return Promise.resolve({
          ok: local,
          status: local ? 200 : 401,
          text: async () =>
            JSON.stringify(
              local
                ? { ok: true, result: localAgents ? { agents: [{ name: "local-a", image: "i", state: "running", harness: "claude", loop_enabled: true, group: null }], count: 1 } : { groups: [], count: 0 } }
                : { ok: false, error: { code: "unauthorized", message: "nope" } },
            ),
        } as unknown as Response);
      }),
    );
    render(
      <MemoryRouter>
        <AllHostsPage />
      </MemoryRouter>,
    );
    await waitFor(() => expect(screen.getByText("local-a")).toBeInTheDocument());
    expect(screen.getByText("prod")).toBeInTheDocument();
    // The failed host surfaces an error but the page still shows the local agent.
    await waitFor(() => expect(screen.getByText(/unauthorized|unreachable/i)).toBeInTheDocument());
  });
});
