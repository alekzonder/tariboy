import type { ReactNode } from "react";
import { Link, Navigate, useSearchParams } from "react-router-dom";
import { ArrowLeftRight } from "lucide-react";
import TabPage from "@/components/TabPage";
import { AboutNumbers } from "@/components/metrics/AboutNumbers";
import { MetricsError, MetricsLoading } from "@/components/metrics/MetricsStatus";
import { Button } from "@/components/ui/button";
import { useMetrics } from "@/hooks/useMetrics";
import type { MetricsReport, MetricsSlice } from "@/lib/api";
import { byVersionDesc, metricsErrorKind, versionLabel, workflowLabel } from "@/lib/metricsFormat";
import {
  METRICS_PERIODS,
  defaultPair,
  metricsWindow,
  parseCompare,
  periodParams,
  shorterThanQuarter,
  useMetricsWindow,
  type CompareQuery,
  type MetricsPeriod,
  type MetricsWindow,
} from "@/lib/metricsQuery";
import { ActivityPeriodBar } from "@/pages/activity/ActivityParts";
import { CompareTable } from "./CompareTable";
import { compareRows, smallSample } from "./compareModel";
import { PageHeading } from "./PageHeading";

const SELECT = "h-7 max-w-40 min-w-0 rounded-control border bg-background px-2 text-body-sm";

function VersionSelect({
  label,
  value,
  versions,
  error,
  onChange,
}: {
  label: string;
  value: string;
  versions: string[];
  error?: string;
  onChange: (value: string) => void;
}) {
  // A version of the URL without data in the period stays selected.
  const options = value && !versions.includes(value) ? [...versions, value] : versions;
  const errorId = `${label.replace(/\s/g, "-")}-error`;
  return (
    <label className="flex shrink-0 items-center gap-1 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <select
        aria-label={label}
        aria-invalid={!!error}
        aria-describedby={error ? errorId : undefined}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className={SELECT}
      >
        {!value && <option value="">Choose…</option>}
        {options.map((v) => (
          <option key={v} value={v}>
            {versionLabel(v)}
          </option>
        ))}
      </select>
      {error && (
        <span id={errorId} role="alert" className="text-xs text-destructive">
          {error}
        </span>
      )}
    </label>
  );
}

function MixedTasks({ a, b }: { a?: MetricsSlice; b?: MetricsSlice }) {
  const [ma, mb] = [a?.tasks.mixed_tasks ?? 0, b?.tasks.mixed_tasks ?? 0];
  return (
    <section aria-labelledby="metrics-mixed" className="space-y-1 rounded border p-3 text-sm">
      <h2 id="metrics-mixed" className="font-medium">
        Mixed tasks
      </h2>
      <p className="text-muted-foreground">
        Tasks that ran on more than one version are not counted above.
      </p>
      {ma === 0 && mb === 0 ? (
        <p>No tasks ran on more than one version.</p>
      ) : (
        <p className="font-mono">
          Version A: {ma} · Version B: {mb}
        </p>
      )}
    </section>
  );
}

/** The table, its notes and the mixed tasks, once A and B are two versions. */
function Comparison({ slices, a, b }: { slices: MetricsSlice[]; a: string; b: string }) {
  const sliceA = slices.find((s) => s.version === a);
  const sliceB = slices.find((s) => s.version === b);
  const missing = [a, b].filter((v) => !slices.some((s) => s.version === v));
  const warning = smallSample(sliceA, sliceB);
  return (
    <>
      <p className="text-xs text-muted-foreground">Change shows version B minus version A.</p>
      {missing.map((v) => (
        <p key={v} className="text-sm">
          Version {versionLabel(v)} has no data in this period.
        </p>
      ))}
      {warning && <p className="text-sm">{warning}</p>}
      <CompareTable rows={compareRows(sliceA, sliceB)} a={a} b={b} />
      <MixedTasks a={sliceA} b={sliceB} />
    </>
  );
}

type Pair = Pick<CompareQuery, "a" | "b">;

/** The name's slices in the report, its versions newest first, and the pair:
 *  the URL's, or the newest and the one before. */
function comparedSlices(report: MetricsReport | null, compare: CompareQuery) {
  const list = report ? (compare.kind === "image" ? report.images : report.workflows) : null;
  const slices = list?.filter((s) => s.name === compare.name);
  const versions = (slices ?? []).map((s) => s.version ?? "").sort(byVersionDesc);
  const pair: Pair = compare.a || compare.b ? compare : defaultPair(versions);
  return { slices, versions, pair };
}

/** What the period holds for the name: nothing, one version, the same version
 *  twice, or a comparison. */
