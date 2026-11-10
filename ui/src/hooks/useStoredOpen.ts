import { useState } from "react";
import { readKey, writeKey, type StorageKey } from "@/lib/storage";

/**
 * Whether a collapsible block is open, kept in Web Storage under `key`. The
 * choice belongs to the block, not to what it shows: a block closed on one
 * task is closed on the next one too.
 */
export function useStoredOpen(key: StorageKey<"open" | "closed">) {
  const [open, setOpen] = useState(() => readKey(key) === "open");
  const change = (next: boolean) => {
    writeKey(key, next ? "open" : "closed");
    setOpen(next);
  };
  return [open, change] as const;
}
