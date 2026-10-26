// The API layer: one transport bound to one daemon, and domain modules built on
// it. Only createApi and server-selection boundaries accept an ApiTarget; every
// domain method addresses the host its client was created for.
import type { Daemon } from "../daemons";
import { bindTransport, type Transport } from "./client";
import { resolveTarget, type ApiTarget } from "./hosts";
import { agentsApi } from "./agents";
import { auditApi } from "./audit";
import { daemonApi } from "./daemon";
import { filesApi } from "./files";
import { groupsApi } from "./groups";
import { harnessesApi } from "./harnesses";
import { imageSourcesApi } from "./imageSources";
import { imagesApi } from "./images";
import { iterationsApi } from "./iterations";
import { pluginsApi } from "./plugins";
import { scriptsApi } from "./scripts";
import { storesApi } from "./stores";
import { tasksApi } from "./tasks";
import { usageApi } from "./usage";
import { workflowsApi } from "./workflows";

export * from "./client";
export * from "./hosts";
export * from "./agents";
export * from "./audit";
export * from "./daemon";
export * from "./files";
export * from "./groups";
export * from "./harnesses";
export * from "./imageSources";
export * from "./images";
export * from "./iterations";
export * from "./plugins";
export * from "./scripts";
export * from "./stores";
export * from "./tasks";
export * from "./usage";
export * from "./workflows";

export interface ApiClient extends Transport {
  /** False while a registered host has no endpoint (an SSH tunnel reconnecting). */
  readonly ready: boolean;
  readonly agents: ReturnType<typeof agentsApi>;
  readonly audit: ReturnType<typeof auditApi>;
  readonly daemon: ReturnType<typeof daemonApi>;
  readonly files: ReturnType<typeof filesApi>;
  readonly groups: ReturnType<typeof groupsApi>;
  readonly harnesses: ReturnType<typeof harnessesApi>;
  readonly imageSources: ReturnType<typeof imageSourcesApi>;
  readonly images: ReturnType<typeof imagesApi>;
  readonly iterations: ReturnType<typeof iterationsApi>;
  readonly plugins: ReturnType<typeof pluginsApi>;
  readonly scripts: ReturnType<typeof scriptsApi>;
  readonly stores: ReturnType<typeof storesApi>;
  readonly tasks: ReturnType<typeof tasksApi>;
  readonly usage: ReturnType<typeof usageApi>;
  readonly workflows: ReturnType<typeof workflowsApi>;
}

// createApi fixes the target once: undefined resolves to the active daemon now,
// null is the same-origin local daemon. A later host switch never redirects a
// client that already exists.
export function createApi(target: ApiTarget): ApiClient {
  const daemon: Daemon | null = resolveTarget(target);
  const t = bindTransport(daemon);
  return {
    ...t,
    ready: !daemon || !!daemon.baseURL,
    agents: agentsApi(t),
    audit: auditApi(t),
    daemon: daemonApi(t),
    files: filesApi(t),
    groups: groupsApi(t),
    harnesses: harnessesApi(t),
    imageSources: imageSourcesApi(t),
    images: imagesApi(t),
    iterations: iterationsApi(t),
    plugins: pluginsApi(t),
    scripts: scriptsApi(t),
    stores: storesApi(t),
    tasks: tasksApi(t),
    usage: usageApi(t),
    workflows: workflowsApi(t),
  };
}
