import { useEffect, useRef, useState, type UIEvent } from "react";

export function useChatScroll(shownCount: number) {
  const bottom = useRef<HTMLDivElement | null>(null);
  const unreadRule = useRef<HTMLDivElement | null>(null);
  const [atBottom, setAtBottom] = useState(true);

  // Arrivals follow only when the customer is already at the conversation end.
  useEffect(() => {
    if (atBottom) bottom.current?.scrollIntoView({ block: "end" });
    // Changing position alone must not itself force a scroll.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [shownCount]);

  const onScroll = (event: UIEvent<HTMLDivElement>) => {
    const box = event.currentTarget;
    setAtBottom(box.scrollHeight - box.scrollTop - box.clientHeight < 40);
  };

  return { bottom, unreadRule, atBottom, onScroll };
}
