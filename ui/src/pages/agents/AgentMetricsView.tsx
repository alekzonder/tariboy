import { useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { MetricsCards } from "@/components/metrics/MetricsCards";
import { MetricsError, MetricsLoading } from "@/components/metrics/MetricsStatus";
import { QualityBlock } from "@/components/metrics/QualityBlock";
import { RefreshControl } from "@/components/metrics/RefreshControl";
import { VersionGroups } from "@/components/metrics/VersionGroups";
import { Button } from "@/components/ui/button";
import { useMetrics } from "@/hooks/useMetrics";
import { activityWindow } from "@/lib/activityPeriod";
import { useAgentName } from "@/lib/agent";
import { compareQuery, metricsPeriodParams, type CompareKind } from "@/lib/metricsQuery";
import { cn } from "@/lib/utils";

/** The window of the tab's period. It ends at the time of the load, the last
 *  period change or Refresh: the tab's own window moves every minute, and
 *  every new window would be a new heavy request. */
function useFrozenWindow() {
  const [searchParams] = useSearchParams();
  const period = ["period", "since", "until"].map((k) => searchParams.get(k) ?? "").join("\n");
  const [clock, setClock] = useState(() => ({ period, now: new Date() }));
  if (clock.period !== period) setClock({ period, now: new Date() });
  return {
    window: activityWindow(searchParams, clock.now, "today"),
    refresh: () => setClock({ period, now: new Date() }),
  };
}

/** Activity → More → Metrics: the completed tasks of this agent in the tab's
 *  period, with Quality and the image and workflow versions it ran. */
export default function AgentMetricsView() {
  const name = useAgentName();
  const { hostId = "local" } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const { window, refresh } = useFrozenWindow();
  const data = useMetrics(
    window.error ? null : { since: window.since, until: window.until, agent: name },
  );
  // The tab's period bar already shows why a custom range cannot be used.
  if (window.error) return null;

  const base = metricsPeriodParams(hostId, window);
  const metricsLink = new URLSearchParams(base);
  metricsLink.set("agent", name);
  const compareHref = (kind: CompareKind) => (group: string, a: string, b: string) =>
    `/metrics/compare?${compareQuery(base, kind, group, a, b)}`;
  const showMonth = () => {
    const next = new URLSearchParams(searchParams);
    next.set("period", "month");
    setSearchParams(next, { replace: true });
  };
  const report = data.report;
  const empty =
    !!report && report.total.usage.iterations === 0 && report.total.tasks.tasks_done === 0;

  return (
    <section aria-label="Agent metrics" className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="min-w-0 text-body text-muted-foreground">
          Completed tasks of this agent in this period. Runs and cost count each task's whole life.{" "}
          <Link to={`/metrics?${metricsLink}`} className="text-primary hover:underline">
            Open in Metrics
          </Link>
        </p>
        <RefreshControl
          updatedAt={data.updatedAt}
          updating={data.updating}
          disabled={false}
          onRefresh={() => {
            refresh();
            data.retry();
          }}
        />
      </div>
      {data.loading && <MetricsLoading />}
      {data.error !== null && <MetricsError error={data.error} onRetry={data.retry} />}
      {report && data.error === null && (
        <div aria-busy={data.updating} className={cn("space-y-4", data.updating && "opacity-60")}>
          {empty ? (
            <div className="space-y-2 rounded-chip border p-4 text-body">
              <p>No completed tasks of this agent in this period.</p>
              {(window.period === "today" || window.period === "week") && (
                <Button type="button" size="sm" variant="outline" onClick={showMonth}>
                  Show month
                </Button>
              )}
            </div>
          ) : (
            <>
              <MetricsCards slice={report.total} usage={false} />
              <QualityBlock quality={report.total.tasks.quality} />
              <h2 className="text-body font-medium">Images</h2>
              <VersionGroups
                kind="image"
                slices={report.images}
                compareHref={compareHref("image")}
              />
              <h2 className="text-body font-medium">Workflows</h2>
              <VersionGroups
                kind="workflow"
                slices={report.workflows}
                compareHref={compareHref("workflow")}
              />
            </>
          )}
        </div>
      )}
    </section>
  );
}
