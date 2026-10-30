import { Link } from "react-router-dom";
import { Input } from "@/components/ui/input";
import { ACTIVITY_PERIODS, type ActivityPeriod, type ActivityWindow } from "@/lib/activityPeriod";
import type { TaskActivityAgent, TaskActivityRow, TaskActivityTotals } from "@/lib/api";
import { fmtDateTime } from "@/lib/time";
import { cn } from "@/lib/utils";

const fmtUSD = (usd: number) => `$${usd.toFixed(2)}`;

/** The period buttons (Hour · Day · Week · Custom unless periods says
 *  otherwise), with the custom range inputs and their error. */
export function ActivityPeriodBar({
  window,
  onPeriod,
  onCustom,
  periods = ACTIVITY_PERIODS,
}: {
  window: ActivityWindow;
  onPeriod: (period: ActivityPeriod) => void;
  onCustom: (bound: "since" | "until", value: string) => void;
  periods?: Array<{ period: ActivityPeriod; label: string }>;
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <div
        role="group"
        aria-label="Period"
        className="flex overflow-hidden rounded-md border text-sm"
      >
        {periods.map(({ period, label }) => (
          <button
            key={period}
            type="button"
            aria-pressed={window.period === period}
            onClick={() => onPeriod(period)}
            className={cn(
              "min-h-8 px-3 py-1 hover:bg-accent",
              window.period === period && "bg-primary text-primary-foreground hover:bg-primary",
            )}
          >
            {label}
          </button>
        ))}
      </div>
      {window.period === "custom" && (
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <label className="flex items-center gap-1">
            <span className="text-muted-foreground">From</span>
            <Input
              type="datetime-local"
              value={window.customSince}
              onChange={(e) => onCustom("since", e.target.value)}
              className="h-auto w-auto rounded border bg-background px-2 py-1 text-sm dark:bg-background"
            />
          </label>
          <label className="flex items-center gap-1">
            <span className="text-muted-foreground">To</span>
            <Input
              type="datetime-local"
              value={window.customUntil}
              onChange={(e) => onCustom("until", e.target.value)}
              className="h-auto w-auto rounded border bg-background px-2 py-1 text-sm dark:bg-background"
            />
          </label>
        </div>
      )}
      {window.error ? (
        <span role="alert" className="text-sm text-destructive">
          {window.error}
        </span>
      ) : (
        <span className="text-xs text-muted-foreground">
          {fmtDateTime(window.since)} – {fmtDateTime(window.until)}
        </span>
      )}
    </div>
  );
}

function allCards(totals: TaskActivityTotals | null): Array<[string, string]> {
  return [
    ["Tasks", totals ? String(totals.tasks) : "—"],
    ["Completed", totals ? String(totals.completed) : "—"],
    ["Status changes", totals ? String(totals.transitions) : "—"],
    ["Comments", totals ? String(totals.comments) : "—"],
    ["Runs", totals ? String(totals.iterations) : "—"],
    ["Cost", totals ? fmtUSD(totals.cost_usd) : "—"],
  ];
}

/** An agent's four: what got done, the runs and money it took. */
function agentCards(totals: TaskActivityTotals | null): Array<[string, string]> {
  const average = totals?.completed ? fmtUSD(totals.cost_usd / totals.completed) : "—";
  return [
    ["Tasks done", totals ? String(totals.completed) : "—"],
    ["Runs", totals ? String(totals.iterations) : "—"],
    ["Cost", totals ? fmtUSD(totals.cost_usd) : "—"],
    ["Avg. cost per task", average],
  ];
}

/** The period's totals: six cards by default, the agent tab's four with
 *  variant="agent". */
export function ActivityTotals({
  totals,
  variant = "all",
}: {
  totals: TaskActivityTotals | null;
  variant?: "all" | "agent";
}) {
  const cards = variant === "agent" ? agentCards(totals) : allCards(totals);
  return (
    <div
      className={cn(
        "grid grid-cols-2 gap-2",
        variant === "agent" ? "lg:grid-cols-4" : "sm:grid-cols-3 lg:grid-cols-6",
      )}
    >
      {cards.map(([label, value]) => (
        <div key={label} data-total={label} className="rounded border p-3">
          <div className="text-xs text-muted-foreground">{label}</div>
          <div className="mt-1 font-mono text-lg">{value}</div>
        </div>
      ))}
    </div>
  );
}

