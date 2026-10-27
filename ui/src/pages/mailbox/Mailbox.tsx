import { lazy, useState, type ReactNode } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { PenLine } from "lucide-react";
import { useApi } from "@/components/apiContext";
import { LazyBoundary } from "@/components/LazyBoundary";
import { Button } from "@/components/ui/button";
import { Segmented } from "@/components/ui/segmented";
import { useApiQuery } from "@/hooks/useApiQuery";
import { DRAWER_QUERY, useMediaQuery } from "@/hooks/useMediaQuery";
import type { Letter, MailBox } from "@/lib/api";
import { cn } from "@/lib/utils";
import { MailboxCompose, type ComposeTarget, type ComposeValues } from "./MailboxCompose";
import { MailboxLetter } from "./MailboxLetter";
import { MailboxList } from "./MailboxList";
import { BOXES, marksReadOnOpen, type MailboxMode } from "./mailboxModel";
import { mailQueries, useLiveMail } from "./mailQueries";
import { useMailboxActions } from "./useMailboxActions";

const TaskDrawer = lazy(() => import("@/pages/tasks/TaskDrawer"));

export interface MailboxProps {
  /** The principal whose mailbox this is; "" is the customer. */
  as: string;
  mode: MailboxMode;
  /** A new letter goes here, with no recipient picker. */
  fixedTo?: string;
  /** Principals the recipient picker offers. */
  recipients?: readonly string[];
  /** Controls at the end of the toolbar, e.g. the agent's counts. */
  toolbar?: ReactNode;
}

function MailboxToolbar({
  box,
  onBox,
  openCount,
  onCompose,
  children,
}: {
  box: MailBox;
  onBox: (box: MailBox) => void;
  openCount: number;
  onCompose: () => void;
  children?: ReactNode;
}) {
  const options = BOXES.map((option) =>
    option.value === "inbox" && openCount > 0
      ? { ...option, label: `${option.label} ${openCount}` }
      : option,
  );
  return (
    <div className="flex h-12 shrink-0 items-center gap-2 pr-3 pl-4">
      <Segmented label="Mailbox" value={box} onChange={onBox} options={options} />
      <div className="ml-auto flex items-center gap-2">
        {children}
        <Button size="sm" onClick={onCompose}>
          <PenLine className="size-3.5" aria-hidden />
          Compose
        </Button>
      </div>
    </div>
  );
}

/** The left pane: one box of `as`, paged back with Load more. */
function MailboxListPane({
  as,
  box,
  narrow,
  selected,
  onSelect,
}: {
  as: string;
  box: MailBox;
  narrow: boolean;
  selected: Letter | null;
  onSelect: (letter: Letter) => void;
}) {
  const api = useApi();
  const list = useInfiniteQuery(mailQueries.box(api, as, box));
  const letters = list.data?.pages.flatMap((page) => page.letters) ?? [];
  return (
    <section
      aria-label="Letter list"
      className={cn("min-h-0 overflow-auto", narrow ? "flex-1" : "w-80 shrink-0 border-r")}
    >
      <MailboxList box={box} letters={letters} selected={selected?.id ?? ""} onSelect={onSelect} />
      {list.hasNextPage && (
        <Button
          size="sm"
          variant="ghost"
          className="m-2"
          disabled={list.isFetchingNextPage}
          onClick={() => void list.fetchNextPage()}
        >
          Load more
        </Button>
      )}
    </section>
  );
}

/** The opened letter as the box lists it now, or as it was opened once it
 *  left the box (a read notice moves to Archive). */
function useCurrentLetter(as: string, box: MailBox, opened: Letter | null): Letter | null {
  const api = useApi();
  const list = useInfiniteQuery(mailQueries.box(api, as, box));
  if (!opened) return null;
  const listed = list.data?.pages.flatMap((page) => page.letters);
  return listed?.find((letter) => letter.id === opened.id) ?? opened;
}

/**
 * One mailbox: Inbox, Sent and Archive of `as`, a list on the left and the
 * selected letter on the right. On a narrow screen only one pane shows.
 */
export default function Mailbox({ as, mode, fixedTo, recipients = [], toolbar }: MailboxProps) {
  const api = useApi();
  const narrow = useMediaQuery(DRAWER_QUERY);
  const [box, setBox] = useState<MailBox>("inbox");
  const [opened, setOpened] = useState<Letter | null>(null);
  const [compose, setCompose] = useState<ComposeTarget | null>(null);
  const [taskKey, setTaskKey] = useState("");
  const refresh = useLiveMail(as);
  const actions = useMailboxActions(as, refresh);
  const openCount = useApiQuery(mailQueries.unread(api, as)).data?.open ?? 0;
  const current = useCurrentLetter(as, box, opened);

  const open = (letter: Letter) => {
    setOpened(letter);
    if (marksReadOnOpen(letter, box)) void actions.read(letter);
  };
  const submit = async (values: ComposeValues) => {
    const done =
      compose?.mode === "reply"
        ? await actions.reply(compose.letter, values.text)
        : await actions.send(values);
    if (done) setCompose(null);
  };

  return (
    <div className="flex h-full min-h-0 flex-col">
      <MailboxToolbar
        box={box}
        onBox={(next) => {
          setBox(next);
          setOpened(null);
        }}
        openCount={openCount}
        onCompose={() => setCompose({ mode: "compose" })}
      >
        {toolbar}
      </MailboxToolbar>
      <div className="flex min-h-0 flex-1 border-t">
        {(!narrow || !current) && (
          <MailboxListPane as={as} box={box} narrow={narrow} selected={current} onSelect={open} />
        )}
        {(!narrow || current) && (
          <section aria-label="Letter" className="min-h-0 min-w-0 flex-1 overflow-auto">
            {current ? (
              <MailboxLetter
                letter={current}
                box={box}
                mode={mode}
                as={as}
                onBack={narrow ? () => setOpened(null) : undefined}
                handlers={{
                  busy: actions.busy,
                  onReply: (letter) => setCompose({ mode: "reply", letter }),
                  onArchive: (letter) => void actions.archive(letter),
                  onRequeue: (letter) => void actions.requeue(letter),
                  onOpenTask: setTaskKey,
                }}
              />
            ) : (
              <p className="px-4 py-3 text-sm text-muted-foreground">Select a letter.</p>
            )}
          </section>
        )}
      </div>
      {compose && (
        <MailboxCompose
          target={compose}
          fixedTo={fixedTo}
          recipients={recipients}
          busy={actions.busy}
          onClose={() => setCompose(null)}
          onSubmit={(values) => void submit(values)}
        />
      )}
      {taskKey && (
        <LazyBoundary resetKey={taskKey}>
          <TaskDrawer key={taskKey} taskKey={taskKey} onClose={() => setTaskKey("")} />
        </LazyBoundary>
      )}
    </div>
  );
}
