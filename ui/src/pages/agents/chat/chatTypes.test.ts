import { afterEach, describe, expect, it } from "vitest";
import {
  CHAT_TYPES_KEY, DEFAULT_CHAT_TYPES, loadChatTypes, saveChatTypes,
} from "./chatTypes";

afterEach(() => localStorage.clear());

describe("chat type filter preference", () => {
  it("starts from the default set", () => {
    expect(loadChatTypes()).toEqual([...DEFAULT_CHAT_TYPES]);
  });

  it("round-trips a chosen set and forgets it once it is the default again", () => {
    saveChatTypes(["message", "task.goal"]);
    expect(loadChatTypes()).toEqual(["message", "task.goal"]);
    saveChatTypes([...DEFAULT_CHAT_TYPES]);
    expect(localStorage.getItem(CHAT_TYPES_KEY)).toBeNull();
    expect(loadChatTypes()).toEqual([...DEFAULT_CHAT_TYPES]);
  });

  it("falls back to the default for unusable stored state", () => {
    localStorage.setItem(CHAT_TYPES_KEY, "{not json");
    expect(loadChatTypes()).toEqual([...DEFAULT_CHAT_TYPES]);
    localStorage.setItem(CHAT_TYPES_KEY, "[]");
    expect(loadChatTypes()).toEqual([...DEFAULT_CHAT_TYPES]);
  });
});
