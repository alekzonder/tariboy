import { apiGet, apiPost } from "@/lib/api";

export type JudgeRunStatus =
  | "snapshotting"
  | "running"
  | "summarizing"
  | "completed"
  | "partial"
  | "cancelled";

// These mirror the operator API. A few installations additionally expose the
// optional model/cost fields; keeping them optional makes the list compatible
// with older daemons while still rendering the richer response when present.
export interface JudgeRun {
  id: string;
  created_at: string;
  updated_at: string;
  creator_iteration: string;
  original_request?: string;
  judge_group: string;
  lead_agent: string;
  judge_agents?: string[];
  summary_agent: string;
  judges_per_iteration: number;
  max_attempts: number;
  status: JudgeRunStatus;
  targets_total: number;
  targets_ready: number;
  assignments_total: number;
  assignments_completed: number;
  current_summary_version: number;
  last_error: string;
  model?: string;
  cost_usd?: number;
  manifest_hash?: string;
}

export interface JudgeTarget {
  id: string; iteration: string; agent: string; sequence: number;
  bundle_hash: string; snapshot_status: string; target_state: string;
  consensus_verdict: string; consensus_score?: number;
  assignments_completed: number; assignments_failed: number; assignments_pending: number;
}

export interface JudgeCitation { bundle_hash?: string; artifact: string; locator: string }
export interface JudgeFinding { criterion?: string; severity?: string; description: string; citations?: JudgeCitation[] }
export interface JudgeAnalysis {
  id: string; target_id: string; judge_agent: string; created_at: string;
  result: { verdict: string; score: number; confidence: number; summary: string; violations?: JudgeFinding[]; strengths?: JudgeFinding[]; recommendations?: { description: string }[]; evidence_gaps?: string[] };
}
export interface JudgeSummary { id: string; version: number; created_at: string; summary_agent: string; result: { executive_conclusion?: string; coverage?: Record<string, number>; cross_iteration_patterns?: string[]; recurring_violations?: string[]; strengths?: string[]; recommendations?: string[] } }
export interface JudgeUsage { iteration: string; requests: number; input_tokens: number; output_tokens: number; cache_write_tokens: number; cache_read_tokens: number; cost_usd: number }
export interface JudgeRunDetail { run: JudgeRun; targets: JudgeTarget[]; analyses: JudgeAnalysis[]; summaries: JudgeSummary[]; usage: JudgeUsage[] }

export interface JudgeRunList {
  runs: JudgeRun[];
  count: number;
}

export const listJudgeRuns = () => apiGet<JudgeRunList>("/api/judges");
export const getJudgeRun = (id: string) => apiGet<JudgeRunDetail>(`/api/judges/${encodeURIComponent(id)}`);
export const getJudgeEvidence = (runID: string, targetID: string, artifact: string, locator: string) => {
  const q = new URLSearchParams({ artifact, locator });
  return apiGet<{ evidence: Record<string, unknown> }>(`/api/judges/${encodeURIComponent(runID)}/targets/${encodeURIComponent(targetID)}/evidence?${q}`);
};
export const retryJudgeRun = (id: string) => apiPost<{ id: string; retried: boolean }>(`/api/judges/${encodeURIComponent(id)}/retry`);
export const cancelJudgeRun = (id: string) => apiPost<{ id: string; cancelled: boolean }>(`/api/judges/${encodeURIComponent(id)}/cancel`);
