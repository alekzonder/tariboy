import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@/test/queryRender";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import AgentAdvancedTab from "./AgentAdvancedTab";
import { AgentNameContext } from "@/lib/agent";

// Each view owns its own behaviour and tests; Advanced only routes between them.
vi.mock("@/pages/AgentPrompt", () => ({ default: () => <div data-testid="prompt-view" /> }));

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      text: async () => JSON.stringify({ ok: true, result: {} }),
    } as unknown as Response),
  );
});
afterEach(() => vi.unstubAllGlobals());

function renderAdvanced(path: string) {
  render(
    <MemoryRouter initialEntries={[path]}>
      <AgentNameContext.Provider value="worker">
        <Routes>
          <Route
            path="/agents/:hostId/:agent/advanced/*"
            element={<AgentAdvancedTab base={path.split("/advanced")[0]} />}
          />
          <Route path="/agents/:hostId/:agent/mailbox" element={<div>Mailbox</div>} />
        </Routes>
      </AgentNameContext.Provider>
    </MemoryRouter>,
  );
}

it("opens on Prompt and no longer lists Channels or Messages", async () => {
  renderAdvanced("/agents/local/worker/advanced");

  expect(await screen.findByTestId("prompt-view")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Channels" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Messages" })).not.toBeInTheDocument();
  expect(screen.getByTestId("tab-page")).toContainElement(
    screen.getByRole("button", { name: "Prompt" }),
  );
});

it("opens an old Channels or Messages link on the default view", async () => {
  renderAdvanced("/agents/local/worker/advanced?view=channels");

  expect(await screen.findByTestId("prompt-view")).toBeInTheDocument();
});

it("links Diagnostics to the same agent's Mailbox and Work without writes", async () => {
  renderAdvanced("/agents/remote/worker/advanced");

  expect(await screen.findByRole("heading", { name: "Diagnostics" })).toBeVisible();
  expect(screen.getAllByRole("button").map((button) => button.textContent)).toEqual([
    "Prompt",
    "Context",
    "Files",
    "Scripts",
    "Full audit",
    "Raw usage",
    "Raw settings",
  ]);
  expect(screen.getByRole("link", { name: "Mailbox" })).toHaveAttribute(
    "href",
    "/agents/remote/worker/mailbox",
  );
  expect(screen.getByRole("link", { name: "Back to Work" })).toHaveAttribute(
    "href",
    "/agents/remote/worker/work",
  );
  fireEvent.click(screen.getByRole("button", { name: "Scripts" }));
  fireEvent.click(screen.getByRole("link", { name: "Mailbox" }));
  expect(
    vi.mocked(fetch).mock.calls.every(([, init]) => !init?.method || init.method === "GET"),
  ).toBe(true);
});
