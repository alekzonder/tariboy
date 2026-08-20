import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { useAgentName } from "@/lib/agent";
import {
  ApiError,
  agentInboxList,
  agentInboxProcessed,
  agentInboxReply,
  agentInboxRequeue,
  subscribeAgentEvents,
  type InboxItem,
  type InboxStatus,
} from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { fmtDateTime } from "@/lib/time";

// The three sub-views map onto the P5 inbox status filter. Queue is the default.
type View = "queue" | "archive" | "dlq";
const VIEW_STATUS: Record<View, InboxStatus> = {
  queue: "pending",
  archive: "processed",
  dlq: "dlq",
};

// A pending Mark-processed / Reply dialog, keyed to the row it acts on.
type DialogState = { mode: "processed" | "reply"; item: InboxItem } | null;

// One inbox row. Actions differ per view: Queue rows can be processed or
// replied to, DLQ rows requeued; Archive rows are read-only and additionally
// show their result + processed time.
function MessageRow({
  item,
  view,
  expanded,
  onToggle,
  onProcess,
  onReply,
  onRequeue,
}: {
  item: InboxItem;
  view: View;
  expanded: boolean;
  onToggle: () => void;
  onProcess: () => void;
  onReply: () => void;
  onRequeue: () => void;
}) {
  const kind = item.kind && item.kind !== "event" ? item.kind : "";
  const hasDetails = !!item.subject || !!item.data;
  return (
    <div className="border-b py-2 last:border-0">
      <div className="flex items-start gap-2">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            <span className="font-mono" title={item.ts}>{fmtDateTime(item.ts)}</span>
            <span>·</span>
            <span className="font-mono">{item.source || "?"}</span>
            <Badge variant="secondary">{item.type || "note"}</Badge>
            {kind && <Badge>{kind}</Badge>}
            {item.attempts > 1 && <span>attempts: {item.attempts}</span>}
            {item.in_reply_to && (
              <span className="font-mono">↩ {item.in_reply_to}</span>
            )}
          </div>
          <div className="mt-1 text-sm break-words whitespace-pre-wrap">{item.text}</div>
          {view === "archive" && item.processed_at && (
            <div className="mt-1 text-xs text-muted-foreground">
              <span className="font-mono" title={item.processed_at}>
                processed {fmtDateTime(item.processed_at)}
              </span>
              {item.result && <span className="ml-2">— {item.result}</span>}
            </div>
          )}
          {hasDetails && (
            <button
              onClick={onToggle}
              className="mt-1 text-xs text-muted-foreground underline-offset-2 hover:underline"
            >
              {expanded ? "hide details" : "show details"}
            </button>
          )}
          {expanded && hasDetails && (
            <pre className="mt-1 overflow-auto rounded border bg-muted/40 p-2 text-xs">
              {JSON.stringify({ subject: item.subject, data: item.data }, null, 2)}
            </pre>
          )}
        </div>
        <div className="flex shrink-0 gap-1">
          {view === "queue" && (
            <>
              <Button size="sm" variant="outline" onClick={onProcess}>Mark processed</Button>
              <Button size="sm" variant="outline" onClick={onReply}>Reply</Button>
            </>
          )}
          {view === "dlq" && (
            <Button size="sm" variant="outline" onClick={onRequeue}>Requeue</Button>
          )}
        </div>
      </div>
    </div>
  );
}

