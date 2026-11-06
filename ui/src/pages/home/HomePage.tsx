import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { ApiProvider } from "@/components/ApiProvider";
import { useTaskActivity } from "@/hooks/useTaskActivity";
import type { HostAgents } from "@/lib/aggregate";
import { activityWindow } from "@/lib/activityPeriod";
import { scopedPath } from "@/lib/navRoutes";
import { targetFor } from "@/lib/terminalsHost";
import { DEFAULT_AGENT_TAB } from "@/pages/agents/agentWorkspaceModel";
import { useNeedsYou } from "@/pages/terminals/needsYouContext";
import { agentPath } from "@/pages/terminals/terminalsHosts";
import { elapsed, formatAge, recentAgents, runningAgents } from "./homeModel";
import { HomeView, RUNNING_LIMIT } from "./HomePage.view";
import { NeedsYouList } from "./NeedsYouList";
import { needsYouView } from "./needsYouModel";

const RECENT = 8;
const MINUTE_MS = 60_000;

type HomeProps = {
  /** The current workspace's hosts, in sidebar order. */
  hosts: HostAgents[];
  pageHostId: string;
  onNewAgent: () => void;
};

/** The hooks of Home: the page host's rolling day, Needs you and the agent lists. */
function HomeContainer({ hosts, pageHostId, onNewAgent }: HomeProps) {
  const navigate = useNavigate();
  const needs = useNeedsYou();
  const [now, setNow] = useState(() => new Date());
  const [runningExpanded, setRunningExpanded] = useState(false);
  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), MINUTE_MS);
    return () => window.clearInterval(timer);
  }, []);
  // No ?period=: the rolling day.
  const day = useMemo(() => activityWindow(new URLSearchParams(), now), [now]);
  const { report, error } = useTaskActivity(day);

  const multiHost = hosts.length >= 2;
  const needsView = needsYouView(needs.hosts, multiHost, now);
  const needsCount = `${needsView.total}${needsView.atLeast ? "+" : ""}`;
  const showAllHref = scopedPath(needsView.showAll, multiHost ? pageHostId : undefined);
  const hostSuffix = (label: string) => (multiHost ? ` · ${label}` : "");
  const allRunning = runningAgents(hosts).map((row) => ({
    key: row.key,
    name: row.agent.name,
    meta: `${elapsed(row.agent.current_iteration_started_at ?? "", now)}${hostSuffix(row.hostLabel)}`,
    href: agentPath(row.hostId, row.agent.name, "console"),
  }));
  const running = runningExpanded ? allRunning : allRunning.slice(0, RUNNING_LIMIT);
  const recent = recentAgents(hosts, RECENT).map((row) => ({
    key: row.key,
    name: row.agent.name,
    meta: `${row.agent.last_iteration_at ? formatAge(row.agent.last_iteration_at, now) : "not run yet"}${hostSuffix(row.hostLabel)}`,
    href: agentPath(row.hostId, row.agent.name, DEFAULT_AGENT_TAB),
  }));

  return (
    <HomeView
      needsYouCount={needsCount}
      needsYou={
        <NeedsYouList
          rows={needsView.rows}
          showAll={
            needsView.total > needsView.rows.length || needsView.atLeast
              ? { label: `Show all (${needsCount})`, href: showAllHref }
              : null
          }
          onOpenLetter={(hostId, letterId) =>
            navigate(scopedPath("inbox", multiHost ? hostId : undefined, { letter: letterId }))
          }
        />
      }
      running={running}
      runningTotal={allRunning.length}
      runningExpanded={runningExpanded}
      onToggleRunning={() => setRunningExpanded((value) => !value)}
      recent={recent}
      totals={report?.totals ?? null}
      todayError={error || null}
      activityHref={scopedPath("activity", multiHost ? pageHostId : undefined)}
      onNewAgent={onNewAgent}
    />
  );
}

/**
 * Home for a workspace with agents: what needs the user, what runs now,
 * today's numbers and the agents worked with last.
 */
export default function HomePage(props: HomeProps) {
  return (
    <ApiProvider target={targetFor(props.pageHostId)}>
      <HomeContainer {...props} />
    </ApiProvider>
  );
}
