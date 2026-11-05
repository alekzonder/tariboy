import type { HarnessCodexApproval, HarnessModelCatalog } from "../api-types.generated";
import type { CodexApprovalMode } from "@/types/harnesses";
import type { Transport } from "./client";

export type * from "@/types/harnesses";

export function harnessesApi(t: Transport) {
  return {
    // The models and efforts the harness CLI on that host supports. CLI
    // failures arrive in the error field of a 200 response.
    getHarnessModels: (harness: string) =>
      t.get<HarnessModelCatalog>(`/api/harnesses/${encodeURIComponent(harness)}/models`),
    getCodexApproval: () => t.get<HarnessCodexApproval>("/api/harnesses/codex/approval-mode"),
    setCodexApproval: (mode: CodexApprovalMode) =>
      t.post<HarnessCodexApproval>("/api/harnesses/codex/approval-mode", { mode }),
  };
}
