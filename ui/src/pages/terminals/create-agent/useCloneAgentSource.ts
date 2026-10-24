import { useCallback, useEffect, useState, type Dispatch, type SetStateAction } from "react";
import { createApi } from "@/lib/api";
import { resolveDaemon } from "@/lib/daemons";
import { cloneAgentDraft, type AgentCreateDraft } from "../agentCreateDraft";
import type { CloneAgentSource } from "../CreateAgentDialog";
import { errorMessage } from "./errorMessage";

export function useCloneAgentSource(
  cloneSource: CloneAgentSource | undefined,
  setDraft: Dispatch<SetStateAction<AgentCreateDraft>>,
) {
  const [sourceState, setSourceState] = useState<"idle" | "loading" | "ready" | "error">(
    cloneSource ? "loading" : "idle",
  );
  const [sourceError, setSourceError] = useState("");
  const fetchSource = useCallback(async (): Promise<AgentCreateDraft | null> => {
    if (!cloneSource) return null;
    const sourceTarget = cloneSource.hostId ? await resolveDaemon(cloneSource.hostId) : null;
    if (cloneSource.hostId && !sourceTarget) {
      throw new Error(`host ${cloneSource.hostId} was not found`);
    }
    // The source agent is read from its own host, whichever host the
    // dialog creates on.
    const source = await createApi(sourceTarget).agents.getAgent(cloneSource.agentName);
    return cloneAgentDraft(source);
  }, [cloneSource]);

  useEffect(() => {
    let alive = true;
    void fetchSource().then(
      (nextDraft) => {
        if (!alive || !nextDraft) return;
        setDraft(nextDraft);
        setSourceState("ready");
      },
      (error) => {
        if (!alive) return;
        setSourceState("error");
        setSourceError(`Could not load source agent: ${errorMessage(error)}`);
      },
    );
    return () => {
      alive = false;
    };
  }, [fetchSource, setDraft]);

  const retrySource = () => {
    setSourceState("loading");
    setSourceError("");
    void fetchSource().then(
      (nextDraft) => {
        if (!nextDraft) return;
        setDraft(nextDraft);
        setSourceState("ready");
      },
      (error) => {
        setSourceState("error");
        setSourceError(`Could not load source agent: ${errorMessage(error)}`);
      },
    );
  };

  return { sourceState, sourceError, retrySource };
}
