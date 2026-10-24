import { useState } from "react";
import { toast } from "sonner";
import { ApiError, type ChatMessage } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import type { FeedMessage } from "./chatFeed";
import { pendingChatMessage, quoteReply } from "./chatModel";

/** The composer's draft and the publish of it. The sent message stays in the
 *  feed as a pending bubble until the refetch brings back the stored one. */
export function useChatSend({
  chatId,
  customer,
  scope,
  markRead,
  refetch,
}: {
  chatId: string;
  customer: string;
  /** The conversation the pending bubble belongs to, so a switch hides it. */
  scope: string;
  markRead: () => void;
  /** Resolves once the conversation has been read again. */
  refetch: () => Promise<void>;
}) {
  const api = useApi();
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const [pendingState, setPending] = useState<{ scope: string; message: ChatMessage } | null>(null);
  const pending = pendingState?.scope === scope ? pendingState.message : null;

  const send = async () => {
    const text = draft.trim();
    if (!text || sending) return;
    setSending(true);
    const message = pendingChatMessage({ chatId, customer, text, now: new Date() });
    setPending({ scope, message });
    try {
      // The chat owns this channel, so the agent reply lands here without the
      // client having to name a reply target.
      await api.messages.messageSend({ channel: message.channel, type: "message", text });
      setDraft("");
      // Speaking is reading: the customer has answered whatever was new.
      markRead();
      await refetch();
    } catch (e) {
      toast.error(`send failed: ${e instanceof ApiError ? e.message : String(e)}`);
    } finally {
      setPending(null);
      setSending(false);
    }
  };

  function reply(item: FeedMessage) {
    setDraft((current) => quoteReply({ text: item.message.text, draft: current }));
  }

  return { draft, setDraft, sending, pending, send, reply };
}
