import type { IterationDetail, IterationLogs, IterationSummary } from "../types";
import type { Transport } from "./client";

export interface TimeoutExtension {
  id: string;
  timeout_deadline: string;
  hard_timeout_deadline: string;
  timeout_extensions: number;
  shim_sync: "success" | "pending";
}

export function iterationsApi(t: Transport) {
  return {
    listIterations: (name: string) =>
      t.agentGet<{ iterations: IterationSummary[]; count: number }>(name, "iterations"),
    getIteration: (name: string, id: string) =>
      t.agentGet<IterationDetail>(name, `iterations/${encodeURIComponent(id)}`),
    getIterationLogs: (name: string, id: string) =>
      t.agentGet<IterationLogs>(name, `iterations/${encodeURIComponent(id)}/logs`),
    extendIterationTimeout: (name: string, id: string) =>
      t.agentPost<TimeoutExtension>(name, `iterations/${encodeURIComponent(id)}/extend-timeout`),
    // Replace one iteration's whole tag set. The daemon also exposes add/remove
    // and accepts a batch of ids; the UI only ever edits one iteration at a
    // time, and a replace expresses both an added and a removed tag.
    setIterationTags: (name: string, iterationId: string, tags: string[]) =>
      t.agentPost<{ tags: Record<string, string[]>; count: number }>(name, "iterations/tags/set", {
        id: [iterationId],
        tag: tags,
      }),
  };
}
