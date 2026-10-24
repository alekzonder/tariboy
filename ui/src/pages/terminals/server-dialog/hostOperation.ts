import type { DaemonMeta } from "@/lib/daemons";
import { hostConnected, type DesktopHostView, type HostOutputEvent } from "@/lib/desktop";
import {
  formatHostOperationError,
  hostStepForPhase,
  isTerminalHostOutput,
  type HostOperationKind,
  type HostOperationStatus,
  type HostProgressStep,
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

/** A connect to an existing daemon that never answers its health check. */
function existingUnavailable(current: HostOperation, next: DesktopHostView): boolean {
  return (
    current.kind === "connect" &&
    next.state === "connecting" &&
    next.message.startsWith("remote daemon health timed out:")
  );
}

function hostStateFailed(current: HostOperation, next: DesktopHostView): boolean {
  if (current.status === "idle") return false;
  const needsAuth = current.kind === "connect" && next.state === "needs_auth";
  return next.state === "failed" || needsAuth || existingUnavailable(current, next);
}

/** The operation after the host reports a new state; the same object when the
 *  state changes nothing. */
export function operationAfterHostState(
  current: HostOperation,
  next: DesktopHostView,
): HostOperation {
  const phaseStep = hostStepForPhase(next.phase);
  if (hostStateFailed(current, next)) {
    const unavailable = existingUnavailable(current, next);
    return {
      ...current,
      status: "failed",
      currentStep: unavailable
        ? "reconnect"
        : current.kind === "connect"
          ? "connect"
          : (phaseStep ?? current.currentStep ?? "connect"),
      error: unavailable
        ? "Existing Tariboy is unavailable at 127.0.0.1:9990."
        : formatHostOperationError(next.message),
    };
  }
  const retrying = current.kind === "connect" && current.status === "failed";
  if (hostConnected(next.state) && (current.status === "running" || retrying)) {
    return { ...current, status: "succeeded", currentStep: "reconnect", error: null };
  }
  if (phaseStep && current.status === "running") return { ...current, currentStep: phaseStep };
  return current;
}

/** The operation after one line of its output: a phase line moves the step and
 *  an error line fails it. The same object when the line changes nothing. */
export function operationAfterOutput(
  current: HostOperation,
  event: HostOutputEvent,
): HostOperation {
  let next = current;
  const step = event.stream === "phase" ? hostStepForPhase(event.text) : null;
  if (step) next = { ...next, currentStep: step };
  if (isTerminalHostOutput(event.stream)) {
    next = {
      ...next,
      status: "failed",
      currentStep: next.currentStep ?? "connect",
      error: formatHostOperationError(event.text),
    };
  }
  return next;
}
