import type { HarnessCatalog } from "../types";
import type { Transport } from "./client";

export type CodexApprovalMode = "yolo" | "approve-for-me";

export function harnessesApi(t: Transport) {
  return {
    // The models and efforts the harness CLI on that host supports. CLI
    // failures arrive in the error field of a 200 response.
    getHarnessModels: (harness: string) =>
      t.get<HarnessCatalog>(`/api/harnesses/${encodeURIComponent(harness)}/models`),
    getCodexApproval: () => t.get<{ mode: CodexApprovalMode }>("/api/harnesses/codex/approval-mode"),
    setCodexApproval: (mode: CodexApprovalMode) =>
      t.post<{ mode: CodexApprovalMode }>("/api/harnesses/codex/approval-mode", { mode }),
  };
}
export type HarnessesApi = ReturnType<typeof harnessesApi>;
