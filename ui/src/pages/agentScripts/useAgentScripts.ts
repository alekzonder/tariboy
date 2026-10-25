import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { useAgentName } from "@/lib/agent";
import type { ApiClient, AgentScript, AgentScriptRun } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { errorText } from "@/lib/errors";
import type { ScriptRequest } from "./scriptRequest";
import { scriptsNeedPolling } from "./scriptsModel";
import { useScriptRuns } from "./useScriptRuns";

export type LaunchRequest = Exclude<ScriptRequest, { error: string }>;

// useScriptPolling loads once on mount, then reloads the scripts and expanded
// runs every 3s while something can change, with one forced log refresh after.
function useScriptPolling({
  active,
  load,
  refreshExpandedRuns,
}: {
  active: boolean;
  load: () => Promise<void>;
  refreshExpandedRuns: (forceLogs?: boolean) => Promise<void>;
}) {
  const wasPolling = useRef(false);
  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);
  useEffect(() => {
    if (!active) {
      if (wasPolling.current) {
        wasPolling.current = false;
        void refreshExpandedRuns(true);
      }
      return;
    }
    wasPolling.current = true;
    const timer = window.setInterval(() => {
      void load();
      void refreshExpandedRuns();
    }, 3000);
    return () => window.clearInterval(timer);
  }, [active, load, refreshExpandedRuns]);
}

async function copyPath(path: string) {
  try {
    await navigator.clipboard.writeText(path);
    toast.success("Log path copied");
  } catch (error) {
    toast.error(`Could not copy path: ${errorText(error)}`);
  }
}

async function downloadLog(api: ApiClient, name: string, run: AgentScriptRun) {
  try {
    const blob = await api.scripts.downloadAgentScriptLog(name, run.id);
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `${run.id}.log`;
    anchor.click();
    URL.revokeObjectURL(url);
  } catch (error) {
    toast.error(`Could not download log: ${errorText(error)}`);
  }
}

// useAgentScripts owns the agent's script list, polling, run expansion and the
// actions the Scripts page offers.
export function useAgentScripts() {
  const api = useApi();
  const name = useAgentName();
  const [scripts, setScripts] = useState<AgentScript[]>([]);
  const [remove, setRemove] = useState<AgentScript | null>(null);

  const load = useCallback(async () => {
    try {
      setScripts((await api.scripts.listAgentScripts(name)).scripts ?? []);
    } catch (error) {
      toast.error(`Could not load scripts: ${errorText(error)}`);
    }
  }, [api, name]);

  const scriptRuns = useScriptRuns(api, name);
  useScriptPolling({
    active: scriptsNeedPolling(scripts),
    load,
    refreshExpandedRuns: scriptRuns.refreshExpandedRuns,
  });

  const launch = async (request: LaunchRequest, clearForm: () => void) => {
    try {
      if (request.mode === "once") await api.scripts.runAgentScriptOnce(name, request.spec);
      else await api.scripts.scheduleAgentScript(name, request.spec);
      clearForm();
      toast.success(request.mode === "once" ? "One-shot run queued" : "Scheduled script started");
      await load();
    } catch (error) {
      toast.error(`Could not start script: ${errorText(error)}`);
    }
  };

  const cancel = async (id: string) => {
    try {
      await api.scripts.cancelAgentScript(name, id);
      toast.success("Cancellation requested");
      await load();
    } catch (error) {
      toast.error(`Could not cancel script: ${errorText(error)}`);
    }
  };

  const rerun = async (definition: AgentScript) => {
    try {
      await api.scripts.rerunAgentScript(name, definition.id);
      toast.success("Run queued");
      scriptRuns.forgetRuns(definition.id);
      await load();
    } catch (error) {
      toast.error(`Could not execute script: ${errorText(error)}`);
    }
  };

  const confirmRemove = async () => {
    if (!remove) return;
    try {
      await api.scripts.removeAgentScript(name, remove.id);
      toast.success("Script removed");
      setRemove(null);
      await load();
    } catch (error) {
      toast.error(`Could not remove script: ${errorText(error)}`);
    }
  };

  return {
    ...scriptRuns,
    scripts,
    remove,
    setRemove,
    launch,
    cancel,
    rerun,
    copyPath,
    download: (run: AgentScriptRun) => downloadLog(api, name, run),
    confirmRemove,
  };
}

export type AgentScriptsState = ReturnType<typeof useAgentScripts>;
