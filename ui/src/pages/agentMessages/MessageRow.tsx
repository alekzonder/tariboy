import type { InboxMessage } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { fmtDateTime } from "@/lib/time";
import { messageRowFlags, type View } from "./agentMessagesModel";

type RowActions = {
  onProcess: () => void;
  onReply: () => void;
  onRequeue: () => void;
};

// One inbox row. Actions differ per view: Queue rows can be processed or
// replied to, DLQ rows requeued; Archive rows are read-only and additionally
// show their result + processed time.
export default function MessageRow({
  item,
  view,
  expanded,
  onToggle,
  ...actions
}: {
  item: InboxMessage;
  view: View;
  expanded: boolean;
  onToggle: () => void;
} & RowActions) {
  const { kind, showAttempts, showProcessed, hasDetails } = messageRowFlags({ item, view });
  return (
    <div className="border-b px-4 py-2 last:border-0">
      <div className="flex items-start gap-2">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            <span className="font-mono" title={item.ts}>
              {fmtDateTime(item.ts)}
            </span>
            <span>·</span>
            <span className="font-mono">{item.source || "?"}</span>
            <Badge variant="secondary">{item.type || "note"}</Badge>
            {kind && <Badge>{kind}</Badge>}
            {showAttempts && <span>attempts: {item.attempts}</span>}
            {item.in_reply_to && <span className="font-mono">↩ {item.in_reply_to}</span>}
          </div>
          <div className="mt-1 text-sm break-words whitespace-pre-wrap">{item.text}</div>
          {showProcessed && (
            <div className="mt-1 text-xs text-muted-foreground">
              <span className="font-mono" title={item.processed_at}>
                processed {fmtDateTime(item.processed_at ?? "")}
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
        <RowButtons view={view} {...actions} />
      </div>
    </div>
  );
}

function RowButtons({ view, onProcess, onReply, onRequeue }: { view: View } & RowActions) {
  return (
    <div className="flex shrink-0 gap-1">
      {view === "queue" && (
        <>
          <Button size="sm" variant="outline" onClick={onProcess}>
            Mark processed
          </Button>
          <Button size="sm" variant="outline" onClick={onReply}>
            Reply
          </Button>
        </>
      )}
      {view === "dlq" && (
        <Button size="sm" variant="outline" onClick={onRequeue}>
          Requeue
        </Button>
      )}
    </div>
  );
}
