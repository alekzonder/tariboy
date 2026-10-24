import { useEffect, useState } from "react";
import type { CodexApprovalMode } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Segmented } from "@/components/ui/segmented";

const reason = (e: unknown) => (e instanceof Error ? e.message : String(e));

const options = [
  { value: "yolo", label: "Yolo" },
  { value: "approve-for-me", label: "Approve for me" },
] as const;

const help: Record<CodexApprovalMode, string> = {
  yolo: "--yolo: no sandbox and no approval prompts; commands have full file and network access.",
  "approve-for-me":
    "--approve-for-me: commands run in the workspace-write sandbox and Codex reviews approval requests itself. " +
    "Writes outside the working directory and network access can be refused.",
};

// Mount with a per-host key (and inside that host's ApiProvider) so one host's
// mode is never shown for another.
export function CodexApprovalCard() {
  const api = useApi();
  const [mode, setMode] = useState<CodexApprovalMode | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    let current = true;
    api.harnesses
      .getCodexApproval()
      .then((r) => {
        if (current) setMode(r.mode);
      })
      .catch((e) => {
        if (current) setError(reason(e));
      });
    return () => {
      current = false;
    };
  }, [api]);

  const change = async (next: CodexApprovalMode) => {
    if (busy || next === mode) return;
    setBusy(true);
    setError("");
    try {
      setMode((await api.harnesses.setCodexApproval(next)).mode);
    } catch (e) {
      setError(reason(e));
    }
    setBusy(false);
  };

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">Codex</CardTitle>
        <CardDescription>
          How Codex agents on this host handle command approvals. Applies from each agent&apos;s
          next iteration.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {mode && (
          <>
            <Segmented
              label="Codex approval mode"
              options={options}
              value={mode}
              onChange={(v) => void change(v)}
              className="w-fit"
            />
            <p className="text-xs text-muted-foreground">{help[mode]}</p>
          </>
        )}
        {error && (
          <p role="alert" className="text-xs text-destructive">
            {error}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
