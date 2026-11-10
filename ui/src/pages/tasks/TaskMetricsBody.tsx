import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { useApi } from "@/components/apiContext";
import { MetricsError } from "@/components/metrics/MetricsStatus";
import { QualityBlock } from "@/components/metrics/QualityBlock";
import { CELL, STICKY_CELL } from "@/components/ui/recipes";
import { Known, Unknown } from "@/components/metrics/Unknown";
import type { useTaskMetrics } from "@/hooks/useTaskMetrics";
import { ApiError, type Task, type TaskMetrics } from "@/lib/api";
import {
  UNKNOWN_REASON,
  VERSION_HINT,
  byVersionDesc,
  fmtDuration,
  fmtMoney,
  fmtTokens,
  metricsErrorKind,
  tokenSum,
  tokenTitle,
  versionLabel,
  workflowLabel,
} from "@/lib/metricsFormat";
import { compareQuery, type CompareKind } from "@/lib/metricsQuery";
import { TONE_DOT, flexibleStatusLabel, taskTone } from "@/lib/statusTone";
import { isWorkflowTask, taskStatusLabel } from "@/lib/tasks";
import { hostToParam } from "@/lib/terminalsHost";
import { cn } from "@/lib/utils";
import { EMPTY, LABEL } from "@/components/ui/recipes";

const NO_WORKFLOW = "This task has no workflow, so it has no rework rounds.";

function Heading({ id, children }: { id: string; children: ReactNode }) {
  return (
    <h3 id={id} className={LABEL}>
      {children}
    </h3>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <dt className="text-ui text-muted-foreground">{label}</dt>
      <dd className="min-w-0 font-mono text-caption tabular-nums">{children}</dd>
    </>
  );
}

function time(value: string, reason: string) {
  if (!value) return <Unknown reason={reason} />;
  return <time dateTime={value}>{new Date(value).toLocaleString()}</time>;
}

function Summary({ task, metrics }: { task: Task; metrics: TaskMetrics }) {
  const done = task.category === "done";
  const workflow = isWorkflowTask(task);
  const { rework_by_outcome: by, usage } = metrics;
  const reworkCount = (value: number) =>
    workflow ? value : <span title={NO_WORKFLOW}>{value}</span>;
  return (
    <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1.5">
      <Field label="Lead time">
        {metrics.lead_time_s === null && !done ? (
          "Not done yet"
        ) : (
          <Known
            value={metrics.lead_time_s}
            format={fmtDuration}
            reason={UNKNOWN_REASON.taskLeadTime}
          />
        )}
      </Field>
      <Field label="Cost">{fmtMoney(usage.cost_usd)}</Field>
      <Field label="Runs">{usage.iterations}</Field>
      <Field label="Tokens">
        <span title={tokenTitle(usage.tokens)}>{fmtTokens(tokenSum(usage.tokens))}</span>
      </Field>
      <Field label="Rework rounds">
        {reworkCount(metrics.rework_rounds)}
        {workflow && (
          <div className="font-sans text-caption text-muted-foreground">
            Changes requested {by.changes_requested} · Rework {by.rework} · Conflict {by.conflict}
          </div>
        )}
      </Field>
      <Field label="Rejected advances">{reworkCount(metrics.rejected_advances)}</Field>
      <Field label="Questions to customer">{metrics.customer_questions}</Field>
      <Field label="Started">{time(metrics.started_at, "The task has not started yet.")}</Field>
      <Field label="Completed">
        {time(
          metrics.completed_at,
          done ? "The task has no completion time." : "The task is not done yet.",
        )}
      </Field>
    </dl>
  );
}

