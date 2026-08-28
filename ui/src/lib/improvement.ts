import { apiGet, apiPost } from "@/lib/api";

export interface ImprovementCitation { bundle_hash: string; artifact: string; locator: string }
export interface ImprovementProposal {
  id: string; judge_run_id: string; revision_hash: string; status: string;
  draft: {
    subject_ids: string[];
    target: { repository: string; base_commit: string; image: string; image_digest: string };
    findings: { severity: string; criterion: string; observation: string; evidence: ImprovementCitation[] }[];
    changes: { file: string; intent: string }[];
    acceptance: string[]; risk: string; rollback_image: string;
  };
}
export interface ImageRelease { id: string; status: string; repository_id: string; git_commit: string; source_digest: string; lock_digest: string; prompt_template_digest: string; image_ref: string; image_digest: string; release_hash: string }
export interface ImprovementDetail { proposal: ImprovementProposal; releases: ImageRelease[] }

export const getImprovement = (id: string) => apiGet<ImprovementDetail>(`/api/improvements/${encodeURIComponent(id)}`);
export const decideImprovementPlan = (id: string, decision: "approve" | "reject", revision: string, reason = "") => apiPost(`/api/improvements/${encodeURIComponent(id)}/plan/${decision}`, { revision, reason });
export const decideReleaseRollout = (id: string, decision: "approve" | "reject", releaseHash: string, reason = "") => apiPost(`/api/image-releases/${encodeURIComponent(id)}/rollout/${decision}`, { "release-hash": releaseHash, reason });
export const stageReleaseRollout = (id: string, agent: string, releaseHash: string) => apiPost(`/api/image-releases/${encodeURIComponent(id)}/rollout/stage`, { agent, "release-hash": releaseHash });