function CompareBody({
  slices,
  name,
  pair,
  window,
  onQuarter,
}: {
  slices: MetricsSlice[];
  name: string;
  pair: Pair;
  window: MetricsWindow;
  onQuarter: () => void;
}) {
  if (slices.length === 0) {
    return (
      <div className="space-y-2 rounded border p-4 text-sm">
        <p>No runs of {name} in this period.</p>
        {shorterThanQuarter(window) && (
          <Button type="button" size="sm" variant="outline" onClick={onQuarter}>
            Show 90 days
          </Button>
        )}
      </div>
    );
  }
  if (!pair.a || !pair.b) return <p className="text-sm">Choose a version to compare with.</p>;
  if (pair.a === pair.b) return null;
  return <Comparison slices={slices} a={pair.a} b={pair.b} />;
}

function CompareToolbar({
  hostControl,
  window,
  onPeriod,
  onCustom,
  versions,
  pair,
  onPair,
}: {
  hostControl?: ReactNode;
  window: MetricsWindow;
  onPeriod: (period: MetricsPeriod) => void;
  onCustom: (bound: "since" | "until", value: string) => void;
  versions: string[];
  pair: Pair;
  onPair: (a: string, b: string) => void;
}) {
  const same = !!pair.a && pair.a === pair.b;
  return (
    <div
      role="toolbar"
      aria-label="Compared versions"
      data-toolbar
      className="flex flex-wrap items-center gap-2 lg:flex-nowrap"
    >
      {hostControl && <div className="flex shrink-0 items-center">{hostControl}</div>}
      <div className="scrollbar-none min-w-0 shrink overflow-x-auto">
        <ActivityPeriodBar
          window={window}
          onPeriod={onPeriod}
          onCustom={onCustom}
          periods={METRICS_PERIODS}
        />
      </div>
      <VersionSelect
        label="Version A"
        value={pair.a}
        versions={versions}
        onChange={(a) => onPair(a, pair.b)}
      />
      <Button type="button" size="sm" variant="outline" onClick={() => onPair(pair.b, pair.a)}>
        <ArrowLeftRight aria-hidden="true" />
        Swap
      </Button>
      <VersionSelect
        label="Version B"
        value={pair.b}
        versions={versions}
        error={same ? "Choose a different version." : undefined}
        onChange={(b) => onPair(pair.a, b)}
      />
    </div>
  );
}

function CompareScreen({
  compare,
  hostControl,
}: {
  compare: CompareQuery;
  hostControl?: ReactNode;
}) {
  const { window, setPeriod, setCustom, refresh } = useMetricsWindow();
  const [searchParams, setSearchParams] = useSearchParams();
  const data = useMetrics(
    window.error
      ? null
      : { since: window.since, until: window.until, [compare.kind]: compare.name },
  );
  const { slices, versions, pair } = comparedSlices(data.report, compare);
  // Both versions go to the URL, so a reload shows the same pair.
  const setPair = (a: string, b: string) => {
    const next = new URLSearchParams(searchParams);
    next.set("a", a);
    next.set("b", b);
    setSearchParams(next, { replace: true });
  };
  const name = compare.kind === "workflow" ? workflowLabel(compare.name) : compare.name;
  const errorKind = data.error ? metricsErrorKind(data.error) : null;
  const barWindow =
    errorKind === "bad_time" ? { ...window, error: "Enter a valid start time." } : window;

  return (
    <TabPage className="overflow-auto">
      <Link
        to={`/metrics?${periodParams(searchParams, window)}`}
        className="text-sm text-primary hover:underline"
      >
        ← Metrics
      </Link>
      <PageHeading
        title={compare.kind === "image" ? "Compare image versions" : "Compare workflow versions"}
        subtitle={name}
        updatedAt={data.updatedAt}
        updating={data.updating}
        disabled={!!window.error}
        onRefresh={() => {
          refresh();
          data.retry();
        }}
      />
      <CompareToolbar
        hostControl={hostControl}
        window={barWindow}
        onPeriod={setPeriod}
        onCustom={setCustom}
        versions={versions}
        pair={pair}
        onPair={setPair}
      />
      {data.loading && <MetricsLoading />}
      {errorKind && errorKind !== "bad_time" && (
        <MetricsError error={data.error} onRetry={data.retry} />
      )}
      {slices && !errorKind && (
        <div
          aria-busy={data.updating}
          className={data.updating ? "space-y-4 opacity-60" : "space-y-4"}
        >
          <CompareBody
            slices={slices}
            name={name}
            pair={pair}
            window={window}
            onQuarter={() => setPeriod("quarter")}
          />
        </div>
      )}
      <AboutNumbers />
    </TabPage>
  );
}

/** Two versions of one image or workflow side by side, over a period. Without
 *  a kind or a name it opens /metrics with the same host and period. */
export default function MetricsComparePage({ hostControl }: { hostControl?: ReactNode }) {
  const [searchParams] = useSearchParams();
  const compare = parseCompare(searchParams);
  if (!compare) {
    const back = periodParams(searchParams, metricsWindow(searchParams, new Date()));
    return <Navigate to={`/metrics?${back}`} replace />;
  }
  return <CompareScreen compare={compare} hostControl={hostControl} />;
}
