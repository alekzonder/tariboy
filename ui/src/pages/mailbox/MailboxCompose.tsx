import { useId, useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import type { Letter } from "@/types";
import { cn } from "@/lib/utils";
import { instantAllowed, principalName, type MailboxMode } from "./mailboxModel";

export type ComposeTarget = { mode: "compose" } | { mode: "reply"; letter: Letter };

export interface ComposeValues {
  to: string;
  subject: string;
  text: string;
  /** No reply needed: the letter is a notice, not a request. */
  notice: boolean;
  /** Deliver to the agent's running iteration after its current tool call. */
  instant: boolean;
}

/** The daemon puts at most this many code points of an instant letter into
 *  the running iteration (`maxInstantBodyRunes`). */
const INSTANT_LIMIT = 8000;
const WARN = "text-amber-700 dark:text-amber-400";
const count = (value: number) => value.toLocaleString("en-US");

function InstantSwitch({
  checked,
  onChange,
}: {
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  const hint = useId();
  return (
    <div className="flex flex-col gap-1">
      <Label className="flex items-center gap-2">
        <Switch
          checked={checked}
          onCheckedChange={onChange}
          aria-label="Deliver to the current iteration"
          aria-describedby={hint}
        />
        Deliver to the current iteration
      </Label>
      <p id={hint} className="text-meta text-muted-foreground">
        The agent sees this letter after its current tool call. If no iteration is running, the
        letter starts one, like any other letter.
      </p>
    </div>
  );
}

/** `N / 8,000` under the text, and a warning over the limit. The warning lives
 *  in its own polite live region, so it is announced once on crossing the
 *  limit and not on every character. */
function InstantCounter({ id, length }: { id: string; length: number }) {
  const over = length > INSTANT_LIMIT;
  return (
    <div className="-mt-2 flex flex-col gap-1">
      <p id={id} className={cn("self-end text-meta", over ? WARN : "text-muted-foreground")}>
        {count(length)} / {count(INSTANT_LIMIT)}
      </p>
      <p data-testid="instant-limit" aria-live="polite" className={cn("text-meta", WARN)}>
        {over &&
          "Over 8,000 characters. The agent sees the first 8,000 in this iteration and reads the full letter with tmail show."}
      </p>
    </div>
  );
}

/** The letter's text; `counted` adds the instant counter, which counts the
 *  trimmed text that is sent. */
function LetterText({
  text,
  onText,
  counted,
}: {
  text: string;
  onText: (text: string) => void;
  counted: boolean;
}) {
  const counter = useId();
  return (
    <>
      <Textarea
        autoFocus
        aria-label="Text"
        // A long letter scrolls, so the counter and the warning stay in view.
        className="max-h-60"
        aria-describedby={counted ? counter : undefined}
        value={text}
        onChange={(event) => onText(event.target.value)}
        rows={5}
      />
      {counted && <InstantCounter id={counter} length={[...text.trim()].length} />}
    </>
  );
}

/** To and Subject of a new letter; `fixedTo` replaces the recipient picker. */
function LetterHeaderFields({
  to,
  onTo,
  fixedTo,
  recipients,
  subject,
  onSubject,
}: {
  to: string;
  onTo: (to: string) => void;
  fixedTo?: string;
  recipients: readonly string[];
  subject: string;
  onSubject: (subject: string) => void;
}) {
  return (
    <div className="flex flex-col gap-3">
      {fixedTo ? (
        <p className="text-sm">To: {principalName(fixedTo)}</p>
      ) : (
        <Label className="flex flex-col items-start gap-1">
          To
          <select
            value={to}
            onChange={(event) => onTo(event.target.value)}
            className="h-8 w-full rounded border bg-background px-2 text-sm"
          >
            {recipients.map((principal) => (
              <option key={principal} value={principal}>
                {principalName(principal)}
              </option>
            ))}
          </select>
        </Label>
      )}
      <Label className="flex flex-col items-start gap-1">
        Subject
        <Input value={subject} onChange={(event) => onSubject(event.target.value)} />
      </Label>
    </div>
  );
}

/** A new letter, or a reply to one. A new letter picks its recipient unless
 *  `fixedTo` names it; the switches make it a notice and deliver it to the
 *  agent's running iteration. */
export function MailboxCompose({
  target,
  mode,
  fixedTo,
  recipients,
  busy,
  onClose,
  onSubmit,
}: {
  target: ComposeTarget;
  mode: MailboxMode;
  fixedTo?: string;
  /** Principals a new letter can go to, e.g. `agent:dev`. */
  recipients: readonly string[];
  busy: boolean;
  onClose: () => void;
  onSubmit: (values: ComposeValues) => void;
}) {
  const reply = target.mode === "reply" ? target.letter : null;
  const [to, setTo] = useState(fixedTo ?? recipients[0] ?? "");
  const [subject, setSubject] = useState("");
  const [text, setText] = useState("");
  const [notice, setNotice] = useState(false);
  const [instantOn, setInstant] = useState(false);
  const offerInstant = instantAllowed(to, reply, mode);
  const instant = offerInstant && instantOn;
  const ready = text.trim() !== "" && (reply !== null || to !== "");
  const title = reply ? `Reply to ${principalName(reply.from)}` : "New letter";

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            {reply
              ? `Re: ${reply.subject || "(no subject)"}`
              : "A request waits for a reply; a notice needs none."}
          </DialogDescription>
        </DialogHeader>
        {!reply && (
          <LetterHeaderFields
            to={to}
            onTo={setTo}
            fixedTo={fixedTo}
            recipients={recipients}
            subject={subject}
            onSubject={setSubject}
          />
        )}
        <LetterText text={text} onText={setText} counted={instant} />
        {!reply && (
          <Label className="flex items-center gap-2">
            <Switch checked={notice} onCheckedChange={setNotice} aria-label="No reply needed" />
            No reply needed
          </Label>
        )}
        {offerInstant && <InstantSwitch checked={instantOn} onChange={setInstant} />}
        <DialogFooter>
          <Button variant="outline" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button
            onClick={() =>
              onSubmit({ to, subject: subject.trim(), text: text.trim(), notice, instant })
            }
            disabled={busy || !ready}
          >
            {reply ? "Send reply" : "Send"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
