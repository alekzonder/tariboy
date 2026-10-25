import { useCallback, useEffect, useLayoutEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useAgentName } from "@/lib/agent";
import type { ChatMessage } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { useApiQuery } from "@/hooks/useApiQuery";
import { isAccessError } from "@/lib/queryClient";
import { useMessagesSocket } from "@/hooks/useMessagesSocket";
import { DEFAULT_CHAT_TYPES, loadChatTypes, saveChatTypes } from "./chatTypes";
import { buildFeed, unreadCount } from "./chatFeed";
import { chatAddress, chatParticipants, filterFeed, sharedChats } from "./chatModel";
import { chatKeys, chatQueries } from "./chatQueries";
import { useChatReadState } from "./useChatReadState";
import { useChatSend } from "./useChatSend";

const QUESTION_TYPE = "task.question";
const NO_MESSAGES: ChatMessage[] = [];

/** The conversation's query, with the page read for it and the page shown. */
function useConversation(options: ReturnType<typeof chatQueries.messages>, scope: string) {
  const conversation = useApiQuery(options);
  const page = conversation.isPlaceholderData ? undefined : conversation.data;
  // A transient failure keeps the last conversation shown, also when the read
  // with a new type filter fails; a refused read shows nothing.
  const [kept, setKept] = useState<{ scope: string; page: typeof page }>();
  if (page && (kept?.scope !== scope || kept.page !== page)) setKept({ scope, page });
  const keep = conversation.isError && !isAccessError(conversation.error) && kept?.scope === scope;
  return { conversation, page, shownPage: conversation.data ?? (keep ? kept.page : undefined) };
}

export function useAgentChat(hostId: string) {
  const name = useAgentName();
  const api = useApi();
  const client = useQueryClient();
  const [types, setTypes] = useState<string[]>(() => loadChatTypes());
  const [query, setQuery] = useState("");
  // Either one of the agent's three kinds or the id of a shared chat it is in.
  const [selected, setSelected] = useState<string>("dm");
  // Pending messages carry the conversation they belong to, so a switch of
  // agent, host or chat never shows the previous one.
  const scope = [name, hostId, selected].join("\u0000");
  const { chatId, writable } = chatAddress({ name, selected });

  // The shared chats are read from the same list the sidebar reads, so a chat
  // created in another window appears here without its own endpoint. A
  // transient failure keeps the tabs that are already shown.
  const list = useApiQuery({ ...chatQueries.list(api), enabled: Boolean(name) });
  const shared = useMemo(() => sharedChats(list.data?.chats ?? [], name), [list.data, name]);
  const loadShared = useCallback(
    () => client.invalidateQueries({ queryKey: chatKeys.list(api.target) }),
    [client, api.target],
  );

  const { conversation, page, shownPage } = useConversation(
    chatQueries.messages(api, chatId, types),
    scope,
  );
  const messages = shownPage?.messages ?? NO_MESSAGES;
  const customer = shownPage?.customer || "user:customer";
  const fetchedAt = page ? conversation.dataUpdatedAt : 0;
  const refetch = useCallback(
    () => client.invalidateQueries({ queryKey: chatKeys.chat(api.target, chatId) }),
    [client, api.target, chatId],
  );

  const current = shared.find((chat) => chat.id === selected);
  const participants = useMemo(
    () => chatParticipants({ selected, customer, name, current }),
    [selected, customer, name, current],
  );

  const { anchor, captureAnchor, markShownRead, markRead, markUnread } = useChatReadState({
    name,
    hostId,
    selected,
    chatId,
    messages,
    customer,
  });

  // Each page read from the daemon: the opening mark is captured before it
  // paints, then the newest message shown is marked read. The mark is the
  // timestamp of a message actually shown, never "now", so one arriving
  // mid-render cannot be marked read without being seen. A hidden tab is not
  // "shown": a chat left open behind another window keeps counting its
  // messages as unread. A chat the customer deliberately marked unread is not
  // marked read again either.
  useLayoutEffect(() => {
    if (page) captureAnchor(page.read_ts ?? "");
  }, [page, fetchedAt, captureAnchor]);
  useEffect(() => {
    if (page) void markShownRead(page.messages?.at(-1)).catch(() => undefined);
  }, [page, fetchedAt, markShownRead]);

  useMessagesSocket({
    enabled: api.ready && Boolean(name),
    // A frame that names its chat refetches only that one; a publication on a
    // channel no chat owns still falls back to the agent it concerns.
    onHint: (hint) => {
      if (hint.chat ? hint.chat === chatId : hint.agent === name) void refetch();
    },
    onOpen: () => {
      void refetch();
    },
  });

  const { draft, setDraft, sending, pending, send, reply } = useChatSend({
    chatId,
    customer,
    scope,
    markRead,
    refetch,
  });

  const feed = useMemo(
    () =>
      buildFeed(pending ? [...messages, pending] : messages, customer, anchor ?? "", participants),
    [messages, pending, customer, anchor, participants],
  );
  const shown = useMemo(() => filterFeed({ feed, query }), [feed, query]);
  const newCount = anchor === null ? 0 : unreadCount(messages, customer, anchor, participants);
  const openQuestion =
    messages.at(-1)?.type === QUESTION_TYPE && messages.at(-1)?.from !== customer;

  const toggleType = (type: string) => {
    const next = types.includes(type) ? types.filter((t) => t !== type) : [...types, type];
    if (next.length === 0) return;
    setTypes(next);
    saveChatTypes(next);
  };

  const resetTypes = () => {
    setTypes([...DEFAULT_CHAT_TYPES]);
    saveChatTypes([...DEFAULT_CHAT_TYPES]);
  };

  return {
    name,
    messages,
    customer,
    types,
    draft,
    setDraft,
    query,
    setQuery,
    sending,
    selected,
    setSelected,
    shared,
    current,
    writable,
    shown,
    newCount,
    openQuestion,
    markRead,
    markUnread,
    toggleType,
    resetTypes,
    send,
    reply,
    loadShared,
  };
}

export type AgentChatState = ReturnType<typeof useAgentChat>;
