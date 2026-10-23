import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { ApiError, chatReadOn, type ApiTarget, type ChatMessage } from "@/lib/api";

export function useChatReadState({
  name, hostId, selected, target, chatId, messages, customer,
}: {
  name: string;
  hostId: string;
  selected: string;
  target: ApiTarget;
  chatId: string;
  messages: ChatMessage[];
  customer: string;
}) {
  // Capture the opening mark once so arrivals cannot move the unread divider.
  const [anchor, setAnchor] = useState<string | null>(null);
  const anchorRef = useRef<string | null>(null);
  const keepUnread = useRef(false);

  useEffect(() => {
    anchorRef.current = null;
    keepUnread.current = false;
    // Preserve the existing scope reset alongside the conversation's message reset.
    // eslint-disable-next-line react/set-state-in-effect
    setAnchor(null);
  }, [name, hostId, selected]);

  const captureAnchor = useCallback((readTS: string) => {
    if (anchorRef.current === null) {
      anchorRef.current = readTS;
      setAnchor(readTS);
    }
  }, []);

  const markShownRead = useCallback(async (newest?: ChatMessage) => {
    // A hidden tab and an explicit Mark unread must never advance the mark.
    if (newest && !keepUnread.current && document.visibilityState !== "hidden") {
      await chatReadOn(target, chatId, newest.ts);
    }
  }, [target, chatId]);

  function markRead() {
    const newest = messages.at(-1);
    keepUnread.current = false;
    anchorRef.current = newest?.ts ?? "";
    setAnchor(anchorRef.current);
    if (newest && name) void chatReadOn(target, chatId, newest.ts).catch(() => undefined);
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
      await chatReadOn(target, chatId, back, { exact: true });
    } catch (error) {
      toast.error(`Could not mark the chat unread: ${error instanceof ApiError ? error.message : String(error)}`);
    }
  }

  return { anchor, captureAnchor, markShownRead, markRead, markUnread };
}
