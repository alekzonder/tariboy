import { beforeEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import type { Task } from "@/lib/api";
import type { HostNeeds } from "@/lib/needsYou";
import { agentRow } from "@/test/fixtures/apiFixtures";
import { NeedsYouContext } from "../needsYouContext";
import { AgentSidebar } from "./AgentSidebar";

beforeEach(() => localStorage.clear());

const task = (over: Partial<Task>): Task =>
  ({ key: "T-1", updated_at: "", assignee: "agent:bob", ...over }) as Task;

function renderSidebar(
  needs: HostNeeds[],
  counts: { tasks: number | null; inbox: number | null } = { tasks: 1, inbox: 0 },
  route = "/",
  drawer = false,
) {
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
    onNavigate: vi.fn(),
  };
  render(
    <MemoryRouter initialEntries={[route]}>
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
          counts={counts}
          query=""
          onQuery={() => {}}
          daemonViews={[]}
          appVersion=""
          actions={actions}
          width={260}
          onResize={() => {}}
          drawer={drawer}
        />
      </NeedsYouContext.Provider>
    </MemoryRouter>,
  );
  return actions;
}

it.each([false, true])(
  "keeps primary navigation before the scrolling agents (drawer=%s)",
  (drawer) => {
    renderSidebar([], undefined, "/", drawer);
    const sidebar = screen.getByTestId("agents-sidebar");
    const controls = Array.from(sidebar.querySelectorAll("a, button"));
    expect(controls.slice(0, 4).map((control) => control.textContent)).toEqual([
      "Home",
      "Tasks1",
      "Inbox",
      "Agents",
    ]);
    const agents = screen.getByRole("group", { name: "Agents" });
    expect(agents).toHaveClass("overflow-y-auto", "min-h-0");
    for (const name of ["Home", "Tasks", "Inbox", "Library", "Settings"]) {
      const link = within(sidebar).getByRole("link", { name: new RegExp(name) });
      expect(link.closest(".overflow-y-auto")).toBeNull();
    }
    const links = within(sidebar).getAllByRole("link");
    expect(links.slice(-3).map((link) => link.textContent)).toEqual([
      "Library",
      "Settings",
      "Local · ready",
    ]);
  },
);

it.each([
  { tasks: 0, inbox: 0 },
  { tasks: null, inbox: null },
])("hides absent and zero navigation counts: %j", (counts) => {
  renderSidebar([], counts);
  expect(
    screen.queryByRole("img", { name: /tasks need you|open letters/ }),
  ).not.toBeInTheDocument();
});

it.each([
  ["/", "Home"],
  ["/tasks", "Tasks"],
  ["/inbox", "Inbox"],
  ["/library/images", "Library"],
  ["/settings/hosts", "Settings"],
])("marks the current navigation page at %s", (route, name) => {
  renderSidebar([], { tasks: 3, inbox: 2 }, route);
  expect(screen.getByRole("link", { name: new RegExp(name) })).toHaveAttribute(
    "aria-current",
    "page",
  );
  expect(screen.getByRole("img", { name: "3 tasks need you" })).toHaveTextContent("3");
  expect(screen.getByRole("img", { name: "2 open letters" })).toHaveTextContent("2");
});

it("keeps navigation available when Agents is collapsed and closes the drawer on a link", () => {
  const actions = renderSidebar([], undefined, "/", true);
  fireEvent.click(screen.getByRole("button", { name: "Agents" }));
  expect(screen.getByRole("button", { name: "Agents" })).toHaveAttribute("aria-expanded", "false");
  expect(screen.queryByRole("group", { name: "Agents" })).not.toBeInTheDocument();
  expect(actions.onNavigate).not.toHaveBeenCalled();
  for (const name of ["Home", "Tasks", "Inbox", "Library", "Settings"]) {
    fireEvent.click(screen.getByRole("link", { name: new RegExp(name) }));
  }
  expect(actions.onNavigate).toHaveBeenCalledTimes(5);
});

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
