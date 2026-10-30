import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { SearchInput } from "@/components/ui/search-input";
import type { HostAgents } from "@/lib/aggregate";
import { scopedPath } from "@/lib/navRoutes";
import { defaultAgentTab } from "@/lib/uiMode";
import { cn } from "@/lib/utils";
import { allAgents } from "./sidebar/sidebarModel";
import { agentPath } from "./terminalsHosts";
import type { TaskHit } from "./TaskSearchLive";

// The task lookup brings the API client: it loads on the first search.
const TaskSearchLive = lazy(() => import("./TaskSearchLive"));

const MAX_AGENTS = 6;

type Hit =
  | { kind: "agent"; id: string; label: string; detail: string; path: string }
  | { kind: "task"; id: string; label: string; detail: string; path: string };

function hits(hosts: HostAgents[], query: string, tasks: TaskHit[]): Hit[] {
  const q = query.trim().toLowerCase();
  if (!q) return [];
  const multiHost = hosts.length >= 2;
  const agents: Hit[] = allAgents(hosts)
    .filter((row) => row.agent.name.toLowerCase().includes(q))
    .slice(0, MAX_AGENTS)
    .map((row) => ({
      kind: "agent",
      id: row.key,
      label: row.agent.name,
      detail: multiHost ? `${row.agent.state} · ${row.hostLabel}` : row.agent.state,
      path: agentPath(row.hostId, row.agent.name, defaultAgentTab()),
    }));
  const found: Hit[] = tasks.map((task) => ({
    kind: "task",
    id: `${task.hostId}\u0000${task.key}`,
    label: `${task.key} ${task.title}`,
    detail: multiHost ? `${task.status} · ${task.hostLabel}` : task.status,
    path: scopedPath("tasks", multiHost ? task.hostId : undefined, { task: task.key }),
  }));
  return [...agents, ...found];
}

/**
 * The titlebar search: it filters the sidebar's agent list as you type and
 * lists matching agents and tasks (by key or title, open or closed) of the
 * workspace's hosts. ⌘K or Ctrl+K focuses it; Enter opens the first result.
 */
export function GlobalSearch({
  hosts,
  query,
  onQuery,
}: {
  hosts: HostAgents[];
  query: string;
  onQuery: (query: string) => void;
}) {
  const navigate = useNavigate();
  const input = useRef<HTMLInputElement | null>(null);
  const [open, setOpen] = useState(false);
  const [tasks, setTasks] = useState<TaskHit[]>([]);
  const results = hits(hosts, query, tasks);

  useEffect(() => {
    const focus = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        input.current?.focus();
        input.current?.select();
      }
    };
    window.addEventListener("keydown", focus);
    return () => window.removeEventListener("keydown", focus);
  }, []);

  const go = (hit: Hit) => {
    setOpen(false);
    input.current?.blur();
    navigate(hit.path);
  };
  const searching = query.trim().length > 0;

  return (
    <div className="relative w-64 max-w-[40vw] min-w-0">
      <SearchInput
        ref={input}
        className="h-6.5"
        iconClassName="size-3.25"
        inputClassName="text-body-sm"
        value={query}
        role="combobox"
        aria-expanded={open && searching}
        aria-controls="global-search-results"
        aria-label="Search agents and tasks"
        placeholder="Search"
        hint="⌘K"
        onChange={(event) => {
          onQuery(event.target.value);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
        onKeyDown={(event) => {
          if (event.key === "Enter" && results[0]) go(results[0]);
          if (event.key === "Escape") {
            onQuery("");
            setOpen(false);
          }
        }}
      />
      {searching && (
        <Suspense fallback={null}>
          <TaskSearchLive hosts={hosts} query={query} onResults={setTasks} />
        </Suspense>
      )}
      {open && searching && (
        <ul
          id="global-search-results"
          role="listbox"
          aria-label="Search results"
          className="absolute top-full right-0 z-50 mt-1 max-h-96 w-96 max-w-[80vw] overflow-y-auto rounded-card bg-popover p-1 text-popover-foreground shadow-[var(--lift)]"
        >
          {results.length === 0 && (
            <li className="px-2 py-1.5 text-body-sm text-muted-foreground">No matches.</li>
          )}
          {results.map((hit, index) => (
            <li key={`${hit.kind}\u0000${hit.id}`} role="option" aria-selected={index === 0}>
              <button
                type="button"
                // Keeps the field focused until the click lands.
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => go(hit)}
                className={cn(
                  "flex w-full items-baseline gap-2 rounded-control px-2 py-1 text-left text-body-sm hover:bg-accent",
                  index === 0 && "bg-accent/60",
                )}
              >
                <span className="w-10 shrink-0 text-caption text-muted-foreground">
                  {hit.kind === "agent" ? "Agent" : "Task"}
                </span>
                <span className="min-w-0 flex-1 truncate">{hit.label}</span>
                <span className="shrink-0 text-caption text-muted-foreground">{hit.detail}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
