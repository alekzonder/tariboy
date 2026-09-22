import { afterEach, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useChatUnread } from "./useChatUnread";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const summary = (agent: string, id: string, kind: string, unread: number) => ({
  id, kind, title: agent, agent, unread, last_ts: "2026-09-15T10:00:00Z",
  last_from: `agent:${agent}`, last_type: "message", last_text: "...",
});

// The dot on the Chat tab stands for the agent, not for one of its three chats:
// an unread task notification must not hide behind a quiet conversation.
it("counts every chat of the agent, not the first one listed", async () => {
  vi.stubGlobal("WebSocket", class { close = vi.fn(); });
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
    ok: true, status: 200,
    text: async () => JSON.stringify({
      ok: true,
      result: {
        customer: "user:customer",
        chats: [
          summary("worker", "dm:worker", "direct", 1),
          summary("worker", "tasks:worker", "tasks", 2),
          summary("worker", "service:worker", "service", 4),
          summary("other", "dm:other", "direct", 9),
        ],
        count: 4,
      },
    }),
  } as Response));

  const { result } = renderHook(() => useChatUnread(null, "worker"));
  await waitFor(() => expect(result.current).toBe(7));
});
