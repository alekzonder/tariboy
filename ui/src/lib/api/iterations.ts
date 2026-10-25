import type {
  IterationList,
  IterationLogs,
  IterationTagResult,
  IterationTimeoutResult,
  IterationView,
} from "../api-types.generated";
import type { Transport } from "./client";

export type {
  IterationLogs,
  IterationRow,
  IterationTimeoutResult,
  IterationView,
} from "../api-types.generated";

export function iterationsApi(t: Transport) {
  return {
    listIterations: (name: string) => t.agentGet<IterationList>(name, "iterations"),
    getIteration: (name: string, id: string) =>
      t.agentGet<IterationView>(name, `iterations/${encodeURIComponent(id)}`),
    getIterationLogs: (name: string, id: string) =>
      t.agentGet<IterationLogs>(name, `iterations/${encodeURIComponent(id)}/logs`),
    extendIterationTimeout: (name: string, id: string) =>
      t.agentPost<IterationTimeoutResult>(
        name,
        `iterations/${encodeURIComponent(id)}/extend-timeout`,
      ),
    // Replace one iteration's whole tag set. The daemon also exposes add/remove
    // and accepts a batch of ids; the UI only ever edits one iteration at a
    // time, and a replace expresses both an added and a removed tag.
    setIterationTags: (name: string, iterationId: string, tags: string[]) =>
      t.agentPost<IterationTagResult>(name, "iterations/tags/set", {
        id: [iterationId],
        tag: tags,
      }),
  };
}
