import type { ReactNode } from "react";
import { useSearchParams } from "react-router-dom";
import TabPage from "@/components/TabPage";
import { AboutNumbers } from "@/components/metrics/AboutNumbers";
import { MetricsCards } from "@/components/metrics/MetricsCards";
import { MetricsError, MetricsLoading } from "@/components/metrics/MetricsStatus";
import { QualityBlock } from "@/components/metrics/QualityBlock";
import { VersionGroups } from "@/components/metrics/VersionGroups";
import { Button } from "@/components/ui/button";
import { Segmented } from "@/components/ui/segmented";
import { useMetrics } from "@/hooks/useMetrics";
import type { MetricsReport } from "@/types";
import { metricsErrorKind } from "@/lib/metricsFormat";
import {
  FILTER_NAMES,
  agentMetricsQuery,
  compareQuery,
  metricsFilters,
  metricsView,
  periodParams,
  shorterThanQuarter,
  useMetricsWindow,
  type CompareKind,
  type MetricsFilters,
  type MetricsView,
  type MetricsWindow,
} from "@/lib/metricsQuery";
import { hostToParam } from "@/lib/terminalsHost";
import { cn } from "@/lib/utils";
import { AgentsTable } from "./AgentsTable";
import { FilterChips, MetricsToolbar } from "./MetricsToolbar";
import { PageHeading } from "./PageHeading";

const VIEWS = [
  { value: "agents", label: "Agents" },
  { value: "images", label: "Images" },
  { value: "workflows", label: "Workflows" },
] as const satisfies ReadonlyArray<{ value: MetricsView; label: string }>;

/** What shows in place of the numbers when the period has none. */
function EmptyPeriod({
  filtered,
  window,
  onQuarter,
  onClear,
}: {
  filtered: boolean;
  window: MetricsWindow;
  onQuarter: () => void;
  onClear: () => void;
}) {
  return (
    <div className="space-y-2 rounded-chip border p-4 text-body">
      <p>
        {filtered
          ? "No data for these filters in this period."
          : "No runs and no completed tasks in this period."}
      </p>
      {filtered ? (
        <Button type="button" size="sm" variant="outline" onClick={onClear}>
          Clear filters
        </Button>
      ) : (
        shorterThanQuarter(window) && (
          <Button type="button" size="sm" variant="outline" onClick={onQuarter}>
            Show 90 days
          </Button>
        )
      )}
    </div>
  );
}

function ReportBody({
  report,
  view,
  onView,
  agentsTable,
  compareHref,
}: {
  report: MetricsReport;
  view: MetricsView;
  onView: (view: MetricsView) => void;
  agentsTable: ReactNode;
  compareHref: (kind: CompareKind) => (name: string, a: string, b: string) => string;
}) {
  return (
    <>
      <MetricsCards slice={report.total} />
      <QualityBlock quality={report.total.tasks.quality} />
      <Segmented label="Metrics view" options={VIEWS} value={view} onChange={onView} />
      {view === "agents" && agentsTable}
      {view === "images" && (
        <VersionGroups kind="image" slices={report.images} compareHref={compareHref("image")} />
      )}
      {view === "workflows" && (
        <VersionGroups
          kind="workflow"
          slices={report.workflows}
          compareHref={compareHref("workflow")}
        />
      )}
    </>
  );
}

/** Completed tasks and AI usage of one host over a period, in total and by
 *  agent, image version and workflow version. The host is the surrounding
 *  ApiProvider's. */
export default function MetricsPage({
  hostId,
  hostControl,
  agents,
}: {
  hostId: string;
  hostControl?: ReactNode;
  /** The host's agents, for the Agent filter and the agent links. */
  agents: string[];
}) {
  const { window, setPeriod, setCustom, refresh } = useMetricsWindow();
  const [searchParams, setSearchParams] = useSearchParams();
  const view = metricsView(searchParams);
  const filters = metricsFilters(searchParams);
  const filtered = FILTER_NAMES.some((name) => filters[name]);
  const data = useMetrics(
    window.error ? null : { since: window.since, until: window.until, ...filters },
  );
  const errorKind = data.error ? metricsErrorKind(data.error) : null;

  const setParams = (changes: Record<string, string>) => {
    const next = new URLSearchParams(searchParams);
    for (const [key, value] of Object.entries(changes)) {
      if (value) next.set(key, value);
      else next.delete(key);
    }
    setSearchParams(next, { replace: true });
  };
  const onFilters = (changes: Partial<MetricsFilters>) => setParams(changes);
  const clear = () => onFilters({ agent: "", image: "", workflow: "" });
  const report = data.report;
  const empty =
    !!report && report.total.usage.iterations === 0 && report.total.tasks.tasks_done === 0;
  const barWindow =
    errorKind === "bad_time" ? { ...window, error: "Enter a valid start time." } : window;
  const compareHref = (kind: CompareKind) => (name: string, a: string, b: string) =>
    `/metrics/compare?${compareQuery(periodParams(searchParams, window), kind, name, a, b)}`;
  const agentHref = (agent: string) =>
    `/agents/${encodeURIComponent(hostToParam(hostId))}/${encodeURIComponent(agent)}/activity?${agentMetricsQuery(window)}`;
  const failed = errorKind && errorKind !== "bad_time" && errorKind !== "invalid_param";

  return (
    <TabPage className="overflow-auto">
      <PageHeading
        title="Metrics"
        subtitle="Completed tasks and AI usage by agent, image and workflow."
        updatedAt={data.updatedAt}
        updating={data.updating}
        disabled={!!window.error}
        onRefresh={() => {
          refresh();
          data.retry();
        }}
      />
      <MetricsToolbar
        hostControl={hostControl}
        window={barWindow}
        onPeriod={setPeriod}
        onCustom={setCustom}
        agents={agents}
        filters={filters}
        onFilters={onFilters}
      />
      <FilterChips
        filters={filters}
        onFilters={onFilters}
        invalid={errorKind === "invalid_param"}
      />
      {data.loading && <MetricsLoading />}
      {failed && <MetricsError error={data.error} onRetry={data.retry} />}
      {report && !failed && !errorKind && (
        <div aria-busy={data.updating} className={cn("space-y-4", data.updating && "opacity-60")}>
          {empty ? (
            <EmptyPeriod
              filtered={filtered}
              window={window}
              onQuarter={() => setPeriod("quarter")}
              onClear={clear}
            />
          ) : (
            <ReportBody
              report={report}
              view={view}
              onView={(next) => setParams({ view: next })}
              agentsTable={
                <AgentsTable slices={report.agents} agents={agents} agentHref={agentHref} />
              }
              compareHref={compareHref}
            />
          )}
        </div>
      )}
      <AboutNumbers />
    </TabPage>
  );
}
