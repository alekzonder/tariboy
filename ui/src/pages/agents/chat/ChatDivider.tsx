/** The day rule between two calendar days of a conversation. */
export function DayDivider({ label }: { label: string }) {
  return (
    <div className="flex items-center gap-2.5 px-4 pt-2.5 pb-1.5">
      <span className="h-px flex-1 bg-border" />
      <span className="text-[10.5px] font-medium tracking-wider text-muted-foreground uppercase">{label}</span>
      <span className="h-px flex-1 bg-border" />
    </div>
  );
}

/** The unread rule. It is anchored to the read mark the chat was opened at, so
 *  it holds its place while the customer reads; only an explicit **Mark read**,
 *  sending a message, or leaving the chat takes it away. */
export function UnreadDivider({ label, onMarkRead }: { label: string; onMarkRead: () => void }) {
  return (
    <div data-testid="chat-unread-divider" className="flex items-center gap-2 px-4 pt-2 pb-1.5">
      <span className="inline-flex h-[18px] items-center rounded-full bg-primary px-2 text-[10.5px] font-semibold text-primary-foreground">
        {label}
      </span>
      <span className="h-px flex-1 bg-primary/45" />
      <button
        type="button"
        onClick={onMarkRead}
        className="h-5 rounded-[6px] px-1.5 text-[11px] text-primary hover:bg-primary/12"
      >
        Mark read
      </button>
    </div>
  );
}
