import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { TOUR_ANCHORS } from "@/lib/onboarding/tour";
import { ActivityTotals } from "@/pages/activity/ActivityParts";
import type { TaskActivityTotals } from "@/types";

export interface HomeAgentLink {
  key: string;
  name: string;
  /** "12 min · build-01", "idle", already formatted. */
  meta: string;
  href: string;
}

export interface HomeViewProps {
  needsYouCount: number;
  /** The Needs you list. It keeps its own task drawer and data, so the
   *  container renders it. */
  needsYou: ReactNode;
  running: HomeAgentLink[];
  recent: HomeAgentLink[];
  totals: TaskActivityTotals | null;
  todayError: string | null;
  activityHref: string;
  onNewAgent: () => void;
}

/** The name and the meta of one agent link. Below `sm` the meta goes to a
 *  second line, so a long meta never hides the name. From `sm` up both stay
 *  on one line and the meta takes at most half of it. */
function AgentLinkText({ name, meta }: { name: string; meta: string }) {
  return (
    <span className="flex min-w-0 flex-1 flex-col sm:flex-row sm:items-baseline sm:gap-2">
      <span className="min-w-0 truncate">{name}</span>
      <span className="min-w-0 truncate text-caption text-muted-foreground sm:max-w-1/2 sm:shrink-0">
        {meta}
      </span>
    </span>
  );
}

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

/** Home for a workspace with agents: what needs the user, what runs now,
 *  today's numbers and the agents worked with last. */
export function HomeView({
  needsYouCount,
  needsYou,
  running,
  recent,
  totals,
  todayError,
  activityHref,
  onNewAgent,
}: HomeViewProps) {
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
      <Section title={`Needs you (${needsYouCount})`}>{needsYou}</Section>
      {/* Full width: the six Today tiles need it at 1024 px. */}
      <Section title={`Running now (${running.length})`}>
        {running.length === 0 && (
          <p className="px-2 text-body-sm text-muted-foreground">No agent is running.</p>
        )}
        <ul className="space-y-0.5">
          {running.map((row) => (
            <li key={row.key}>
              <Link
                to={row.href}
                className="flex items-baseline gap-2 rounded-control px-2 py-1 text-body-sm hover:bg-accent"
              >
                <span
                  aria-hidden="true"
                  className="mt-1.75 size-1.5 shrink-0 self-start rounded-full bg-status-running sm:mt-0 sm:self-center"
                />
                <AgentLinkText name={row.name} meta={row.meta} />
              </Link>
            </li>
          ))}
        </ul>
      </Section>
      <Section
        title="Today"
        aside={
          <Link
            to={activityHref}
            className="ml-auto text-body-sm text-muted-foreground hover:text-foreground"
          >
            History ›
          </Link>
        }
      >
        <ActivityTotals totals={totals} />
        {todayError && (
          <p role="alert" className="px-2 text-body-sm text-destructive">
            {todayError}
          </p>
        )}
      </Section>
      <Section title="Recent">
        <ul className="grid gap-x-6 gap-y-0.5 sm:grid-cols-2">
          {recent.map((row) => (
            <li key={row.key} className="min-w-0">
              <Link
                to={row.href}
                className="flex items-baseline gap-2 rounded-control px-2 py-1 text-body-sm hover:bg-accent"
              >
                <AgentLinkText name={row.name} meta={row.meta} />
              </Link>
            </li>
          ))}
        </ul>
      </Section>
    </div>
  );
}
