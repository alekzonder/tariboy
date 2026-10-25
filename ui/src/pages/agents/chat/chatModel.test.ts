import { describe, expect, it } from "vitest";
import type { ChatSummary } from "@/lib/api";
import type { FeedItem } from "./chatFeed";
import {
  chatAddress,
  chatParticipants,
  filterFeed,
  pendingChatMessage,
  quoteReply,
} from "./chatModel";

describe("chatAddress", () => {
  it("addresses an agent kind by kind and name and a shared chat by its id", () => {
    expect(chatAddress({ name: "worker", selected: "dm" })).toEqual({
      chatId: "dm:worker",
      writable: true,
    });
    expect(chatAddress({ name: "worker", selected: "tasks" })).toEqual({
      chatId: "tasks:worker",
      writable: false,
    });
    expect(chatAddress({ name: "worker", selected: "team-alpha" })).toEqual({
      chatId: "team-alpha",
      writable: true,
    });
    expect(chatAddress({ name: "", selected: "dm" }).chatId).toBe("");
  });
});

describe("chatParticipants", () => {
  it("names the customer and the agent in its own chats and the members of a shared one", () => {
    expect(
      chatParticipants({ selected: "dm", customer: "user:c", name: "worker", current: undefined }),
    ).toEqual(["user:c", "agent:worker"]);
    const current = { participants: ["user:c", "agent:a"] } as ChatSummary;
    expect(chatParticipants({ selected: "team", customer: "user:c", name: "w", current })).toEqual(
      current.participants,
    );
    expect(
      chatParticipants({ selected: "team", customer: "user:c", name: "w", current: undefined }),
    ).toEqual([]);
  });
});

describe("filterFeed", () => {
  const message = (text: string): FeedItem =>
    ({ kind: "message", message: { text }, mine: false, head: true }) as FeedItem;
  const day: FeedItem = { kind: "day", id: "d", label: "Today" };

  it("keeps dividers and the messages whose text contains the query", () => {
    const feed = [day, message("Hello there"), message("bye")];
    expect(filterFeed({ feed, query: "  " })).toBe(feed);
    expect(filterFeed({ feed, query: " HELLO " })).toEqual([day, feed[1]]);
  });
});

describe("pendingChatMessage", () => {
  it("is the customer message in the chat channel stamped now", () => {
    const now = new Date("2026-09-15T10:00:00Z");
    expect(pendingChatMessage({ chatId: "dm:w", customer: "user:c", text: "hi", now })).toEqual({
      id: `pending:${now.getTime()}`,
      channel: "chat:dm:w",
      ts: now.toISOString(),
      from: "user:c",
      source: "user:c",
      type: "message",
      text: "hi",
    });
  });
});

describe("quoteReply", () => {
  it("quotes every line above the current draft", () => {
    expect(quoteReply({ text: "a\nb", draft: "c" })).toBe("> a\n> b\n\nc");
  });
});
