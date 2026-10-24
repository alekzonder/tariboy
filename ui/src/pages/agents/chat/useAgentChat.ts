import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { useAgentName } from "@/lib/agent";
import { ApiError, type ChatMessage, type ChatSummary } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { useMessagesSocket } from "@/hooks/useMessagesSocket";
import { DEFAULT_CHAT_TYPES, loadChatTypes, saveChatTypes } from "./chatTypes";
import { buildFeed, unreadCount, type FeedMessage } from "./chatFeed";
import { sharedChats } from "./chatModel";
import { isKind } from "./chatKinds";
import { useChatReadState } from "./useChatReadState";

const QUESTION_TYPE = "task.question";

export function useAgentChat(hostId: string) {
  const name = useAgentName();
  const api = useApi();
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [customer, setCustomer] = useState("user:customer");
  const [types, setTypes] = useState<string[]>(() => loadChatTypes());
  const [draft, setDraft] = useState("");
  const [query, setQuery] = useState("");
  const [sending, setSending] = useState(false);
  // The message being published, shown with a `sent` receipt until the refetch
  // brings back the stored one.
  const [pending, setPending] = useState<ChatMessage | null>(null);
  // Either one of the agent's three kinds or the id of a shared chat it is in.
  const [selected, setSelected] = useState<string>("dm");
  const [shared, setShared] = useState<ChatSummary[]>([]);
  const typesKey = types.join(",");
  // The chat id the endpoints are addressed by. An agent name still resolves to
  // the personal chat, so the sidebar keeps working with what it knows.
  const chatId = !name ? "" : isKind(selected) ? `${selected}:${name}` : selected;
  // Only the conversation and the shared chats are written into: the other two
  // are the agent's own notification feeds, and the customer is an observer in
  // the service one.
  const writable = selected === "dm" || !isKind(selected);
  const current = shared.find((chat) => chat.id === selected);
  // Only a participant's message can be unread, the rule the daemon counts by.
  const participants = useMemo(
    () => (isKind(selected) ? [customer, `agent:${name}`] : current?.participants ?? []),
    [selected, customer, name, current],
  );

  const { anchor, captureAnchor, markShownRead, markRead, markUnread } = useChatReadState({
    name, hostId, selected, chatId, messages, customer,
  });

  const load = useCallback(async () => {
    if (!name || !api.ready) return;
    try {
      const page = await api.messages.chatMessages(chatId, { types });
      captureAnchor(page.read_ts ?? "");
      setMessages(page.messages ?? []);
      setCustomer(page.customer || "user:customer");
      const newest = page.messages?.[page.messages.length - 1];
      // The mark is the timestamp of a message actually shown, never "now", so
      // one arriving mid-render cannot be marked read without being seen. A
      // hidden tab is not "shown": a chat left open behind another window keeps
      // counting its messages as unread. A chat the customer deliberately
      // marked unread is not marked read again either.
      await markShownRead(newest);
    } catch {
      // Keep the last conversation on a transient failure.
    }
    // typesKey stands for the type list, which changes identity every render.
    // The client keeps its identity until the host's endpoint moves, and a
    // moved endpoint (a reconnect) must re-read the conversation.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [name, api, typesKey, selected]);

  useEffect(() => {
    setMessages([]);
    setPending(null);
  }, [name, hostId, selected]);

  // The shared chats are read from the same list the sidebar reads, so a chat
  // created in another window appears here without its own endpoint.
  const loadShared = useCallback(async () => {
    if (!name || !api.ready) return;
    try {
      const page = await api.messages.chatList();
      setShared(sharedChats(page.chats ?? [], name));
    } catch {
      // A transient failure keeps the tabs that are already shown.
    }
  }, [name, api]);

  useEffect(() => { void load(); }, [load]);
  useEffect(() => { void loadShared(); }, [loadShared]);
  useMessagesSocket({
    enabled: api.ready && Boolean(name),
    // A frame that names its chat refetches only that one; a publication on a
    // channel no chat owns still falls back to the agent it concerns.
    onHint: (hint) => {
      if (hint.chat ? hint.chat === chatId : hint.agent === name) void load();
    },
    onOpen: () => { void load(); },
  });

  const feed = useMemo(
    () => buildFeed(
      pending ? [...messages, pending] : messages,
      customer,
      anchor ?? "",
      participants,
    ),
    [messages, pending, customer, anchor, participants],
  );
  const shown = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return feed;
    return feed.filter((item) => item.kind !== "message" || item.message.text.toLowerCase().includes(needle));
  }, [feed, query]);
  const newCount = anchor === null ? 0 : unreadCount(messages, customer, anchor, participants);
  const openQuestion = messages.at(-1)?.type === QUESTION_TYPE
    && messages.at(-1)?.from !== customer;

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

  const send = async () => {
    const text = draft.trim();
    if (!text || sending) return;
    setSending(true);
    const chatChannel = `chat:${chatId}`;
    setPending({
      id: `pending:${Date.now()}`, channel: chatChannel, ts: new Date().toISOString(),
      from: customer, source: customer, type: "message", text,
    });
    try {
      // The chat owns this channel, so the agent reply lands here without the
      // client having to name a reply target.
      await api.messages.messageSend({ channel: chatChannel, type: "message", text });
      setDraft("");
      // Speaking is reading: the customer has answered whatever was new.
      markRead();
      await load();
    } catch (e) {
      toast.error(`send failed: ${e instanceof ApiError ? e.message : String(e)}`);
    } finally {
      setPending(null);
      setSending(false);
    }
  };

  function reply(item: FeedMessage) {
    const quoted = item.message.text.split("\n").map((line) => `> ${line}`).join("\n");
    setDraft((current) => `${quoted}\n\n${current}`);
  }

  return {
    name, messages, customer, types, draft, setDraft, query, setQuery,
    sending, selected, setSelected, shared, current, writable, shown,
    newCount, openQuestion, markRead, markUnread, toggleType, resetTypes,
    send, reply, loadShared,
  };
}

export type AgentChatState = ReturnType<typeof useAgentChat>;
