import { useEffect, useRef, useState, type UIEvent } from "react";

export function useChatScroll(shownCount: number) {
  const bottom = useRef<HTMLDivElement | null>(null);
  const unreadRule = useRef<HTMLDivElement | null>(null);
  const [atBottom, setAtBottom] = useState(true);
  // The effect reads position from a ref: changing position alone must not scroll.
  const atBottomRef = useRef(true);

  // Arrivals follow only when the customer is already at the conversation end.
  useEffect(() => {
    if (atBottomRef.current) bottom.current?.scrollIntoView({ block: "end" });
  }, [shownCount]);

  const onScroll = (event: UIEvent<HTMLDivElement>) => {
    const box = event.currentTarget;
    atBottomRef.current = box.scrollHeight - box.scrollTop - box.clientHeight < 40;
    setAtBottom(atBottomRef.current);
  };

  return { bottom, unreadRule, atBottom, onScroll };
}
