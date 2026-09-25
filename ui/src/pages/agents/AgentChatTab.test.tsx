import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import AgentChatTab from "./AgentChatTab";
import { AgentNameContext } from "@/lib/agent";
import { UI_MODE_KEY } from "@/lib/uiMode";

beforeEach(() => {
  localStorage.clear();
  vi.stubGlobal("WebSocket", class { close = vi.fn(); });
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
    ok: true, status: 200,
    text: async () => JSON.stringify({ ok: true, result: { messages: [], count: 0, read_ts: "" } }),
  } as unknown as Response));
});
afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
});

function renderTab(path = "/") {
  render(
    <MemoryRouter initialEntries={[path]}>
      <AgentNameContext.Provider value="worker">
        <AgentChatTab />
      </AgentNameContext.Provider>
    </MemoryRouter>,
  );
}

it("in Simple shows only the personal chat, its toolbar starting at search", async () => {
  renderTab("/?view=channels");

  expect(await screen.findByRole("searchbox", { name: "Search messages" })).toBeInTheDocument();
  expect(screen.queryByRole("group", { name: "Messaging view" })).not.toBeInTheDocument();
  expect(screen.queryByRole("tablist", { name: "Agent chats" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "New chat" })).not.toBeInTheDocument();
});

it("in Expert keeps the Chat/Channels switch and the agent chats", async () => {
  localStorage.setItem(UI_MODE_KEY, "expert");
  renderTab();

  expect(await screen.findByRole("group", { name: "Messaging view" })).toBeInTheDocument();
  expect(screen.getByRole("tablist", { name: "Agent chats" })).toBeInTheDocument();
  expect(screen.getByRole("searchbox", { name: "Search messages" })).toBeInTheDocument();
});
