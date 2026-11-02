import type { ReactNode } from "react";
import type { MetricsSlice } from "@/lib/api";
import {
  UNKNOWN_REASON,
  avgTokens,
  fmtAvg,
  fmtDuration,
  fmtMoney,
  fmtTokens,
  tokenSum,
  tokenTitle,
} from "@/lib/metricsFormat";
import { Known } from "./Unknown";

function Card({ label, value, detail }: { label: string; value: ReactNode; detail?: ReactNode }) {
  return (
    <div data-card={label} className="min-w-0 rounded border p-3">
      <div className="truncate text-xs text-muted-foreground">{label}</div>
      <div data-value className="mt-1 font-mono text-lg">
        {value}
      </div>
      {detail !== undefined && (
        <div data-detail className="text-xs text-muted-foreground">
          {detail}
        </div>
      )}
    </div>
  );
}

function Group({
  title,
  caption,
  children,
}: {
  title: string;
  caption: string;
  children: ReactNode;
}) {
  const id = `metrics-group-${title.replace(/\W+/g, "-").toLowerCase()}`;
  return (
    <section aria-labelledby={id} className="space-y-2">
      <div>
        <h2 id={id} className="text-sm font-medium">
          {title}
        </h2>
        <p className="text-xs text-muted-foreground">{caption}</p>
      </div>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4">{children}</div>
    </section>
  );
}

const perTask = (value: number | undefined, format: (v: number) => string) => (
  <Known value={value} format={format} reason={UNKNOWN_REASON.perTask} />
);

/** The cards of one slice: what its completed tasks took over their whole
 *  life, and, unless `usage` is false, the AI usage made in the period. */
export function MetricsCards({ slice, usage = true }: { slice: MetricsSlice; usage?: boolean }) {
  const { tasks } = slice;
  const avg = tasks.avg;
  const total = (n: number) => `total ${n}`;
  const avgTok = avg ? avgTokens(avg) : undefined;
  return (
    <div className="space-y-4">
      <Group
        title="Completed tasks"
        caption="Tasks completed in this period. Runs and cost count each task's whole life."
      >
        <Card label="Tasks done" value={tasks.tasks_done} />
        <Card
          label="Lead time"
          value={
            <Known
              value={tasks.lead_time_median_s}
              format={fmtDuration}
              reason={UNKNOWN_REASON.leadTime}
            />
          }
          detail={
            <>
              avg{" "}
              <Known
                value={tasks.lead_time_avg_s}
                format={fmtDuration}
                reason={UNKNOWN_REASON.leadTime}
              />
            </>
          }
        />
        <Card
          label="Cost per task"
          value={perTask(avg?.cost_usd, fmtMoney)}
          detail={`total ${fmtMoney(tasks.sum.cost_usd)}`}
        />
        <Card
          label="Runs per task"
          value={perTask(avg?.iterations, fmtAvg)}
          detail={total(tasks.sum.iterations)}
        />
        <Card
          label="Rework per task"
          value={perTask(avg?.rework_rounds, fmtAvg)}
          detail={total(tasks.sum.rework_rounds)}
        />
        <Card
          label="Rejected advances per task"
          value={perTask(avg?.rejected_advances, fmtAvg)}
          detail={total(tasks.sum.rejected_advances)}
        />
        <Card
          label="Questions per task"
          value={perTask(avg?.customer_questions, fmtAvg)}
          detail={total(tasks.sum.customer_questions)}
        />
        <Card
          label="Tokens per task"
          value={
            avgTok ? (
              <span title={tokenTitle(avgTok)}>{fmtTokens(tokenSum(avgTok))}</span>
            ) : (
              perTask(undefined, fmtTokens)
            )
          }
          detail={
            <span title={tokenTitle(tasks.sum.tokens)}>
              total {fmtTokens(tokenSum(tasks.sum.tokens))}
            </span>
          }
        />
      </Group>
      {tasks.tasks_done === 0 && (
        <p className="text-sm text-muted-foreground">No task was completed in this period.</p>
      )}
      {usage && (
        <Group
          title="AI usage in this period"
          caption="Runs and cost of AI requests made in this period."
        >
          <Card label="Runs" value={slice.usage.iterations} />
          <Card label="Cost" value={fmtMoney(slice.usage.cost_usd)} />
          <Card
            label="Tokens"
            value={
              <span title={tokenTitle(slice.usage.tokens)}>
                {fmtTokens(tokenSum(slice.usage.tokens))}
              </span>
            }
          />
        </Group>
      )}
      <p className="text-xs text-muted-foreground">
        — means the value is not known. It is not zero.
      </p>
    </div>
  );
}
