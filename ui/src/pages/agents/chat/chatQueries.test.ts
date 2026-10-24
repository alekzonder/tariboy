import { expect, it, vi } from "vitest";
import type { ApiClient } from "@/lib/api";
import { apiServerKey } from "@/lib/queryClient";
import { chatKeys, chatQueries } from "./chatQueries";

function fakeApi() {
  const chatList = vi.fn().mockResolvedValue({ chats: [] });
  const chatMessages = vi.fn().mockResolvedValue({ messages: [] });
  const api = { target: null, messages: { chatList, chatMessages } } as unknown as ApiClient;
  return { api, chatList, chatMessages };
}

it("keys chat reads under the server and reads them from that client", async () => {
  const { api, chatList, chatMessages } = fakeApi();
  const list = chatQueries.list(api);
  expect(list.queryKey).toEqual([...apiServerKey(null), "chats", "list"]);
  await (list.queryFn as () => Promise<unknown>)();
  expect(chatList).toHaveBeenCalledWith();

  const messages = chatQueries.messages(api, "dm:w", ["message"]);
  expect(messages.queryKey.slice(0, chatKeys.chat(null, "dm:w").length)).toEqual(
    chatKeys.chat(null, "dm:w"),
  );
  expect(messages.enabled).toBe(true);
  await (messages.queryFn as () => Promise<unknown>)();
  expect(chatMessages).toHaveBeenCalledWith("dm:w", { types: ["message"] });
  expect(chatQueries.messages(api, "", ["message"]).enabled).toBe(false);
});

it("keeps showing the same chat while another type filter loads, never another chat", () => {
  const { api } = fakeApi();
  const options = chatQueries.messages(api, "dm:w", ["message", "task.goal"]);
  const keep = options.placeholderData as (
    previous: unknown,
    query: { queryKey: readonly unknown[] } | undefined,
  ) => unknown;
  const page = { messages: [] };
  expect(keep(page, { queryKey: chatKeys.messages(null, "dm:w", ["message"]) })).toBe(page);
  expect(keep(page, { queryKey: chatKeys.messages(null, "tasks:w", ["message"]) })).toBeUndefined();
  expect(keep(page, undefined)).toBeUndefined();
  expect(options.gcTime).toBe(0);
});
