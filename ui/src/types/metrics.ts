export type {
  MetricsQuality,
  MetricsReport,
  MetricsSlice,
  MetricsTokens,
  TaskMetrics,
} from "@/lib/api-types.generated";

/** The period and filters of `GET /api/metrics`. `image` and `workflow` are
 *  `NAME` or `NAME:VERSION`; `workflow` may be `none`. */
export interface MetricsFilter {
  since: string;
  until: string;
  agent?: string;
  image?: string;
  workflow?: string;
}
