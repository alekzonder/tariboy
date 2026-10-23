import type { DaemonMeta } from "@/lib/daemons";
import type { DesktopHostView, HostOutputEvent } from "@/lib/desktop";
import type {
  HostOperationKind,
  HostOperationStatus,
  HostProgressStep,
} from "@/lib/hostProgress";

export interface HostOperation {
  kind: HostOperationKind;
  status: HostOperationStatus;
  currentStep: HostProgressStep | null;
  error: string | null;
}

export function idleOperation(kind: HostOperationKind): HostOperation {
  return {
    kind,
    status: "idle",
    currentStep: null,
    error: null,
  };
}

export function hostMeta(view: DesktopHostView): DaemonMeta {
  return {
    id: view.id,
    label: view.label,
    baseURL: view.base_url,
    kind: view.kind,
    state: view.state,
    sshAlias: view.ssh_alias,
    phase: view.phase,
    platform: view.platform,
    arch: view.arch,
    prerequisites: view.prerequisites,
    message: view.message,
    lastDaemonVersion: view.last_daemon_version,
  };
}

export function preflightView(event: HostOutputEvent): Partial<DesktopHostView> | null {
  if (event.stream !== "stdout" || !event.text.trim().startsWith("{")) return null;
  try {
    const value = JSON.parse(event.text) as {
      platform?: string;
      arch?: string;
      prerequisites?: string[];
    };
    if (!value.platform && !value.arch) return null;
    return {
      platform: value.platform ?? "",
      arch: value.arch ?? "",
      prerequisites: value.prerequisites ?? [],
    };
  } catch {
    return null;
  }
}