// Per-agent Messages tab: Queue | Archive | DLQ sub-views over the P5 inbox
// endpoints. All lists are newest-first (the backend orders them). Operator
// actions (mark-processed, reply, requeue) mutate then reload the active view;
// live refresh rides the SSE `message` stream with a poll fallback.
export default function AgentMessages() {
  const name = useAgentName();
  const [view, setView] = useState<View>("queue");
  const [items, setItems] = useState<InboxItem[]>([]);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [dialog, setDialog] = useState<DialogState>(null);
  const [dialogText, setDialogText] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(
    (v: View) =>
      agentInboxList(name, VIEW_STATUS[v])
        .then((r) => setItems(r.messages ?? []))
        .catch(() => { /* keep last rows on a transient failure */ }),
    [name],
  );

  // Reload the active view; refresh on SSE `message` events with a 3s poll
  // fallback (the SSE hub drops on a full buffer, so it is only a hint).
  useEffect(() => {
    if (!name) return;
    void load(view);
    const t = window.setInterval(() => void load(view), 3000);
    const off = subscribeAgentEvents(name, ["message"], () => void load(view));
    return () => { window.clearInterval(t); off(); };
  }, [name, view, load]);

  const toggle = (id: string) =>
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });

  const openDialog = (mode: "processed" | "reply", item: InboxItem) => {
    setDialogText("");
    setDialog({ mode, item });
  };

  const submitDialog = async () => {
    if (!dialog) return;
    const text = dialogText.trim();
    if (!text) {
      toast.error(dialog.mode === "processed" ? "result is required" : "reply text is required");
      return;
    }
    setBusy(true);
    try {
      if (dialog.mode === "processed") {
        await agentInboxProcessed(name, dialog.item.id, text);
        toast.success("marked processed");
      } else {
        await agentInboxReply(name, dialog.item.id, text);
        toast.success("replied");
      }
      setDialog(null);
      await load(view); // the row leaves Queue for Archive
    } catch (e) {
      toast.error(`failed: ${e instanceof ApiError ? e.message : String(e)}`);
    } finally {
      setBusy(false);
    }
  };

  const requeue = async (item: InboxItem) => {
    try {
      await agentInboxRequeue(name, item.id);
      toast.success("requeued");
      await load(view);
    } catch (e) {
      toast.error(`requeue failed: ${e instanceof ApiError ? e.message : String(e)}`);
    }
  };

  const empty = {
    queue: "Queue is empty.",
    archive: "No archived messages.",
    dlq: "Dead-letter queue is empty.",
  }[view];

  return (
    <div className="flex h-full flex-col gap-3">
      <Tabs value={view} onValueChange={(v) => setView(v as View)}>
        <TabsList>
          <TabsTrigger value="queue">Queue</TabsTrigger>
          <TabsTrigger value="archive">Archive</TabsTrigger>
          <TabsTrigger value="dlq">DLQ</TabsTrigger>
        </TabsList>
        {(["queue", "archive", "dlq"] as View[]).map((v) => (
          <TabsContent key={v} value={v}>
            <div className="rounded border bg-muted/20 p-2">
              {items.length === 0 && (
                <p className="p-2 text-sm text-muted-foreground">{empty}</p>
              )}
              {items.map((it) => (
                <MessageRow
                  key={it.id}
                  item={it}
                  view={v}
                  expanded={expanded.has(it.id)}
                  onToggle={() => toggle(it.id)}
                  onProcess={() => openDialog("processed", it)}
                  onReply={() => openDialog("reply", it)}
                  onRequeue={() => void requeue(it)}
                />
              ))}
            </div>
          </TabsContent>
        ))}
      </Tabs>

      <Dialog open={dialog !== null} onOpenChange={(o) => { if (!o) setDialog(null); }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{dialog?.mode === "reply" ? "Reply" : "Mark processed"}</DialogTitle>
            <DialogDescription>
              {dialog?.mode === "reply"
                ? "Publish a reply — this also marks the message processed."
                : "A non-empty result is required."}
            </DialogDescription>
          </DialogHeader>
          <Textarea
            autoFocus
            value={dialogText}
            onChange={(e) => setDialogText(e.target.value)}
            placeholder={dialog?.mode === "reply" ? "reply text" : "result"}
            rows={4}
          />
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialog(null)} disabled={busy}>Cancel</Button>
            <Button onClick={() => void submitDialog()} disabled={busy}>
              {dialog?.mode === "reply" ? "Reply" : "Mark processed"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
