import { useEffect, useMemo, useState, type ReactNode } from "react";
import { Link, useNavigate } from "react-router-dom";
import { Plus } from "lucide-react";
import { ApiProvider } from "@/components/ApiProvider";
import { Button } from "@/components/ui/button";
import { useTaskActivity } from "@/hooks/useTaskActivity";
import type { HostAgents } from "@/lib/aggregate";
import { activityWindow } from "@/lib/activityPeriod";
import { scopedPath } from "@/lib/navRoutes";
import { needsYouRows } from "@/lib/needsYou";
import { TOUR_ANCHORS } from "@/lib/onboarding/tour";
import { targetFor } from "@/lib/terminalsHost";
import { defaultAgentTab } from "@/lib/uiMode";
import { ActivityTotals } from "@/pages/activity/ActivityParts";
import { useNeedsYou } from "@/pages/terminals/needsYouContext";
import { agentPath } from "@/pages/terminals/terminalsHosts";
import { elapsed, recentAgents, runningAgents } from "./homeModel";
import { NeedsYouList } from "./NeedsYouList";

const RECENT = 8;
const MINUTE_MS = 60_000;

function Section({
  title,
  aside,
  children,
}: {
  title: string;
  aside?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className="min-w-0 space-y-1.5">
      <div className="flex items-center gap-2 px-2">
        <h2 className="text-body font-semibold">{title}</h2>
        {aside}
      </div>
      {children}
    </section>
  );
}

/** The rolling day's numbers of the page host, the Activity tiles. */
function Today() {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), MINUTE_MS);
    return () => window.clearInterval(timer);
  }, []);
  // No ?period=: the rolling day.
  const day = useMemo(() => activityWindow(new URLSearchParams(), now), [now]);
  const { report, error } = useTaskActivity(day);
  return (
    <>
      <ActivityTotals totals={report?.totals ?? null} />
      {error && <p className="px-2 text-body-sm text-destructive">{error}</p>}
    </>
  );
}

/**
 * Home for a workspace with agents: what needs the user, what runs now,
 * today's numbers and the agents worked with last.
 */
export default function HomePage({
  hosts,
  pageHostId,
  onNewAgent,
}: {
  /** The current workspace's hosts, in sidebar order. */
  hosts: HostAgents[];
  pageHostId: string;
  onNewAgent: () => void;
}) {
  const navigate = useNavigate();
  const needs = useNeedsYou();
  const rows = needsYouRows(needs.hosts);
  const multiHost = hosts.length >= 2;
  const running = runningAgents(hosts);
  const recent = recentAgents(hosts, RECENT);
  const now = new Date();
  const hostSuffix = (label: string) => (multiHost ? ` · ${label}` : "");

  return (
    <div className="h-full space-y-6 overflow-y-auto p-4">
      <header className="flex items-center gap-3">
        <h1 className="text-lg font-semibold">Home</h1>
        <Button
          className="ml-auto"
          size="sm"
          data-tour={TOUR_ANCHORS.newAgent}
          onClick={onNewAgent}
        >
          <Plus className="size-3.5" aria-hidden="true" />
          New agent
        </Button>
      </header>
      <Section title={`Needs you (${rows.length})`}>
        <NeedsYouList
          rows={rows}
          multiHost={multiHost}
          onOpenLetter={(hostId, letterId) =>
            navigate(scopedPath("inbox", multiHost ? hostId : undefined, { letter: letterId }))
          }
        />
      </Section>
      <div className="grid gap-6 lg:grid-cols-2">
        <Section title={`Running now (${running.length})`}>
          {running.length === 0 && (
            <p className="px-2 text-body-sm text-muted-foreground">No agent is running.</p>
          )}
          <ul className="space-y-0.5">
            {running.map((row) => (
              <li key={row.key}>
                <Link
                  to={agentPath(row.hostId, row.agent.name, "console")}
                  className="flex items-baseline gap-2 rounded-control px-2 py-1 text-body-sm hover:bg-accent"
                >
                  <span
                    aria-hidden="true"
                    className="size-1.5 shrink-0 self-center rounded-full bg-status-running"
                  />
                  <span className="min-w-0 truncate">{row.agent.name}</span>
                  <span className="shrink-0 text-caption text-muted-foreground">
                    {elapsed(row.agent.current_iteration_started_at ?? "", now)}
                    {hostSuffix(row.hostLabel)}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        </Section>
        <Section
          title="Today"
          aside={
            <Link
              to={scopedPath("activity", multiHost ? pageHostId : undefined)}
              className="ml-auto text-body-sm text-muted-foreground hover:text-foreground"
            >
              History ›
            </Link>
          }
        >
          <ApiProvider target={targetFor(pageHostId)}>
            <Today />
          </ApiProvider>
        </Section>
      </div>
      <Section title="Recent">
        <ul className="grid gap-x-6 gap-y-0.5 sm:grid-cols-2">
          {recent.map((row) => (
            <li key={row.key} className="min-w-0">
              <Link
                to={agentPath(row.hostId, row.agent.name, defaultAgentTab())}
                className="flex items-baseline gap-2 rounded-control px-2 py-1 text-body-sm hover:bg-accent"
              >
                <span className="min-w-0 truncate">{row.agent.name}</span>
                <span className="shrink-0 text-caption text-muted-foreground">
                  {row.agent.state}
                  {hostSuffix(row.hostLabel)}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      </Section>
    </div>
  );
}
