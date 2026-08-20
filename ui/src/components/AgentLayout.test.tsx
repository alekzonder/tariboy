import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { AgentLayout } from "./AgentLayout";

function stubFetch(opts: {
  agents?: Array<{ name: string; state?: string; group?: string | null; alias?: string }>;
} = {}) {
  const agents = (opts.agents ?? []).map((a) => ({ state: "running", group: null, ...a }));
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((url: string) => {
      const path = String(url);
      let result: unknown = { agents: [], count: 0 };
      if (path.includes("/api/agents")) {
        result = { agents, count: agents.length };
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        text: async () => JSON.stringify({ ok: true, result }),
      } as Response);
    }),
  );
}

beforeEach(() => {
  stubFetch();
});
afterEach(() => vi.restoreAllMocks());

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/agent/:name/*" element={<AgentLayout />}>
          <Route index element={<div>overview body</div>} />
          <Route path="settings" element={<div>settings body</div>} />
        </Route>
      </Routes>
    </MemoryRouter>,
  );
}

describe("AgentLayout", () => {
  it("renders the unified header (alias + status) above the tabs on the Overview route", () => {
    renderAt("/agent/foo");
    expect(screen.getByText("overview body")).toBeInTheDocument();
    // The unified header now renders on Overview too: AliasEditor's 'Agent:'
    // heading plus the live loop status. The old name+badge header is gone.
    expect(screen.getByText(/Agent:/)).toBeInTheDocument();
    expect(screen.getByText(/status:/)).toBeInTheDocument();
    expect(screen.queryByText("agent")).not.toBeInTheDocument();
  });

  it("renders the same unified header above the tabs on non-Overview routes", () => {
    renderAt("/agent/foo/settings");
    expect(screen.getByText("settings body")).toBeInTheDocument();
    expect(screen.getByText(/Agent:/)).toBeInTheDocument();
    expect(screen.getByText(/status:/)).toBeInTheDocument();
    // foo renders as the agent name inside the AliasEditor heading.
    expect(screen.getByText("foo")).toBeInTheDocument();
  });

  it("keeps the tab nav visible on the Overview route", () => {
    renderAt("/agent/foo");
    expect(screen.getByText("Settings")).toBeInTheDocument();
  });

  it("renders a group header for each group and lists its agents under it", async () => {
    stubFetch({
      agents: [
        { name: "lead", group: "dev-team" },
        { name: "worker", group: "dev-team" },
        { name: "solo" },
      ],
    });
    // View the ungrouped agent so the grouped agents' names appear only in the
    // sidebar (the top header renders the viewed agent's name too).
    renderAt("/agent/solo/settings");
    // The group header shows the group label; both grouped agents render.
    expect(await screen.findByTestId("agent-group-dev-team")).toHaveTextContent("dev-team");
    expect(screen.getByText("lead")).toBeInTheDocument();
    expect(screen.getByText("worker")).toBeInTheDocument();
    // The ungrouped agent still renders (sidebar + header), with no header for it.
    expect(screen.getAllByText("solo").length).toBeGreaterThan(0);
    expect(screen.queryByTestId("agent-group-solo")).not.toBeInTheDocument();
  });

  it("sets the document title alias-first for an aliased agent", async () => {
    stubFetch({ agents: [{ name: "foo", alias: "Alpha" }] });
    renderAt("/agent/foo/settings");
    await waitFor(() => expect(document.title).toBe("Alpha (foo)"));
  });

  it("sets the document title to the bare name for a non-aliased agent", async () => {
    stubFetch({ agents: [{ name: "foo" }] });
    renderAt("/agent/foo/settings");
    await waitFor(() => expect(document.title).toBe("foo"));
  });

  it("restores the captured initial document title when the agent view unmounts", async () => {
    // jsdom starts document.title empty; seed the value index.html would so the
    // component captures it on mount and we can assert the reset restores THAT.
    document.title = "ya-agent";
    stubFetch({ agents: [{ name: "foo", alias: "Alpha" }] });
    const { unmount } = renderAt("/agent/foo/settings");
    await waitFor(() => expect(document.title).toBe("Alpha (foo)"));
    unmount();
    expect(document.title).toBe("ya-agent");
  });

  it("renders no group headers when every agent is ungrouped", async () => {
    stubFetch({ agents: [{ name: "foo" }, { name: "bar" }] });
    renderAt("/agent/foo/settings");
    await screen.findByText("foo");
    expect(screen.getByText("bar")).toBeInTheDocument();
    expect(screen.queryByTestId("agent-group-null")).not.toBeInTheDocument();
  });
});
