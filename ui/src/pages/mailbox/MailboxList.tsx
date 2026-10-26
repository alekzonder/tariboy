import type { Letter, MailBox } from "@/lib/api";
import { cn } from "@/lib/utils";
import { formatTaskTime } from "@/pages/tasks/taskTime";
import { principalName } from "./mailboxModel";

const EMPTY: Record<MailBox, string> = {
  inbox: "No open letters.",
  sent: "No letters sent.",
  archive: "No archived letters.",
};

function LetterTags({ letter }: { letter: Letter }) {
  return (
    <>
      {letter.kind !== "message" && (
        <span className="rounded-control bg-muted px-1.5 text-caption text-muted-foreground">
          {letter.kind}
        </span>
      )}
      {!letter.needs_reply && (
        <span className="rounded-control bg-muted px-1.5 text-caption text-muted-foreground">
          notice
        </span>
      )}
      {letter.dead_at && (
        <span className="rounded-control bg-destructive/12 px-1.5 text-caption text-destructive">
          dead · {letter.dead_reason}
        </span>
      )}
    </>
  );
}

/** One row per letter: who (the recipient in Sent), subject, tags, preview, time. */
export function MailboxList({
  box,
  letters,
  selected,
  onSelect,
}: {
  box: MailBox;
  letters: readonly Letter[];
  selected: string;
  onSelect: (letter: Letter) => void;
}) {
  if (letters.length === 0) {
    return <p className="px-4 py-3 text-sm text-muted-foreground">{EMPTY[box]}</p>;
  }
  return (
    <ul aria-label="Letters" className="divide-y">
      {letters.map((letter) => {
        const unread = box === "inbox" && !letter.read_at;
        return (
          <li key={letter.id}>
            <button
              type="button"
              aria-current={letter.id === selected || undefined}
              onClick={() => onSelect(letter)}
              className={cn(
                "flex w-full flex-col gap-0.5 px-4 py-2 text-left hover:bg-accent",
                letter.id === selected && "bg-accent",
              )}
            >
              <span className="flex items-center gap-1.5 text-ui">
                {unread && (
                  <span
                    role="img"
                    aria-label="Unread"
                    className="size-1.5 shrink-0 rounded-full bg-primary"
                  />
                )}
                <span className={cn("truncate", unread && "font-medium")}>
                  {principalName(box === "sent" ? letter.to : letter.from)}
                </span>
                <span className="ml-auto shrink-0 text-caption text-muted-foreground">
                  {formatTaskTime(letter.created_at)}
                </span>
              </span>
              <span className="flex min-w-0 items-center gap-1.5 text-ui">
                <span className="truncate">{letter.subject || "(no subject)"}</span>
                <LetterTags letter={letter} />
              </span>
              <span className="truncate text-meta text-muted-foreground">{letter.body}</span>
            </button>
          </li>
        );
      })}
    </ul>
  );
}
