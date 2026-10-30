import { beforeEach, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import type { Task } from "@/lib/api";
import type { HostNeeds } from "@/lib/needsYou";
import { agentRow } from "@/test/fixtures/apiFixtures";
import { NeedsYouContext } from "../needsYouContext";
import { AgentSidebar } from "./AgentSidebar";

beforeEach(() => localStorage.clear());

const task = (over: Partial<Task>): Task =>
  ({ key: "T-1", updated_at: "", assignee: "agent:bob", ...over }) as Task;

function renderSidebar(needs: HostNeeds[]) {
  const actions = {
    onSelect: vi.fn(),
    onReorder: vi.fn(),
    onClone: vi.fn(),
    onCreate: vi.fn(),
    onSelectHost: vi.fn(),
    onSelectTeam: vi.fn(),
    onEditServer: vi.fn(),
    onRemoveServer: vi.fn(),
    onConnectHost: vi.fn(),
  };
  render(
    <MemoryRouter>
      <NeedsYouContext.Provider value={{ hosts: needs, loaded: true, reload: () => {} }}>
        <AgentSidebar
          hosts={[
            {
              host: { id: "", label: "Local" },
              agents: [
                agentRow({ name: "bob", interactive: true, state: "idle" }),
                agentRow({ name: "carol", interactive: true, state: "running" }),
                agentRow({ name: "dave", interactive: true, state: "idle" }),
                agentRow({ name: "erin", interactive: true, state: "stopped", enabled: false }),
              ],
            },
          ]}
          pageHostId=""
          counts={{ tasks: 1, inbox: 0 }}
          query=""
          onQuery={() => {}}
          daemonViews={[]}
          appVersion=""
          actions={actions}
          width={260}
          onResize={() => {}}
        />
      </NeedsYouContext.Provider>
    </MemoryRouter>,
  );
}

it("shows a pill only on agents that need the user, work or are off", () => {
  renderSidebar([
    {
      hostId: "",
      hostLabel: "Local",
      tasks: [
        task({ key: "A-1", waiting_on: "customer", status: "approval" }),
        task({
          key: "A-2",
          waiting_on: "customer",
          workflow_initial: true,
          assignee: "agent:dave",
        }),
      ],
      letters: [],
      open: 0,
    },
  ]);
  expect(screen.getByRole("button", { name: "Open bob" })).toHaveTextContent("bobneeds you · 1");
  expect(screen.getByRole("button", { name: "Open carol" })).toHaveTextContent("carolworking");
  expect(screen.getByRole("button", { name: "Open dave" })).toHaveTextContent(/^dave$/);
  expect(screen.getByRole("button", { name: "Open erin" })).toHaveTextContent("erinoff");
});