export function ActivityTabs<V extends string>({
  views,
  active,
  onSelect,
}: {
  views: Array<{ view: V; label: string }>;
  active: V;
  onSelect: (view: V) => void;
}) {
  return (
    <div
      role="tablist"
      aria-label="Activity view"
      className="scrollbar-none flex gap-1 overflow-x-auto"
    >
      {views.map(({ view, label }) => (
        <button
          key={view}
          type="button"
          role="tab"
          aria-selected={active === view}
          onClick={() => onSelect(view)}
          className={cn(
            "shrink-0 rounded px-3 py-1 text-sm whitespace-nowrap hover:bg-accent",
            active === view ? "bg-accent font-medium" : "text-muted-foreground",
          )}
        >
          {label}
        </button>
      ))}
    </div>
  );
}

/** What happened to the task in the period, in the order it is most useful. */
function periodSummary(row: TaskActivityRow): string {
  const parts: string[] = [];
  if (row.outcomes.length) parts.push(`outcomes: ${row.outcomes.join(", ")}`);
  if (row.entered_statuses.length) parts.push(`entered: ${row.entered_statuses.join(" → ")}`);
  if (row.completed) parts.push("completed");
  return parts.join(" · ");
}

export function TaskActivityTable({
  rows,
  onOpenTask,
  showAgents = false,
}: {
  rows: TaskActivityRow[];
  onOpenTask: (key: string) => void;
  showAgents?: boolean;
}) {
  return (
    <div className="overflow-x-auto rounded border">
      <table className="w-full min-w-180 text-sm">
        <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
          <tr>
            <th className="px-3 py-2">Task</th>
            <th className="px-3 py-2">Title</th>
            <th className="px-3 py-2">Status</th>
            {showAgents && <th className="px-3 py-2">Agents</th>}
            <th className="px-3 py-2">In this period</th>
            <th className="px-3 py-2 text-right">Runs</th>
            <th className="px-3 py-2 text-right">Comments</th>
            <th className="px-3 py-2 text-right">Cost</th>
            <th className="px-3 py-2">Last activity</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.key} className="border-t align-top">
              <td className="px-3 py-1.5">
                <button
                  type="button"
                  onClick={() => onOpenTask(row.key)}
                  className="font-mono text-xs text-primary hover:underline"
                >
                  {row.key}
                </button>
              </td>
              <td className="px-3 py-1.5">{row.title}</td>
              <td className="px-3 py-1.5 whitespace-nowrap">{row.status}</td>
              {showAgents && <td className="px-3 py-1.5">{row.agents.join(", ")}</td>}
              <td className="px-3 py-1.5 text-muted-foreground">{periodSummary(row)}</td>
              <td className="px-3 py-1.5 text-right font-mono">{row.iterations}</td>
              <td className="px-3 py-1.5 text-right font-mono">{row.comments}</td>
              <td className="px-3 py-1.5 text-right font-mono">{fmtUSD(row.cost_usd)}</td>
              <td className="px-3 py-1.5 whitespace-nowrap text-muted-foreground">
                {row.last_at ? fmtDateTime(row.last_at) : "—"}
              </td>
            </tr>
          ))}
          {rows.length === 0 && (
            <tr>
              <td
                colSpan={showAgents ? 9 : 8}
                className="px-3 py-4 text-center text-muted-foreground"
              >
                No work on tasks in this period.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

export function AgentActivityTable({
  agents,
  agentHref,
}: {
  agents: TaskActivityAgent[];
  agentHref: (agent: string) => string;
}) {
  return (
    <div className="overflow-x-auto rounded border">
      <table className="w-full min-w-130 text-sm">
        <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
          <tr>
            <th className="px-3 py-2">Agent</th>
            <th className="px-3 py-2 text-right">Tasks</th>
            <th className="px-3 py-2 text-right">Completed</th>
            <th className="px-3 py-2 text-right">Runs</th>
            <th className="px-3 py-2 text-right">Requests</th>
            <th className="px-3 py-2 text-right">Cost</th>
          </tr>
        </thead>
        <tbody>
          {agents.map((row) => (
            <tr key={row.agent} className="border-t">
              <td className="px-3 py-1.5">
                <Link to={agentHref(row.agent)} className="text-primary hover:underline">
                  {row.agent}
                </Link>
              </td>
              <td className="px-3 py-1.5 text-right font-mono">{row.tasks}</td>
              <td className="px-3 py-1.5 text-right font-mono">{row.completed}</td>
              <td className="px-3 py-1.5 text-right font-mono">{row.iterations}</td>
              <td className="px-3 py-1.5 text-right font-mono">{row.requests}</td>
              <td className="px-3 py-1.5 text-right font-mono">{fmtUSD(row.cost_usd)}</td>
            </tr>
          ))}
          {agents.length === 0 && (
            <tr>
              <td colSpan={6} className="px-3 py-4 text-center text-muted-foreground">
                No agent worked on tasks in this period.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

export function TruncatedNote({ shown }: { shown: number }) {
  return (
    <p className="text-xs text-muted-foreground">
      Showing the {shown} most recent tasks. The totals count every task in the period.
    </p>
  );
}