function TimeByStatus({ task, metrics }: { task: Task; metrics: TaskMetrics }) {
  const entries = Object.entries(metrics.status_time_s).sort((a, b) => b[1] - a[1]);
  const total = entries.reduce((sum, [, seconds]) => sum + seconds, 0);
  const workflow = isWorkflowTask(task);
  const label = (status: string) =>
    workflow ? taskStatusLabel({ ...task, status }) : flexibleStatusLabel(status);
  // A workflow status has no tone of its own; only the current one is known.
  const tone = (status: string) => {
    if (!workflow) return taskTone(status);
    return status === metrics.status ? taskTone(task.category) : "quiet";
  };
  return (
    <section aria-labelledby="task-metrics-status" className="flex flex-col gap-2">
      <Heading id="task-metrics-status">Time by status</Heading>
      {total === 0 ? (
        <p className={EMPTY}>No time in a status yet.</p>
      ) : (
        <>
          <div aria-hidden="true" className="flex h-2 gap-0.5 overflow-hidden rounded-chip">
            {entries.map(([status, seconds]) => (
              <span
                key={status}
                className={cn("h-full", TONE_DOT[tone(status)])}
                style={{ width: `${(seconds / total) * 100}%` }}
              />
            ))}
          </div>
          <ul aria-label="Time by status" className="flex flex-col gap-1">
            {entries.map(([status, seconds]) => (
              <li key={status} className="flex min-w-0 flex-wrap items-center gap-x-2 text-ui">
                <span
                  aria-hidden="true"
                  className={cn("size-2 shrink-0 rounded-full", TONE_DOT[tone(status)])}
                />
                <span className="min-w-0 break-words">{label(status)}</span>
                <span className="font-mono text-caption tabular-nums">{fmtDuration(seconds)}</span>
                <span className="font-mono text-caption text-muted-foreground tabular-nums">
                  {Math.round((seconds / total) * 100)}%
                </span>
                {status === metrics.status && (
                  <span className="text-caption text-muted-foreground">
                    current, still counting
                  </span>
                )}
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  );
}

const imageText = (image: string, version: string) => `${image} · ${versionLabel(version)}`;

function Agents({ metrics }: { metrics: TaskMetrics }) {
  return (
    <section aria-labelledby="task-metrics-agents" className="flex flex-col gap-2">
      <Heading id="task-metrics-agents">Agents</Heading>
      {metrics.agents.length === 0 ? (
        <p className={EMPTY}>No agent runs recorded.</p>
      ) : (
        <div className="overflow-x-auto rounded-chip border">
          <table aria-label="Agents" className="w-full text-body">
            <thead className="bg-muted text-left text-ui text-muted-foreground">
              <tr>
                <th scope="col" className={cn(CELL, STICKY_CELL, "bg-muted font-normal")}>
                  Agent
                </th>
                <th scope="col" className={cn(CELL, "text-right font-normal")}>
                  Runs
                </th>
                <th scope="col" className={cn(CELL, "text-right font-normal")}>
                  Cost
                </th>
                <th scope="col" className={cn(CELL, "font-normal")}>
                  Image versions
                </th>
              </tr>
            </thead>
            <tbody>
              {metrics.agents.map((row) => (
                <tr key={row.agent} className="border-t">
                  <th scope="row" className={cn(CELL, STICKY_CELL, "text-left font-normal")}>
                    {row.agent}
                  </th>
                  <td className={cn(CELL, "text-right font-mono")}>{row.usage.iterations}</td>
                  <td className={cn(CELL, "text-right font-mono")}>
                    {fmtMoney(row.usage.cost_usd)}
                  </td>
                  <td className={cn(CELL, "font-mono")}>
                    {row.images.map((i) => imageText(i.image, i.image_version)).join(", ")}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

/** The known versions of each name that has two or more, newest first. */
function mixedVersions(pairs: Array<{ name: string; version: string }>): Map<string, string[]> {
  const byName = new Map<string, Set<string>>();
  for (const { name, version } of pairs) {
    if (version === "unknown" || !version) continue;
    byName.set(name, (byName.get(name) ?? new Set()).add(version));
  }
  const mixed = new Map<string, string[]>();
  for (const [name, versions] of byName) {
    if (versions.size > 1) mixed.set(name, [...versions].sort(byVersionDesc));
  }
  return mixed;
}

function Mixed({
  kind,
  pairs,
}: {
  kind: CompareKind;
  pairs: Array<{ name: string; version: string }>;
}) {
  const api = useApi();
  const base = new URLSearchParams({ host: hostToParam(api.target?.id ?? ""), period: "quarter" });
  return [...mixedVersions(pairs)].map(([name, versions]) => (
    <p key={`${kind}:${name}`} className="text-ui text-muted-foreground">
      This task ran on {versions.length} {kind} versions. Version comparisons do not count it.{" "}
      <Link
        to={`/metrics/compare?${compareQuery(base, kind, name, versions[versions.length - 1], versions[0])}`}
        className="text-primary hover:underline"
      >
        Compare these versions
      </Link>
    </p>
  ));
}

function Versions({ metrics }: { metrics: TaskMetrics }) {
  const workflows = metrics.workflows.filter((w) => w.name !== "none" && w.name !== "unknown");
  return (
    <section aria-labelledby="task-metrics-versions" className="flex flex-col gap-2">
      <Heading id="task-metrics-versions">Versions</Heading>
      <ul aria-label="Images" className="flex flex-col gap-1 text-ui">
        {metrics.images.map((i) => (
          <li key={`${i.image}:${i.image_version}`} className="flex flex-wrap gap-x-2">
            <span className="min-w-0 break-all">
              <Link
                to={`/library/images/${encodeURIComponent(i.image)}`}
                className="text-primary hover:underline"
              >
                {i.image}
              </Link>{" "}
              ·{" "}
              <span
                title={i.image_version === "unknown" ? VERSION_HINT.unknown : undefined}
                className={cn(
                  "font-mono",
                  i.image_version === "unknown" && "text-muted-foreground",
                )}
              >
                {versionLabel(i.image_version)}
              </span>
            </span>
            <span className="font-mono text-caption tabular-nums">{i.iterations} runs</span>
            <span className="font-mono text-caption tabular-nums">{fmtMoney(i.cost_usd)}</span>
          </li>
        ))}
      </ul>
      {metrics.workflows.length === 0 ? (
        <p className={EMPTY}>No workflow</p>
      ) : (
        <ul aria-label="Workflows" className="flex flex-col gap-1 text-ui">
          {metrics.workflows.map((w) => (
            <li key={`${w.name}:${w.version}:${w.digest}`} className="break-all">
              {workflowLabel(w.name)}
              {w.version ? ` · ${w.version}` : ""}
            </li>
          ))}
        </ul>
      )}
      <Mixed
        kind="image"
        pairs={metrics.images.map((i) => ({ name: i.image, version: i.image_version }))}
      />
      <Mixed kind="workflow" pairs={workflows} />
    </section>
  );
}

/** What stands in place of the numbers: the load, a missing task, an older
 *  host, or an error with Retry. */
function Status({ state }: { state: ReturnType<typeof useTaskMetrics> }) {
  if (state.loading) {
    return (
      <p role="status" className={EMPTY}>
        Loading metrics…
      </p>
    );
  }
  const { error } = state;
  const missing =
    error instanceof ApiError && error.code === "not_found" && metricsErrorKind(error) === "other";
  if (missing) return <p className={EMPTY}>Metrics are not available for this task.</p>;
  return <MetricsError error={error} onRetry={state.retry} title="Could not load task metrics." />;
}

/** The open Metrics section: summary, time by status, agents, versions and
 *  Quality of one task. */
export default function TaskMetricsBody({
  task,
  state,
}: {
  task: Task;
  state: ReturnType<typeof useTaskMetrics>;
}) {
  const { metrics } = state;
  if (!metrics || state.error) return <Status state={state} />;
  return (
    <div className={cn("flex flex-col gap-4 pt-2", state.updating && "opacity-60")}>
      {state.updating && <p className="text-caption text-muted-foreground">Updating…</p>}
      <Summary task={task} metrics={metrics} />
      <TimeByStatus task={task} metrics={metrics} />
      <Agents metrics={metrics} />
      <Versions metrics={metrics} />
      <QualityBlock quality={metrics.quality} />
    </div>
  );
}
