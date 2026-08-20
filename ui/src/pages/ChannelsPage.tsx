import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import { toast } from "sonner";
import { apiGet, apiPost, ApiError } from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { ScrollArea } from "@/components/ui/scroll-area";
import { cn } from "@/lib/utils";
import { AgentSubscriptions } from "@/components/AgentSubscriptions";
import { useAgentName } from "@/lib/agent";

interface Channel { name: string; kind: string }
interface Msg { id: string; ts: string; type: string; source: string; text: string }
// A distinct unit of provider demand on a channel (§8.1): the watch identity,
// its params, and the agents subscribed under it. Read-only view.
interface Watch { watch: string; params?: Record<string, unknown> | null; subscribers?: string[] }

export default function ChannelsPage() {
  // Under /agent/:name/channels the left column is the agent's manageable
  // subscriptions (AgentSubscriptions); the global mount (no :name) keeps the
  // browse-all-channels + tail/send panel. messenger chat-routes (bind/create) live
  // on the Plugins tab now — they are plugin-global config, not per-agent.
  const { name: routeName } = useParams<{ name?: string }>();
  const contextName = useAgentName();
  const name = routeName || contextName || undefined;
  const [channels, setChannels] = useState<Channel[]>([]);
  const [sel, setSel] = useState<string | null>(null);
  const [msgs, setMsgs] = useState<Msg[]>([]);
  const [watches, setWatches] = useState<Watch[]>([]);
  const [text, setText] = useState("");
  const [type, setType] = useState("note");

  const loadTail = (ch: string) =>
    apiGet<{ messages: Msg[] }>(`/api/channels/${encodeURIComponent(ch)}/messages`).then((r) => setMsgs(r.messages)).catch(() => setMsgs([]));
  const loadWatches = (ch: string) =>
    apiGet<{ watches: Watch[] }>(`/api/channels/${encodeURIComponent(ch)}/watches`).then((r) => setWatches(r.watches ?? [])).catch(() => setWatches([]));

  // The global channel list backs the browse panel; under an agent the left
  // column is AgentSubscriptions, so we only fetch here when unscoped.
  useEffect(() => {
    if (name) return;
    apiGet<{ channels: Channel[] }>("/api/channels")
      .then((r) => { setChannels(r.channels); setSel(null); })
      .catch(() => setChannels([]));
  }, [name]);
  useEffect(() => {
    if (!sel) { setWatches([]); return; }
    void loadTail(sel);
    void loadWatches(sel);
    const t = window.setInterval(() => void loadTail(sel), 2000);
    return () => window.clearInterval(t);
  }, [sel]);

  const send = async () => {
    if (!sel) return;
    try {
      await apiPost("/api/messages", { channel: sel, type, text });
      setText("");
      toast.success("sent");
      void loadTail(sel);
    } catch (e) {
      toast.error(`send failed: ${e instanceof ApiError ? e.message : String(e)}`);
    }
  };

  return (
    <div className="flex h-full flex-col gap-3">
      <div className="flex flex-1 gap-4 overflow-hidden">
      {name ? (
        <AgentSubscriptions name={name} selected={sel} onSelect={setSel} />
      ) : (
        <div className="w-2/5 min-w-[12rem] shrink-0 overflow-auto">
          <div className="p-2 text-xs font-medium text-muted-foreground">CHANNELS</div>
          {channels.map((c) => (
            <button key={c.name} onClick={() => setSel(c.name)}
              className={cn("flex w-full items-center justify-between px-2 py-1.5 text-left text-sm hover:bg-accent", sel === c.name && "bg-accent")}>
              <span className="truncate font-mono text-xs">{c.name}</span>
              <Badge variant="secondary">{c.kind}</Badge>
            </button>
          ))}
          {channels.length === 0 && <p className="p-2 text-sm text-muted-foreground">No channels.</p>}
        </div>
      )}
      <div className="flex flex-1 flex-col gap-3 overflow-hidden">
        {sel ? (
          <>
          <Card className="flex flex-1 flex-col overflow-hidden">
            <CardHeader className="pb-2"><CardTitle className="text-base font-mono">{sel}</CardTitle></CardHeader>
            <CardContent className="flex flex-1 flex-col gap-2 overflow-hidden">
              <ScrollArea className="h-[45vh] rounded border bg-muted/30 p-2">
                {msgs.map((m) => (
                  <div key={m.id} className="border-b py-1 text-sm last:border-0">
                    <span className="mr-2 text-xs text-muted-foreground">{m.source}·{m.type}</span>
                    <span>{m.text}</span>
                  </div>
                ))}
                {msgs.length === 0 && <p className="text-sm text-muted-foreground">No messages.</p>}
              </ScrollArea>
              <div className="flex gap-2">
                <Input value={type} onChange={(e) => setType(e.target.value)} className="h-8 w-28" placeholder="type" />
                <Input value={text} onChange={(e) => setText(e.target.value)} className="h-8 flex-1" placeholder="message text"
                  onKeyDown={(e) => { if (e.key === "Enter") void send(); }} />
                <Button size="sm" onClick={() => void send()}>Send</Button>
              </div>
            </CardContent>
          </Card>
          <WatchesCard watches={watches} />
          </>
        ) : (
          <p className="text-sm text-muted-foreground">Select a channel.</p>
        )}
      </div>
      </div>
    </div>
  );
}

// WatchesCard renders a channel's distinct provider watches (§8.1/§10, Phase R):
// each watch's identity, its params pretty-printed, and the subscribed agents.
// Read-only — this is an inspect view, not an editor.
function WatchesCard({ watches }: { watches: Watch[] }) {
  return (
    <Card className="shrink-0">
      <CardHeader className="pb-2"><CardTitle className="text-sm">Watches</CardTitle></CardHeader>
      <CardContent className="pt-0">
        {watches.length === 0 ? (
          <p className="text-sm text-muted-foreground">No watches on this channel.</p>
        ) : (
          <ScrollArea className="max-h-[22vh]">
            <ul className="space-y-2">
              {watches.map((w) => (
                <li key={w.watch} className="rounded border p-2 text-sm">
                  <div className="mb-1 font-mono text-xs">{w.watch}</div>
                  {w.params && Object.keys(w.params).length > 0 && (
                    <pre className="mb-1 overflow-x-auto rounded bg-muted/50 p-1 text-xs">{JSON.stringify(w.params, null, 2)}</pre>
                  )}
                  <div className="flex flex-wrap gap-1">
                    {(w.subscribers ?? []).map((s) => (
                      <Badge key={s} variant="secondary" className="font-mono text-xs">{s}</Badge>
                    ))}
                    {(w.subscribers ?? []).length === 0 && (
                      <span className="text-xs text-muted-foreground">no subscribers</span>
                    )}
                  </div>
                </li>
              ))}
            </ul>
          </ScrollArea>
        )}
      </CardContent>
    </Card>
  );
}
