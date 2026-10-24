import type { InboxItem, InboxStatus } from "@/lib/api";

// The three sub-views map onto the P5 inbox status filter. Queue is the default.
export type View = "queue" | "archive" | "dlq";
export const VIEWS: [View, string][] = [
  ["queue", "Queue"],
  ["archive", "Archive"],
  ["dlq", "DLQ"],
];
export const VIEW_STATUS: Record<View, InboxStatus> = {
  queue: "pending",
  archive: "processed",
  dlq: "dlq",
};
export const EMPTY_TEXT: Record<View, string> = {
  queue: "Queue is empty.",
  archive: "No archived messages.",
  dlq: "Dead-letter queue is empty.",
};

// Which optional labels one inbox row shows: a non-event kind badge, the retry
// count, the Archive-only processed line, and the details toggle.
export function messageRowFlags({ item, view }: { item: InboxItem; view: View }) {
  return {
    kind: item.kind && item.kind !== "event" ? item.kind : "",
    showAttempts: item.attempts > 1,
    showProcessed: view === "archive" && !!item.processed_at,
    hasDetails: !!item.subject || !!item.data,
  };
}

export type DialogMode = "clear" | "processed" | "reply";

// A pending Mark-processed / Reply / Clear-queue dialog.
export type DialogState =
  | { mode: "clear" }
  | { mode: "processed"; item: InboxItem }
  | { mode: "reply"; item: InboxItem }
  | null;

type DialogCopy = {
  title: string;
  description: string;
  placeholder: string;
  variant: "destructive" | "default";
  // The toast shown when the text is empty; "" when no text is needed.
  required: string;
};

const DIALOG_COPY: Record<DialogMode, DialogCopy> = {
  clear: {
    title: "Clear queue",
    description:
      "Every pending delivery for this agent is physically deleted and cannot be recovered. Archive and DLQ are not changed; shared messages needed by other agents are retained.",
    placeholder: "result",
    variant: "destructive",
    required: "",
  },
  processed: {
    title: "Mark processed",
    description: "A non-empty result is required.",
    placeholder: "result",
    variant: "default",
    required: "result is required",
  },
  reply: {
    title: "Reply",
    description: "Publish a reply — this also marks the message processed.",
    placeholder: "reply text",
    variant: "default",
    required: "reply text is required",
  },
};

// The dialog's copy by mode; a closed dialog keeps the processed copy, as the
// closing animation still renders it.
export function dialogCopy(mode: DialogMode | undefined): DialogCopy {
  return DIALOG_COPY[mode ?? "processed"];
}
