import { useState } from "react";
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
import type { Letter } from "@/lib/api";
import { principalName } from "./mailboxModel";

export type ComposeTarget = { mode: "compose" } | { mode: "reply"; letter: Letter };

export interface ComposeValues {
  to: string;
  subject: string;
  text: string;
  /** No reply needed: the letter is a notice, not a request. */
  notice: boolean;
}

/** A new letter, or a reply to one. A new letter picks its recipient unless
 *  `fixedTo` names it; the switch makes it a notice. */
export function MailboxCompose({
  target,
  fixedTo,
  recipients,
  busy,
  onClose,
  onSubmit,
}: {
  target: ComposeTarget;
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
          <div className="flex flex-col gap-3">
            {fixedTo ? (
              <p className="text-sm">To: {principalName(fixedTo)}</p>
            ) : (
              <Label className="flex flex-col items-start gap-1">
                To
                <select
                  value={to}
                  onChange={(event) => setTo(event.target.value)}
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
              <Input value={subject} onChange={(event) => setSubject(event.target.value)} />
            </Label>
          </div>
        )}
        <Textarea
          autoFocus
          aria-label="Text"
          value={text}
          onChange={(event) => setText(event.target.value)}
          rows={5}
        />
        {!reply && (
          <Label className="flex items-center gap-2">
            <Switch checked={notice} onCheckedChange={setNotice} aria-label="No reply needed" />
            No reply needed
          </Label>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button
            onClick={() => onSubmit({ to, subject: subject.trim(), text: text.trim(), notice })}
            disabled={busy || !ready}
          >
            {reply ? "Send reply" : "Send"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
