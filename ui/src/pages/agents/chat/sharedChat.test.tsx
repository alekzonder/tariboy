import { afterEach, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import AgentChat from "./AgentChat";
import { chatIdFromTitle, sharedChats } from "./chatModel";
import { AgentNameContext } from "@/lib/agent";
import type { ChatSummary } from "@/lib/api";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  localStorage.clear();
});

function summary(over: Partial<ChatSummary>): ChatSummary {
  return {
    id: "x", kind: "direct", title: "x", agent: "worker",
    last_ts: "", last_from: "", last_type: "", last_text: "", unread: 0, ...over,
  };
}

// A shared chat belongs to no single agent, so the agent it concerns can only
// be read off its participants. An agent's own three chats are never shared.
it("keeps only the chats this agent takes part in that no agent owns", () => {
  const chats = [
    summary({ id: "dm:worker", agent: "worker" }),
    summary({ id: "team-alpha", kind: "group", agent: "", participants: ["user:customer", "agent:worker"] }),
    summary({ id: "team-beta", kind: "group", agent: "", participants: ["user:customer", "agent:other"] }),
    summary({ id: "team-none", kind: "group", agent: "" }),
  ];
  expect(sharedChats(chats, "worker").map((chat) => chat.id)).toEqual(["team-alpha"]);
});

// The title is what the customer types; the id is a channel segment, and the
// daemon refuses anything else.
it("derives a channel-safe chat id from the title", () => {
  expect(chatIdFromTitle("Team Alpha")).toBe("team-alpha");
  expect(chatIdFromTitle("  Ops / On-call  ")).toBe("ops-on-call");
  expect(chatIdFromTitle("!!!")).toBe("");
});

interface Call { path: string; method: string; body: unknown }

function stub(): Call[] {
  const calls: Call[] = [];
  vi.stubGlobal("WebSocket", class { close = vi.fn(); });
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string, init?: RequestInit) => {
    calls.push({
      path, method: init?.method ?? "GET",
      body: init?.body ? JSON.parse(init.body as string) : undefined,
    });
    let result: unknown = { ok: true };
    if (path === "/api/chats") {
      result = {
        customer: "user:customer", count: 1,
        chats: [summary({
          id: "team-alpha", kind: "group", title: "Team Alpha", agent: "",
          participants: ["user:customer", "agent:worker", "agent:reviewer"],
        })],
      };
    } else if (path === "/api/agents") {
      result = { agents: [{ name: "worker" }, { name: "reviewer" }], count: 2 };
    } else if (path.startsWith("/api/chats/")) {
      result = {
        customer: "user:customer", agent: "", chat: "team-alpha", kind: "group",
        messages: [], count: 0, read_ts: "",
      };
    }
    return Promise.resolve({
      ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result }),
    } as Response);
  }));
  return calls;
}

function renderChat() {
  return render(
    <AgentNameContext.Provider value="worker">
      <AgentChat />
    </AgentNameContext.Provider>,
  );
}

// A shared chat is another thread this agent is in, so it sits beside the
// agent's own three, names who is in it, and is written into like a
// conversation — the two notification feeds are not.
it("opens a shared chat from its own tab and sends into its channel", async () => {
  const calls = stub();
  renderChat();
  const tab = await screen.findByRole("tab", { name: "Team Alpha" });

  await userEvent.click(tab);
  await waitFor(() => expect(calls.some((call) =>
    call.path.startsWith(`/api/chats/${encodeURIComponent("team-alpha")}`))).toBe(true));
  expect(await screen.findByText("customer, worker, reviewer")).toBeTruthy();

  const box = screen.getByRole("textbox", { name: /message/i });
  await userEvent.type(box, "ready");
  await userEvent.keyboard("{Meta>}{Enter}{/Meta}");
  await waitFor(() => {
    const sent = calls.find((call) => call.path === "/api/messages" && call.method === "POST");
    expect((sent?.body as { channel: string })?.channel).toBe("chat:team-alpha");
  });
});

// Starting a chat names the agents in it, because membership is what carries
// delivery: an agent that is not a participant is never woken by it.
it("creates a chat with this agent and the ones picked", async () => {
  const calls = stub();
  renderChat();
  await userEvent.click(await screen.findByRole("button", { name: "New chat" }));
  await userEvent.type(screen.getByLabelText("Title"), "Team Alpha");
  await userEvent.click(await screen.findByLabelText("reviewer"));
  await userEvent.click(screen.getByRole("button", { name: "Create" }));

  await waitFor(() => {
    const created = calls.find((call) => call.path === "/api/chats" && call.method === "POST");
    expect(created?.body).toEqual({
      id: "team-alpha", title: "Team Alpha", participants: "agent:worker,agent:reviewer",
    });
  });
});
