import { useEffect, useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { useAgentName } from "@/lib/agent";
import { agentGetOn, setIterationTagsOn, subscribeAgentEventsOn } from "@/lib/api";
import type { IterationSummary } from "@/lib/types";
import { FullAuditLog } from "@/components/FullAuditLog";
import { IterationAuditLog } from "@/components/IterationAuditLog";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { fmtDateTime } from "@/lib/time";
import { cn } from "@/lib/utils";
import { paramToHost, targetFor } from "@/lib/terminalsHost";

// Map a backend iteration status to a badge variant. Backend Status is one of
// running|done|no_i_am_done|harness_error|timeout|killed (internal/agent/
// agent.go): a completed-clean run reads as 'default', the failure states stand
// out as 'destructive', and in-flight/incomplete ('running', 'no_i_am_done')
// stay neutral 'secondary'.
function statusVariant(
  status: string,
): "default" | "secondary" | "destructive" {
  if (status === "done") return "default";
  if (status === "harness_error" || status === "timeout" || status === "killed")
    return "destructive";
  return "secondary";
}

// AuditLogPage is the merged Iterations + Audit Log view: one tab, a left column
// listing "Full log" plus every iteration (newest first), and a right pane that
// shows the full cross-iteration stream (default) or a single iteration's log.
export default function AuditLogPage() {
  const name = useAgentName();
  const { hostId: hostParam = "local" } = useParams();
  const hostId = paramToHost(hostParam);
  const descriptor = targetFor(hostId);
  const [searchParams, setSearchParams] = useSearchParams();
  // Tag and started-at filters live in the URL beside ?iteration=, so a filtered
  // view is shareable and survives a reload. They are passed to the daemon,
  // which owns the matching (any listed tag; inclusive date bounds).
  const tagFilter = searchParams.get("tag") ?? "";
  const afterFilter = searchParams.get("started_after") ?? "";
  const beforeFilter = searchParams.get("started_before") ?? "";
  const query = new URLSearchParams();
  if (tagFilter) query.set("tag", tagFilter);
  if (afterFilter) query.set("started_after", afterFilter);
  if (beforeFilter) query.set("started_before", beforeFilter);
  const queryString = query.toString();
  const listKey = `${hostId}\0${descriptor?.baseURL ?? ""}\0${descriptor?.token ?? ""}\0${name}\0${queryString}`;
  const [listState, setListState] = useState<{ key: string; items: IterationSummary[] } | null>(null);
  const items = listState?.key === listKey ? listState.items : [];
  const loaded = listState?.key === listKey;
  // selected iteration id; null = the Full log (default). Preselect from the
  // ?iteration= query param (e.g. an iteration row on the Usage tab links here);
  // absent param keeps the default Full-log pane.
  const selected = searchParams.get("iteration");
  const select = (iteration: string | null) => {
    const next = new URLSearchParams(searchParams);
    if (iteration) next.set("iteration", iteration);
    else next.delete("iteration");
    setSearchParams(next);
  };
  // A filter change replaces the history entry: typing into a filter must not
  // fill the back stack with one entry per keystroke.
  const setFilter = (key: string, value: string) => {
    const next = new URLSearchParams(searchParams);
    if (value) next.set(key, value);
    else next.delete(key);
    setSearchParams(next, { replace: true });
  };

  // Load iterations and keep them fresh: a light 5s poll plus the SSE
  // ["iteration"] stream so newly started/finished iterations appear promptly.
  // Backend returns oldest-first; sort newest-first for display.
  useEffect(() => {
    if (!name) return;
    let current = true;
    let generation = 0;
    const target = targetFor(hostId);
    const load = () => {
      const requestGeneration = ++generation;
      void agentGetOn<{ iterations: IterationSummary[]; count: number }>(
        target,
        name,
        queryString ? `iterations?${queryString}` : "iterations",
      )
        .then((r) => {
          if (current && requestGeneration === generation) setListState({
            key: listKey,
            items: r.iterations
              .slice()
              .sort((a, b) => b.started_at.localeCompare(a.started_at)),
          });
        })
        .catch(() => { /* keep last on a transient failure */ });
    };
    load();
    const t = window.setInterval(load, 5000);
    const off = subscribeAgentEventsOn(target, name, ["iteration"], () => load());
    return () => { current = false; window.clearInterval(t); off(); };
  }, [hostId, listKey, name, queryString]);

  const selectedItem = selected ? items.find((it) => it.id === selected) : null;

  // Tag edits replace the whole set of the one selected iteration and patch the
  // loaded row in place, so the badge list updates before the next poll.
  const [draftTag, setDraftTag] = useState("");
  const [tagError, setTagError] = useState("");
  const saveTags = (id: string, tags: string[]) => {
    setTagError("");
    void setIterationTagsOn(targetFor(hostId), name, id, tags)
      .then((r) => {
        const applied = r.tags?.[id] ?? tags;
        setListState((prev) => prev && ({
          ...prev,
          items: prev.items.map((it) => (it.id === id ? { ...it, tags: applied } : it)),
        }));
      })
      .catch((e: unknown) => setTagError(e instanceof Error ? e.message : "could not save tags"));
  };

  return (
    <div className="flex h-full gap-4">
      <div className="flex w-64 shrink-0 flex-col gap-2 overflow-auto">
        <div className="flex flex-col gap-1">
          <Input
            aria-label="Filter by tag"
            placeholder="tag, or tag,tag"
            value={tagFilter}
            onChange={(e) => setFilter("tag", e.target.value)}
          />
          <div className="flex gap-1">
            <Input
              aria-label="Started after"
              type="date"
              value={afterFilter}
              onChange={(e) => setFilter("started_after", e.target.value)}
            />
            <Input
              aria-label="Started before"
              type="date"
              value={beforeFilter}
              onChange={(e) => setFilter("started_before", e.target.value)}
            />
          </div>
        </div>
        <button
          onClick={() => select(null)}
          className={cn(
            "flex w-full items-center px-2 py-1.5 text-left text-sm hover:bg-accent",
            selected === null && "bg-accent",
          )}
        >
          Full log
        </button>
        {items.map((it) => (
          <button
            key={it.id}
            onClick={() => select(it.id)}
            className={cn(
              "flex w-full items-center justify-between gap-2 px-2 py-1.5 text-left text-sm hover:bg-accent",
              selected === it.id && "bg-accent",
            )}
          >
            <span className="font-mono text-xs">{fmtDateTime(it.started_at)}</span>
            <span className="flex flex-wrap items-center justify-end gap-1">
              {(it.tags ?? []).map((tag) => (
                <Badge key={tag} variant="outline">{tag}</Badge>
              ))}
              {it.productive === false && (
                <Badge variant="outline" title="finished with i-am-done --idle (no productive work)">
                  idle
                </Badge>
              )}
              <Badge variant={statusVariant(it.status)}>{it.status}</Badge>
            </span>
          </button>
        ))}
      </div>
      <div className="min-h-0 flex-1 overflow-hidden">
        {selected === null ? (
          <FullAuditLog name={name} />
        ) : selectedItem ? (
          <div className="flex h-full flex-col">
            <div className="mb-2 flex flex-wrap items-center gap-1">
              {(selectedItem.tags ?? []).map((tag) => (
                <Badge key={tag} variant="outline" className="gap-1">
                  {tag}
                  <button
                    aria-label={`Remove tag ${tag}`}
                    onClick={() =>
                      saveTags(selectedItem.id, (selectedItem.tags ?? []).filter((t) => t !== tag))
                    }
                  >
                    ×
                  </button>
                </Badge>
              ))}
              <Input
                aria-label="Add tag"
                className="h-7 w-32"
                placeholder="add tag"
                value={draftTag}
                onChange={(e) => setDraftTag(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key !== "Enter" || !draftTag.trim()) return;
                  saveTags(selectedItem.id, [
                    ...(selectedItem.tags ?? []).filter((t) => t !== draftTag.trim()),
                    draftTag.trim(),
                  ]);
                  setDraftTag("");
                }}
              />
              {tagError && <span role="alert" className="text-xs text-destructive">{tagError}</span>}
            </div>
            <div className="min-h-0 flex-1">
              <IterationAuditLog
                name={name}
                iterationId={selected}
                iterationStatus={selectedItem.status}
                iterationProductive={selectedItem.productive}
              />
            </div>
          </div>
        ) : loaded ? (
          <p role="status" className="text-sm text-muted-foreground">Iteration {selected} was not found.</p>
        ) : (
          <p className="text-sm text-muted-foreground">Loading iteration…</p>
        )}
      </div>
    </div>
  );
}
