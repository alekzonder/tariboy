import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { useAgentName } from "@/lib/agent";
import {
  ApiError, chatMessagesOn, chatReadOn, messageSendOn, type ChatMessage,
} from "@/lib/api";
import { useMessagesSocket } from "@/hooks/useMessagesSocket";
import { targetFor } from "@/lib/terminalsHost";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { fmtDateTime } from "@/lib/time";
import { cn } from "@/lib/utils";
import {
  DEFAULT_CHAT_TYPES, EXTRA_CHAT_TYPES, loadChatTypes, saveChatTypes,
} from "./chatTypes";

const ALL_TYPES = [...DEFAULT_CHAT_TYPES, ...EXTRA_CHAT_TYPES];

/** One message, sided by who sent it: the customer on the right, the agent on
 *  the left, the same way a chat reads. */
function Bubble({ message, mine }: { message: ChatMessage; mine: boolean }) {
  return (
    <div className={cn("flex", mine ? "justify-end" : "justify-start")}>
      <div className={cn(
        "max-w-[80%] rounded-lg px-3 py-2",
        mine ? "bg-primary/10" : "bg-muted/60",
      )}>
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <span className="font-mono">{message.from}</span>
          <span title={message.ts}>{fmtDateTime(message.ts)}</span>
          <Badge variant="secondary">{message.type || "note"}</Badge>
        </div>
        <div className="mt-1 text-sm break-words whitespace-pre-wrap">{message.text}</div>
      </div>
    </div>
  );
}

/**
 * The conversation with one agent: the customer's messages in that agent's
 * inbox merged with the agent's messages in the customer's own channel. The
 * daemon owns that projection; this view renders it, sends into it, and marks
 * it read. Live updates arrive over the host's message socket, which is a hint
 * to refetch — the HTTP response stays authoritative.
 */
export default function AgentChat({ hostId = "" }: { hostId?: string }) {
  const name = useAgentName();
  const target = targetFor(hostId);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [customer, setCustomer] = useState("user:customer");
  const [types, setTypes] = useState<string[]>(() => loadChatTypes());
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const [showFilter, setShowFilter] = useState(false);
  const bottom = useRef<HTMLDivElement | null>(null);
  const typesKey = types.join(",");

  const load = useCallback(async () => {
    if (!name) return;
    try {
      const page = await chatMessagesOn(target, name, { types });
      setMessages(page.messages ?? []);
      setCustomer(page.customer || "user:customer");
      const newest = page.messages?.[page.messages.length - 1];
      // The mark is the timestamp of a message actually shown, never "now", so
      // one arriving mid-render cannot be marked read without being seen. A
      // hidden tab is not "shown": a chat left open behind another window keeps
      // counting its messages as unread.
      if (newest && document.visibilityState !== "hidden") {
        await chatReadOn(target, name, newest.ts);
      }
    } catch {
      // Keep the last conversation on a transient failure.
    }
    // target is derived from hostId and typesKey stands for the type list; both
    // change identity on every render otherwise.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [name, hostId, typesKey]);

  useEffect(() => { void load(); }, [load]);
  useMessagesSocket({
    target,
    enabled: (!target || Boolean(target.baseURL)) && Boolean(name),
    onHint: (hint) => { if (hint.agent === name) void load(); },
    onOpen: () => { void load(); },
  });
  useEffect(() => { bottom.current?.scrollIntoView({ block: "end" }); }, [messages]);

  const toggleType = (type: string) => {
    const next = types.includes(type) ? types.filter((t) => t !== type) : [...types, type];
    if (next.length === 0) return;
    setTypes(next);
    saveChatTypes(next);
  };
  const resetTypes = () => {
    setTypes([...DEFAULT_CHAT_TYPES]);
    saveChatTypes([...DEFAULT_CHAT_TYPES]);
  };

  const send = async () => {
    const text = draft.trim();
    if (!text || sending) return;
    setSending(true);
    try {
      // reply_to is what keeps the agent's reply in this chat instead of in its
      // own inbox.
      await messageSendOn(target, {
        channel: `agent:${name}:inbox`, type: "message", text, reply_to: customer,
      });
      setDraft("");
      await load();
    } catch (e) {
      toast.error(`send failed: ${e instanceof ApiError ? e.message : String(e)}`);
    } finally {
      setSending(false);
    }
  };

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <Button size="sm" variant="outline" onClick={() => setShowFilter((open) => !open)}>
          {showFilter ? "Hide types" : "Message types"}
        </Button>
        <span className="text-xs text-muted-foreground">{types.length} of {ALL_TYPES.length} types shown</span>
      </div>
      {showFilter && (
        <div className="flex flex-wrap items-center gap-1.5 rounded-md border p-2">
          {ALL_TYPES.map((type) => (
            <button
              key={type}
              type="button"
              aria-pressed={types.includes(type)}
              onClick={() => toggleType(type)}
              className={cn(
                "rounded-full border px-2 py-0.5 font-mono text-xs",
                types.includes(type) ? "bg-primary/10 border-primary/40" : "text-muted-foreground",
              )}
            >
              {type}
            </button>
          ))}
          <Button size="sm" variant="ghost" onClick={resetTypes}>Reset</Button>
        </div>
      )}
      <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-auto rounded-md border p-3">
        {messages.length === 0 && <p className="text-sm text-muted-foreground">No messages in this chat yet.</p>}
        {messages.map((message) => (
          <Bubble key={message.id} message={message} mine={message.from === customer} />
        ))}
        <div ref={bottom} />
      </div>
      <div className="flex shrink-0 items-end gap-2">
        <Textarea
          aria-label={`Message ${name}`}
          value={draft}
          rows={2}
          placeholder={`Message ${name}…`}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) { e.preventDefault(); void send(); }
          }}
        />
        <Button onClick={() => void send()} disabled={sending || !draft.trim()}>Send</Button>
      </div>
    </div>
  );
}
