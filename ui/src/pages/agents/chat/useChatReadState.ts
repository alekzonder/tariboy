import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { ApiError, type ChatMessage } from "@/lib/api";
import { useApi } from "@/components/apiContext";

export function useChatReadState({
  name,
  hostId,
  selected,
  chatId,
  messages,
  customer,
}: {
  name: string;
  hostId: string;
  selected: string;
  chatId: string;
  messages: ChatMessage[];
  customer: string;
}) {
  const api = useApi();
  // Capture the opening mark once so arrivals cannot move the unread divider.
  // The mark carries its conversation, so a switch starts without a divider.
  const scope = [name, hostId, selected].join("\u0000");
  const scopeRef = useRef(scope);
  const [anchorState, setAnchorState] = useState<{ scope: string; ts: string } | null>(null);
  const anchor = anchorState?.scope === scope ? anchorState.ts : null;
  const anchorRef = useRef<string | null>(null);
  const keepUnread = useRef(false);
  const setAnchor = useCallback(
    (ts: string) => setAnchorState({ scope: scopeRef.current, ts }),
    [],
  );

  useEffect(() => {
    scopeRef.current = scope;
    anchorRef.current = null;
    keepUnread.current = false;
  }, [scope]);

  const captureAnchor = useCallback(
    (readTS: string) => {
      if (anchorRef.current === null) {
        anchorRef.current = readTS;
        setAnchor(readTS);
      }
    },
    [setAnchor],
  );

  const markShownRead = useCallback(
    async (newest?: ChatMessage) => {
      // A hidden tab and an explicit Mark unread must never advance the mark.
      if (newest && !keepUnread.current && document.visibilityState !== "hidden") {
        await api.messages.chatRead(chatId, newest.ts);
      }
    },
    [api, chatId],
  );

  function markRead() {
    const newest = messages.at(-1);
    keepUnread.current = false;
    anchorRef.current = newest?.ts ?? "";
    setAnchor(anchorRef.current);
    if (newest && name) void api.messages.chatRead(chatId, newest.ts).catch(() => undefined);
  }

  async function markUnread() {
    // Unread means the agent's last word is new again, so the mark goes back to
    // the message before it. That is the one write that moves the mark
    // backwards, and it says so.
    const lastAgent = messages.findLastIndex((message) => message.from !== customer);
    if (lastAgent < 0 || !name) return;
    const back = messages[lastAgent - 1]?.ts ?? "";
    keepUnread.current = true;
    anchorRef.current = back;
    setAnchor(back);
    try {
      await api.messages.chatRead(chatId, back, { exact: true });
    } catch (error) {
      toast.error(
        `Could not mark the chat unread: ${error instanceof ApiError ? error.message : String(error)}`,
      );
    }
  }

  return { anchor, captureAnchor, markShownRead, markRead, markUnread };
}
