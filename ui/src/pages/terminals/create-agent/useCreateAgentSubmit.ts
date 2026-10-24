import { useState } from "react";
import { toast } from "sonner";
import { rememberRuntimePreset } from "@/lib/runtimePresets";
import type { AgentCreateDraft } from "../agentCreateDraft";
import type { CreateAgentDialogProps } from "../CreateAgentDialog";
import type { useCreateAgentResources } from "./useCreateAgentResources";
import { buildCreateAgentSpec } from "./createAgentSpec";
import { errorMessage } from "./errorMessage";

export function useCreateAgentSubmit(
  draft: AgentCreateDraft,
  { host, api, targetIsUsable, bare, schemaV2, defaultModel, defaultEffort }: ReturnType<typeof useCreateAgentResources>,
  blocker: { message: string } | null,
  { onCreated, onOpenChange }: CreateAgentDialogProps,
) {
  const [busy, setBusy] = useState(false);
  const [submitError, setSubmitError] = useState("");
  const [startError, setStartError] = useState("");
  const [created, setCreated] = useState<{ host: string; name: string } | null>(null);
  const submit = async () => {
    if (blocker || !targetIsUsable) {
      setSubmitError(blocker?.message ?? "The selected host is not ready");
      return;
    }
    let spec;
    try {
      spec = buildCreateAgentSpec(draft, bare, schemaV2);
    } catch (error) {
      setSubmitError(errorMessage(error));
      return;
    }

    setBusy(true);
    setSubmitError("");
    setStartError("");
    try {
      const result = await api.agents.createAgent(spec);
      if (draft.model.trim() && draft.model.trim() !== defaultModel) {
        rememberRuntimePreset(draft.harness, "models", draft.model);
      }
      if (draft.effort.trim() && draft.effort.trim() !== defaultEffort) {
        rememberRuntimePreset(draft.harness, "efforts", draft.effort);
      }
      const nextCreated = { host, name: result.name };
      setCreated(nextCreated);
      onCreated(host, result.name);
      if (!draft.startNow) {
        onOpenChange(false);
        return;
      }
      try {
        await api.agents.startAgent(result.name);
        onOpenChange(false);
      } catch (error) {
        setStartError(
          `Agent created but could not be started: ${errorMessage(error)}. Check the host and retry start.`,
        );
      }
    } catch (error) {
      const message = errorMessage(error);
      setSubmitError(message);
      toast.error(`create failed: ${message}`);
    } finally {
      setBusy(false);
    }
  };

  const retryStart = async () => {
    if (!created) return;
    setBusy(true);
    setStartError("");
    try {
      await api.agents.startAgent(created.name);
      onOpenChange(false);
    } catch (error) {
      setStartError(
        `Agent is still stopped: ${errorMessage(error)}. Check the host and retry start.`,
      );
    } finally {
      setBusy(false);
    }
  };

  return { busy, submitError, setSubmitError, startError, created, submit, retryStart };
}
