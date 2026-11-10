import type { MetricsQuality } from "@/types";
import { QUALITY_AXES } from "@/lib/metricsFormat";

/** The assessment scores of a slice or a task: one row per axis, or an empty
 *  state while nothing is scored. */
export function QualityBlock({ quality }: { quality: MetricsQuality }) {
  return (
    <section aria-labelledby="metrics-quality" className="space-y-2 rounded-chip border p-3">
      <div>
        <h2 id="metrics-quality" className="text-body font-medium">
          Quality
        </h2>
        <p className="text-ui text-muted-foreground">Average assessment score from 0 to 100.</p>
      </div>
      {quality === null ? (
        <div>
          <p className="text-body font-medium">No quality scores yet</p>
          <p className="text-body text-muted-foreground">
            Scores appear here after tasks are assessed.
          </p>
        </div>
      ) : (
        <ul className="space-y-2">
          {QUALITY_AXES.map(({ key, label }) => {
            const axis = quality[key];
            return (
              <li
                key={key}
                aria-label={label}
                className="grid grid-cols-1 items-center gap-1 sm:grid-cols-4 sm:gap-3"
              >
                <span className="text-body">{label}</span>
                {axis ? (
                  <>
                    <span className="font-mono text-heading">{Math.round(axis.avg)}</span>
                    <span aria-hidden="true" className="h-2 overflow-hidden rounded-chip bg-muted">
                      <span
                        className="block h-full bg-primary"
                        style={{ width: `${Math.max(0, Math.min(100, axis.avg))}%` }}
                      />
                    </span>
                    <span className="text-ui text-muted-foreground">
                      min {Math.round(axis.min)} · {axis.count} scores
                    </span>
                  </>
                ) : (
                  <span className="text-body text-muted-foreground sm:col-span-3">No scores</span>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
