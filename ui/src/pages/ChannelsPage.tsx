import { useCallback, useEffect, useState, type ReactNode } from "react";
import { useParams } from "react-router-dom";
import { toast } from "sonner";
import {
  ApiError,
  type ChannelList,
  type ChannelMessage,
  type ChannelRow,
  type ChannelTail,
  type ChannelWatch,
  type ChannelWatchList,
} from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { AgentSubscriptions } from "@/components/AgentSubscriptions";
import { useAgentName } from "@/lib/agent";

const NO_MSGS: ChannelMessage[] = [];
const NO_WATCHES: ChannelWatch[] = [];

export default function ChannelsPage({ leading }: { leading?: ReactNode } = {}) {
  // Under /agent/:name/channels the left column is the agent's manageable
  // subscriptions (AgentSubscriptions); the global mount (no :name) keeps the
  // browse-all-channels + tail/send panel. messenger chat-routes (bind/create) live
  // on the Plugins tab now — they are plugin-global config, not per-agent.
  const api = useApi();
  const { name: routeName } = useParams<{ name?: string }>();
  const contextName = useAgentName();
  const name = routeName || contextName || undefined;
  const [channels, setChannels] = useState<ChannelRow[]>([]);
  const [sel, setSel] = useState<string | null>(null);
  // Loaded data carries its channel; another channel's data never renders.
  const [tail, setTail] = useState<{ ch: string; messages: ChannelMessage[] } | null>(null);
  const [watchList, setWatchList] = useState<{ ch: string; watches: ChannelWatch[] } | null>(null);
  const msgs = tail && tail.ch === sel ? tail.messages : NO_MSGS;
  const watches = watchList && watchList.ch === sel ? watchList.watches : NO_WATCHES;
  const [text, setText] = useState("");
  const [type, setType] = useState("note");

  const fetchTail = useCallback(
    (ch: string) =>
      api.get<ChannelTail>(`/api/channels/${encodeURIComponent(ch)}/messages`).then(
        (r) => r.messages,
        () => NO_MSGS,
      ),
    [api],
  );
  const fetchWatches = useCallback(
    (ch: string) =>
      api.get<ChannelWatchList>(`/api/channels/${encodeURIComponent(ch)}/watches`).then(
        (r) => r.watches ?? NO_WATCHES,
        () => NO_WATCHES,
      ),
    [api],
  );

  // The global channel list backs the browse panel; under an agent the left
  // column is AgentSubscriptions, so we only fetch here when unscoped.
  useEffect(() => {
    if (name) return;
    api
      .get<ChannelList>("/api/channels")
      .then((r) => {
        setChannels(r.channels);
        setSel(null);
      })
      .catch(() => setChannels([]));
  }, [api, name]);
  useEffect(() => {
    if (!sel) return;
    let live = true;
    const loadTail = () =>
      void fetchTail(sel).then((messages) => {
        if (live) setTail({ ch: sel, messages });
      });
    loadTail();
    void fetchWatches(sel).then((list) => {
      if (live) setWatchList({ ch: sel, watches: list });
    });
    const t = window.setInterval(loadTail, 2000);
    return () => {
      live = false;
      window.clearInterval(t);
    };
  }, [fetchTail, fetchWatches, sel]);

  const send = async () => {
    if (!sel) return;
    try {
      await api.post("/api/messages", { channel: sel, type, text });
      setText("");
      toast.success("sent");
      void fetchTail(sel).then((messages) => setTail({ ch: sel, messages }));
    } catch (e) {
      toast.error(`send failed: ${e instanceof ApiError ? e.message : String(e)}`);
    }
  };

  return (
    <div className="flex h-full min-h-0 flex-col">
      {/* One 48px control row, like the chat's: the caller's view switch, then
          the selected channel and how many provider watches feed it. */}
      <div className="flex h-12 shrink-0 items-center gap-2 pr-3 pl-4">
        {leading}
        {sel ? (
          <>
            <span className="truncate font-mono text-body-sm font-medium">{sel}</span>
            <span className="shrink-0 text-meta text-muted-foreground">
              {watches.length} watch{watches.length === 1 ? "" : "es"} · refreshed every 2s
            </span>
          </>
        ) : (
          <span className="text-meta text-muted-foreground">Select a channel.</span>
        )}
      </div>
      <div className="flex min-h-0 flex-1 border-t">
        {name ? (
          <AgentSubscriptions name={name} selected={sel} onSelect={setSel} />
        ) : (
          <div className="w-2/5 min-w-[12rem] shrink-0 overflow-auto">
            <div className="p-2 text-xs font-medium text-muted-foreground">CHANNELS</div>
            {channels.map((c) => (
              <button
                key={c.name}
                onClick={() => setSel(c.name)}
                className={cn(
                  "flex w-full items-center justify-between px-2 py-1.5 text-left text-sm hover:bg-accent",
                  sel === c.name && "bg-accent",
                )}
              >
                <span className="truncate font-mono text-xs">{c.name}</span>
                <Badge variant="secondary">{c.kind}</Badge>
              </button>
            ))}
            {channels.length === 0 && (
              <p className="p-2 text-sm text-muted-foreground">No channels.</p>
            )}
          </div>
        )}
        {sel && (
          <div className="flex min-w-0 flex-1 flex-col border-l">
            <TailMessages msgs={msgs} />
            <Watches watches={watches} />
            <div className="flex gap-2 border-t px-3 py-2.5">
              <Input
                value={type}
                onChange={(e) => setType(e.target.value)}
                className="h-8 w-28 font-mono"
                placeholder="type"
              />
              <Input
                value={text}
                onChange={(e) => setText(e.target.value)}
                className="h-8 flex-1"
                placeholder="message text"
                onKeyDown={(e) => {
                  if (e.key === "Enter") void send();
                }}
              />
              <Button size="sm" onClick={() => void send()}>
                Send
              </Button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

// TailMessages is the selected channel's recent messages, oldest first.
function TailMessages({ msgs }: { msgs: ChannelMessage[] }) {
  return (
    <div className="min-h-0 flex-1 overflow-auto py-1">
      {msgs.map((m) => (
        <div
          key={m.id}
          className="grid grid-cols-[4.5rem_minmax(0,10rem)_minmax(0,1fr)] gap-2.5 px-4 py-1.5 text-body-sm"
        >
          <span className="font-mono text-meta text-muted-foreground tabular-nums" title={m.ts}>
            {m.ts ? fmtTime(m.ts) : ""}
          </span>
          <span className="truncate font-mono text-meta text-muted-foreground">
            {m.source} · {m.type}
          </span>
          <span className="break-words whitespace-pre-wrap">{m.text}</span>
        </div>
      ))}
      {msgs.length === 0 && <p className="px-4 py-2 text-sm text-muted-foreground">No messages.</p>}
    </div>
  );
}

// fmtTime is the wall-clock time of a tail row; the full timestamp is its title.
function fmtTime(iso: string): string {
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? iso : date.toLocaleTimeString([], { hour12: false });
}

// Watches renders a channel's distinct provider watches (§8.1/§10, Phase R):
// each watch's identity, its params pretty-printed, and the subscribed agents.
// Read-only — this is an inspect view, not an editor — and collapsible so the
// tail keeps the height when a channel has many watches.
function Watches({ watches }: { watches: ChannelWatch[] }) {
  const [open, setOpen] = useState(true);
  return (
    <div className="shrink-0 border-t px-4 py-2 text-sm">
      <div className="flex items-center gap-2">
        <span className="font-medium">Watches</span>
        <Badge variant="secondary">{watches.length}</Badge>
        <button
          type="button"
          onClick={() => setOpen(!open)}
          className="ml-auto text-xs text-muted-foreground underline-offset-2 hover:underline"
        >
          {open ? "hide" : "show"}
        </button>
      </div>
      {open &&
        (watches.length === 0 ? (
          <p className="mt-1 text-sm text-muted-foreground">No watches on this channel.</p>
        ) : (
          <ul className="mt-1 max-h-[22vh] space-y-2 overflow-auto">
            {watches.map((w) => (
              <li key={w.watch} className="py-1">
                <div className="flex flex-wrap items-center gap-1">
                  <span className="mr-1 font-mono text-xs">{w.watch}</span>
                  {(w.subscribers ?? []).map((s) => (
                    <Badge key={s} variant="secondary" className="font-mono text-xs">
                      {s}
                    </Badge>
                  ))}
                  {(w.subscribers ?? []).length === 0 && (
                    <span className="text-xs text-muted-foreground">no subscribers</span>
                  )}
                </div>
                {w.params && Object.keys(w.params).length > 0 && (
                  <pre className="mt-1 overflow-x-auto rounded bg-muted/50 p-1 text-xs">
                    {JSON.stringify(w.params, null, 2)}
                  </pre>
                )}
              </li>
            ))}
          </ul>
        ))}
    </div>
  );
}
