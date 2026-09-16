import { afterEach, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import AgentChat from "./AgentChat";
import { AgentNameContext } from "@/lib/agent";
import { DEFAULT_CHAT_TYPES } from "./chatTypes";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  localStorage.clear();
});

interface Call { path: string; method: string; body: unknown }

const feed = [
  {
    id: "m1", channel: "agent:worker:inbox", ts: "2026-09-15T10:00:00.000000000Z",
    from: "user:customer", source: "user:customer", type: "message", text: "please look at this",
  },
  {
    id: "m2", channel: "user:customer", ts: "2026-09-15T10:05:00.000000000Z",
    from: "agent:worker", source: "system:tasks", type: "task.question", text: "which option?",
    data: { task_key: "TEST-7" }, subject: { task_key: "TEST-7" },
  },
  {
    id: "m3", channel: "user:customer", ts: "2026-09-15T10:06:00.000000000Z",
    from: "agent:worker", source: "agent:worker", type: "message", text: "done with TEST-8 now",
  },
];

const taskDetail = {
  task: {
    key: "TEST-7", queue: "TEST", parent_key: "", position: 0, priority: "P1",
    title: "Answer the question", description: "", status: "open", pull_request: "",
    author: "user:customer", customer: "user:customer", group: "", assignee: "agent:worker",
    manual_block_reason: "", blocked: false, revision: 1,
    created_at: "2026-09-15T10:00:00Z", updated_at: "2026-09-15T10:00:00Z", completed_at: "",
  },
  comments: [], waiting_for: [], relations: [],
};

function stubChat() {
  const calls: Call[] = [];
  vi.stubGlobal("WebSocket", class {
    close = vi.fn();
  });
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string, init?: RequestInit) => {
    calls.push({
      path, method: init?.method ?? "GET",
      body: init?.body ? JSON.parse(init.body as string) : undefined,
    });
    let result: unknown = { ok: true };
    if (path.startsWith("/api/chats/worker?") || path === "/api/chats/worker") {
      result = { customer: "user:customer", agent: "worker", messages: feed, count: feed.length };
    } else if (path.startsWith("/api/tasks/TEST-7/events")) {
      result = { events: [], count: 0 };
    } else if (path.startsWith("/api/tasks/TEST-7")) {
      result = taskDetail;
    } else if (path === "/api/task-principals") {
      result = { customer: "user:customer", agents: ["worker"], groups: [] };
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

it("renders both halves of the conversation and marks it read at the newest shown message", async () => {
  const calls = stubChat();
  renderChat();
  expect(await screen.findByText("please look at this")).toBeInTheDocument();
  expect(screen.getByText("which option?")).toBeInTheDocument();

  const feedCall = calls.find((call) => call.path.startsWith("/api/chats/worker?"));
  expect(feedCall?.path).toContain(encodeURIComponent(DEFAULT_CHAT_TYPES.join(",")));
  await waitFor(() => {
    const read = calls.find((call) => call.path === "/api/chats/worker/read");
    expect(read?.body).toEqual({ ts: "2026-09-15T10:06:00.000000000Z" });
  });
});

it("sends into the agent's inbox with the customer channel as the reply target", async () => {
  const calls = stubChat();
  renderChat();
  await screen.findByText("please look at this");
  await userEvent.type(screen.getByLabelText("Message worker"), "on it");
  await userEvent.click(screen.getByRole("button", { name: "Send" }));
  await waitFor(() => {
    const sent = calls.find((call) => call.path === "/api/messages");
    expect(sent?.body).toEqual({
      channel: "agent:worker:inbox", type: "message", text: "on it", reply_to: "user:customer",
    });
  });
});

it("keeps a chosen message-type filter and refetches with it", async () => {
  const calls = stubChat();
  renderChat();
  await screen.findByText("please look at this");
  await userEvent.click(screen.getByRole("button", { name: "Message types" }));
  await userEvent.click(screen.getByRole("button", { name: "task.goal" }));
  await waitFor(() => {
    expect(calls.some((call) => call.path.includes(encodeURIComponent("task.goal")))).toBe(true);
  });
  expect(JSON.parse(localStorage.getItem("terminals:chat-types:v1") ?? "[]")).toContain("task.goal");
});

it("opens a task from its message without leaving the chat", async () => {
  const calls = stubChat();
  renderChat();
  await screen.findByText("which option?");
  // The key rides in the message data, and a key written in plain text is a link too.
  expect(screen.getByRole("button", { name: "TEST-8" })).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "TEST-7" }));
  expect(await screen.findByText("Answer the question")).toBeInTheDocument();
  expect(calls.some((call) => call.path.startsWith("/api/tasks/TEST-7"))).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: "Close task detail" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(screen.getByText("which option?")).toBeInTheDocument();
});
