import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { ApiProvider } from "@/components/ApiProvider";
import { useTaskActivity } from "@/hooks/useTaskActivity";
import type { HostAgents } from "@/lib/aggregate";
import { activityWindow } from "@/lib/activityPeriod";
import { scopedPath } from "@/lib/navRoutes";
import { needsYouRows } from "@/lib/needsYou";
import { targetFor } from "@/lib/terminalsHost";
import { DEFAULT_AGENT_TAB } from "@/pages/agents/agentWorkspaceModel";
import { useNeedsYou } from "@/pages/terminals/needsYouContext";
import { agentPath } from "@/pages/terminals/terminalsHosts";
import { elapsed, recentAgents, runningAgents } from "./homeModel";
import { HomeView } from "./HomePage.view";
import { NeedsYouList } from "./NeedsYouList";

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
  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), MINUTE_MS);
    return () => window.clearInterval(timer);
  }, []);
  // No ?period=: the rolling day.
  const day = useMemo(() => activityWindow(new URLSearchParams(), now), [now]);
  const { report, error } = useTaskActivity(day);

  const rows = needsYouRows(needs.hosts);
  const multiHost = hosts.length >= 2;
  const hostSuffix = (label: string) => (multiHost ? ` · ${label}` : "");
  const running = runningAgents(hosts).map((row) => ({
    key: row.key,
    name: row.agent.name,
    meta: `${elapsed(row.agent.current_iteration_started_at ?? "", now)}${hostSuffix(row.hostLabel)}`,
    href: agentPath(row.hostId, row.agent.name, "console"),
  }));
  const recent = recentAgents(hosts, RECENT).map((row) => ({
    key: row.key,
    name: row.agent.name,
    meta: `${row.agent.state}${hostSuffix(row.hostLabel)}`,
    href: agentPath(row.hostId, row.agent.name, DEFAULT_AGENT_TAB),
  }));

  return (
    <HomeView
      needsYouCount={rows.length}
      needsYou={
        <NeedsYouList
          rows={rows}
          multiHost={multiHost}
          onOpenLetter={(hostId, letterId) =>
            navigate(scopedPath("inbox", multiHost ? hostId : undefined, { letter: letterId }))
          }
        />
      }
      running={running}
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
