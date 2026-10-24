import { useState, type ReactNode } from "react";
import { toast } from "sonner";
import { useAgentName } from "@/lib/agent";
import { ApiError, type InboxItem } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import { EMPTY_TEXT, VIEWS, type View } from "@/pages/agentMessages/agentMessagesModel";
import MessageDialog from "@/pages/agentMessages/MessageDialog";
import MessageRow from "@/pages/agentMessages/MessageRow";
import { useAgentMessages } from "@/pages/agentMessages/useAgentMessages";
import { useMessageDialog } from "@/pages/agentMessages/useMessageDialog";

// Per-agent delivery queue (Chat → Queue): Queue | Archive | DLQ sub-views over
// the P5 inbox endpoints. `leading` is the caller's view switch, rendered at the
// start of the same 48px toolbar as the sub-views. All lists are newest-first
// (the backend orders them). Operator actions (mark-processed, reply, requeue)
// mutate then reload the active view; live refresh rides the SSE `message`
// stream with a poll fallback.
export default function AgentMessages({ leading }: { leading?: ReactNode } = {}) {
  const api = useApi();
  const name = useAgentName();
  const [view, setView] = useState<View>("queue");
  const { items, reload } = useAgentMessages(view);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const dialog = useMessageDialog(reload);

  const toggle = (id: string) =>
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  const requeue = async (item: InboxItem) => {
    try {
      await api.messages.agentInboxRequeue(name, item.id);
      toast.success("requeued");
      await reload();
    } catch (e) {
      toast.error(`requeue failed: ${e instanceof ApiError ? e.message : String(e)}`);
    }
  };

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-12 shrink-0 items-center gap-2 pr-3 pl-4">
        {leading}
        <ViewTabs view={view} onView={setView} />
        <span className="text-meta text-muted-foreground">
          {items.length} message{items.length === 1 ? "" : "s"}
        </span>
        {view === "queue" && items.length > 0 && (
          <Button
            size="sm"
            variant="outline"
            className="ml-auto"
            onClick={dialog.openClear}
            disabled={dialog.busy}
          >
            Clear queue
          </Button>
        )}
      </div>
      <div role="tabpanel" className="min-h-0 flex-1 overflow-auto border-t">
        {items.length === 0 && (
          <p className="px-4 py-3 text-sm text-muted-foreground">{EMPTY_TEXT[view]}</p>
        )}
        {items.map((it) => (
          <MessageRow
            key={it.id}
            item={it}
            view={view}
            expanded={expanded.has(it.id)}
            onToggle={() => toggle(it.id)}
            onProcess={() => dialog.openItem("processed", it)}
            onReply={() => dialog.openItem("reply", it)}
            onRequeue={() => void requeue(it)}
          />
        ))}
      </div>

      <MessageDialog state={dialog} />
    </div>
  );
}

function ViewTabs({ view, onView }: { view: View; onView: (v: View) => void }) {
  return (
    <div
      role="tablist"
      aria-label="Queue view"
      className="flex shrink-0 gap-0.5 rounded-panel bg-muted p-0.5"
    >
      {VIEWS.map(([key, label]) => (
        <button
          key={key}
          type="button"
          role="tab"
          aria-selected={view === key}
          onClick={() => onView(key)}
          className={`h-5.5 rounded-control px-2.25 text-meta ${
            view === key
              ? "bg-card font-medium text-foreground shadow-[var(--raise)]"
              : "text-muted-foreground hover:text-foreground"
          }`}
        >
          {label}
        </button>
      ))}
    </div>
  );
}
