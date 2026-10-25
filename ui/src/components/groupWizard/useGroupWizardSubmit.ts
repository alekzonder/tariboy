import { useState, type Dispatch } from "react";
import { ApiError, type ApiClient, type ImageSummary } from "@/lib/api";
import { commaFieldError } from "@/components/agentFormValues";
import {
  allowsPluginOverrides,
  leaderNameOf,
  specFor,
  type AgentDraft,
  type WizardAction,
  type WizardState,
} from "./groupWizardModel";

interface SubmitInput {
  api: ApiClient;
  state: WizardState;
  dispatch: Dispatch<WizardAction>;
  name: string;
  baseCwd: string;
  images: ImageSummary[];
  onCreated?: () => void;
}

const errorReason = (e: unknown) => (e instanceof ApiError ? e.message : String(e));

// The N+1 orchestration behind "Create group": one POST /api/groups, then one
// POST /api/agents per row. Partial failure is not rolled back; only failed
// rows are retried, and retry never re-creates the group.
export function useGroupWizardSubmit(input: SubmitInput) {
  const { api, state, dispatch, name, baseCwd, images, onCreated } = input;
  const { agents, rowStatus } = state;
  const [busy, setBusy] = useState(false);
  const [groupCreated, setGroupCreated] = useState(false);
  const [groupErr, setGroupErr] = useState<string | null>(null);
  const [formErr, setFormErr] = useState<string | null>(null);

  const leaderName = leaderNameOf(agents, state.leaderIdx);
  const allowsPlugins = (a: AgentDraft) => allowsPluginOverrides(images, a.image);

  const createGroup = async () => {
    try {
      await api.request("POST", "/api/groups", {
        name: name.trim(),
        lead: leaderName || undefined,
      });
      setGroupCreated(true);
      setGroupErr(null);
      return true;
    } catch (e) {
      setGroupErr(errorReason(e));
      setBusy(false);
      return false;
    }
  };

  // Run the N+1 orchestration over `drafts`. `withGroup` is false on retry
  // (the group already exists). Agents are created sequentially so the call
  // order is deterministic and the per-row status renders as each settles.
  const orchestrate = async (drafts: AgentDraft[], withGroup: boolean) => {
    setBusy(true);
    if (withGroup && !(await createGroup())) return;
    for (const a of drafts) {
      dispatch({ type: "setRowStatus", id: a.id, status: { kind: "creating" } });
      try {
        const r = await api.agents.createAgent(specFor(a, baseCwd, name, allowsPlugins(a)));
        dispatch({ type: "setRowStatus", id: a.id, status: { kind: "created", name: r.name } });
      } catch (e) {
        dispatch({
          type: "setRowStatus",
          id: a.id,
          status: { kind: "failed", reason: errorReason(e) },
        });
      }
    }
    setBusy(false);
    onCreated?.();
  };

  const create = () => {
    setFormErr(null);
    for (const a of agents) {
      const commaErr = commaFieldError(a.envRows, allowsPlugins(a) ? a.plugins : []);
      if (commaErr) {
        setFormErr(`${a.name.trim() || "agent"}: ${commaErr}`);
        return;
      }
    }
    void orchestrate(agents, true);
  };
  const retryFailed = () => {
    const failed = agents.filter((a) => rowStatus[a.id]?.kind === "failed");
    if (failed.length) void orchestrate(failed, false);
  };

  const anyFailed = agents.some((a) => rowStatus[a.id]?.kind === "failed");

  return { busy, groupCreated, groupErr, formErr, create, retryFailed, anyFailed, leaderName };
}
