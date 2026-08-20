import { it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import ChannelsPage from "./ChannelsPage";

afterEach(() => vi.restoreAllMocks());

// Global mount (/channels): no :name param.
function renderGlobal() {
  return render(
    <MemoryRouter initialEntries={["/channels"]}>
      <Routes>
        <Route path="/channels" element={<ChannelsPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

// Agent-scoped mount (/agent/:name/channels).
function renderForAgent(name: string) {
  return render(
    <MemoryRouter initialEntries={[`/agent/${name}/channels`]}>
      <Routes>
        <Route path="/agent/:name/channels" element={<ChannelsPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

it("renders the global left column at 40% width (w-2/5)", async () => {
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string) => {
    let result: unknown = {};
    if (path === "/api/channels") result = { channels: [{ name: "chat:room", kind: "chat" }], count: 1 };
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result }) } as Response);
  }));
  renderGlobal();
  // The CHANNELS label sits directly inside the column root, which carries the
  // 40%-width class plus the min-width floor.
  const column = (await screen.findByText("CHANNELS")).parentElement!;
  expect(column).toHaveClass("w-2/5");
  expect(column).toHaveClass("min-w-[12rem]");
});

it("per-agent mount: clicking a subscription row selects it and populates the right pane", async () => {
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string) => {
    let result: unknown = {};
    if (path === "/api/agents/worker/subscriptions") result = { channels: [{ name: "group:dev:broadcast", kind: "group_broadcast", protected: true }], count: 1 };
    else if (path === "/api/channels") result = { channels: [], count: 0 };
    else if (path.includes("/messages")) result = { messages: [{ id: "m1", ts: "", type: "note", source: "op", text: "pane-hello" }], count: 1 };
    else if (path.includes("/watches")) result = { watches: [], count: 0 };
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result }) } as Response);
  }));
  renderForAgent("worker");

  // The row is present and the right pane starts on its empty state.
  await waitFor(() => expect(screen.getByText("group:dev:broadcast")).toBeInTheDocument());
  expect(screen.getByText("Select a channel.")).toBeInTheDocument();

  // Clicking the subscription row selects that channel: the empty state clears
  // and the channel's message tail renders in the right pane.
  fireEvent.click(screen.getByText("group:dev:broadcast"));
  await waitFor(() => expect(screen.getByText("pane-hello")).toBeInTheDocument());
  expect(screen.queryByText("Select a channel.")).toBeNull();
});

it("lists channels, tails a selected channel, and sends a message", async () => {
  const posts: Array<{ path: string; body: unknown }> = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string, init?: RequestInit) => {
    let body: unknown = { ok: true, result: {} };
    if (init?.method === "POST") posts.push({ path, body: init?.body ? JSON.parse(init.body as string) : undefined });
    if (path === "/api/channels") body = { ok: true, result: { channels: [{ name: "chat:room", kind: "chat" }], count: 1 } };
    else if (path.includes("/messages")) body = { ok: true, result: { messages: [{ id: "m1", ts: "", type: "note", source: "op", text: "hello" }], count: 1 } };
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify(body) } as Response);
  }));
  renderGlobal();

  await waitFor(() => expect(screen.getByText("chat:room")).toBeInTheDocument());
  fireEvent.click(screen.getByText("chat:room"));
  await waitFor(() => expect(screen.getByText("hello")).toBeInTheDocument());

  fireEvent.change(screen.getByPlaceholderText("message text"), { target: { value: "hi there" } });
  fireEvent.click(screen.getByText("Send"));
  await waitFor(() =>
    expect(posts.some((p) => p.path === "/api/messages" && (p.body as { text?: string })?.text === "hi there")).toBe(true),
  );
});

it("shows a selected channel's watches (watch, params, subscribers), read-only", async () => {
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string) => {
    let result: unknown = {};
    if (path === "/api/channels") result = { channels: [{ name: "issue-provider:issues", kind: "provider" }], count: 1 };
    else if (path.includes("/watches")) result = {
      watches: [
        { watch: "project=alpha", params: { project: "alpha", label: "bug" }, subscribers: ["worker", "lead"] },
      ], count: 1,
    };
    else if (path.includes("/messages")) result = { messages: [], count: 0 };
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result }) } as Response);
  }));
  renderGlobal();

  await waitFor(() => expect(screen.getByText("issue-provider:issues")).toBeInTheDocument());
  fireEvent.click(screen.getByText("issue-provider:issues"));

  // Watch identity, pretty-printed params, and subscribers all surface.
  await waitFor(() => expect(screen.getByText("project=alpha")).toBeInTheDocument());
  expect(screen.getByText(/"project": "alpha"/)).toBeInTheDocument();
  expect(screen.getByText(/worker/)).toBeInTheDocument();
  expect(screen.getByText(/lead/)).toBeInTheDocument();
});

it("scopes to one agent's subscriptions and no longer shows the messenger card", async () => {
  const seen: string[] = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string) => {
    seen.push(path);
    let result: unknown = {};
    // Under an agent the left column is AgentSubscriptions (its own fetches); the
    // messenger chat-routes card moved to the Plugins tab, so it must NOT appear here.
    if (path === "/api/agents/worker/subscriptions") result = { channels: [{ name: "group:dev:broadcast", kind: "group_broadcast", protected: true }], count: 1 };
    else if (path === "/api/channels") result = { channels: [], count: 0 };
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result }) } as Response);
  }));
  renderForAgent("worker");

  await waitFor(() => expect(screen.getByText("group:dev:broadcast")).toBeInTheDocument());
  // The agent-scoped subscriptions endpoint was queried.
  expect(seen).toContain("/api/agents/worker/subscriptions");
  // The messenger chat-routes card is gone from the agent tab.
  expect(screen.queryByText("messenger chats")).toBeNull();
});
