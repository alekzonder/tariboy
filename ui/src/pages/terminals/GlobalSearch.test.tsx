import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useState } from "react";
import { MemoryRouter } from "react-router-dom";
import { fireEvent, render, screen, waitFor } from "@/test/queryRender";
import type { HostAgents } from "@/lib/aggregate";
import type { Daemon } from "@/lib/daemons";
import { agentRow } from "@/test/fixtures/apiFixtures";
import { RoutedLocation } from "@/test/terminals/RoutedLocation";
import { GlobalSearch } from "./GlobalSearch";

const listTasks = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();
  return {
    ...actual,
    createApi: (target: Daemon | null) => ({
      tasks: { listTasks: (query: unknown) => listTasks(target?.id ?? "", query) },
    }),
  };
});
vi.mock("@/lib/terminalsHost", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/terminalsHost")>()),
  targetFor: (id: string) => (id === "" ? null : { id, label: id, baseURL: "", token: "" }),
}));

const agent = (name: string) =>
  agentRow({ name, image: "img", state: "idle", harness: "claude" });
const oneHost: HostAgents[] = [{ host: { id: "", label: "Local" }, agents: [agent("builder")] }];
const twoHosts: HostAgents[] = [
  ...oneHost,
  { host: { id: "d1", label: "prod" }, agents: [agent("deployer")] },
  { host: { id: "d2", label: "down" }, agents: [], error: "unreachable" },
];

function Harness({ hosts }: { hosts: HostAgents[] }) {
  const [query, setQuery] = useState("");
  return (
    <>
      <GlobalSearch hosts={hosts} query={query} onQuery={setQuery} />
      <output data-testid="query">{query}</output>
    </>
  );
}

function renderSearch(hosts: HostAgents[]) {
  return render(
    <MemoryRouter initialEntries={["/"]}>
      <Harness hosts={hosts} />
      <RoutedLocation />
    </MemoryRouter>,
  );
}

const field = () => screen.getByRole("combobox", { name: "Search agents and tasks" });

beforeEach(() => {
  listTasks.mockReset().mockImplementation(async (host: string) => {
    if (host === "d1") throw new Error("boom");
    return { tasks: [{ key: "DEV-7", title: "Fix the build", status: "review" }] };
  });
});
afterEach(() => vi.useRealTimers());

describe("GlobalSearch", () => {
  it("hands the text to the sidebar filter and lists matching agents", () => {
    renderSearch(oneHost);
    fireEvent.change(field(), { target: { value: "build" } });
    expect(screen.getByTestId("query")).toHaveTextContent("build");
    expect(screen.getByRole("option", { name: /^Agent\s*builder/ })).toBeInTheDocument();
  });

  it("finds tasks of every reachable host once typing stops, open or closed", async () => {
    renderSearch(twoHosts);
    fireEvent.change(field(), { target: { value: "build" } });
    expect(listTasks).not.toHaveBeenCalled();
    expect(await screen.findByRole("option", { name: /DEV-7 Fix the build/ })).toBeInTheDocument();
    expect(listTasks).toHaveBeenCalledTimes(2);
    expect(listTasks).toHaveBeenCalledWith("", { text: "build", status_view: "all", limit: 8 });
    expect(listTasks).toHaveBeenCalledWith("d1", { text: "build", status_view: "all", limit: 8 });
    // The failing host adds nothing and says nothing.
    expect(screen.getAllByRole("option", { name: /DEV-7/ })).toHaveLength(1);
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("opens the first result on Enter: a task on Tasks, naming its host only with several", async () => {
    renderSearch(oneHost);
    fireEvent.change(field(), { target: { value: "DEV-7" } });
    await screen.findByRole("option", { name: /DEV-7/ });
    fireEvent.keyDown(field(), { key: "Enter" });
    await waitFor(() =>
      expect(screen.getByTestId("location").textContent).toBe("/tasks?task=DEV-7"),
    );
  });

  it("names the host of a task when the workspace has two hosts", async () => {
    renderSearch(twoHosts.slice(0, 2).map((host) => ({ ...host, agents: [] })));
    listTasks.mockImplementation(async (host: string) => ({
      tasks: host === "" ? [] : [{ key: "OPS-1", title: "x", status: "open" }],
    }));
    fireEvent.change(field(), { target: { value: "OPS" } });
    const option = await screen.findByRole("option", { name: /OPS-1/ });
    expect(option).toHaveTextContent("prod");
    fireEvent.click(option.querySelector("button")!);
    await waitFor(() =>
      expect(screen.getByTestId("location").textContent).toBe("/tasks?host=d1&task=OPS-1"),
    );
  });

  it("focuses on ⌘K and Ctrl+K", () => {
    renderSearch(oneHost);
    fireEvent.keyDown(window, { key: "k", metaKey: true });
    expect(field()).toHaveFocus();
    field().blur();
    fireEvent.keyDown(window, { key: "K", ctrlKey: true });
    expect(field()).toHaveFocus();
  });
});
