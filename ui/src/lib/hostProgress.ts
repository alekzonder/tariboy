export type HostOperationKind = "provision" | "update";

export type HostOperationStatus =
  | "idle"
  | "running"
  | "succeeded"
  | "failed";

export type HostProgressStep =
  | "connect"
  | "check"
  | "upload"
  | "install"
  | "service"
  | "reconnect";

export type HostProgressStepState =
  | "pending"
  | "active"
  | "complete"
  | "failed";

export const HOST_PROGRESS_STEPS: ReadonlyArray<{
  id: HostProgressStep;
  provisionLabel: string;
  updateLabel: string;
}> = [
  { id: "connect", provisionLabel: "Connect to host", updateLabel: "Connect to host" },
  { id: "check", provisionLabel: "Check server", updateLabel: "Check server" },
  { id: "upload", provisionLabel: "Upload release", updateLabel: "Upload release" },
  { id: "install", provisionLabel: "Install release", updateLabel: "Install release" },
  { id: "service", provisionLabel: "Start Tariboy", updateLabel: "Restart Tariboy" },
  { id: "reconnect", provisionLabel: "Connect to Tariboy", updateLabel: "Reconnect" },
];

const PHASE_STEPS: Record<string, HostProgressStep> = {
  resolve: "connect",
  authenticate: "connect",
  auth: "connect",
  preflight: "check",
  stage: "upload",
  upload: "upload",
  stage_release: "install",
  verify: "install",
  verify_install: "install",
  install: "install",
  activate: "install",
  rollback: "install",
  start: "service",
  restart: "service",
  daemon: "service",
  status: "service",
  tunnel: "reconnect",
  connect: "reconnect",
  health: "reconnect",
};

export function hostStepForPhase(phase: string): HostProgressStep | null {
  return PHASE_STEPS[phase] ?? null;
}

export function hostStepStates(
  current: HostProgressStep | null,
  status: HostOperationStatus,
): Record<HostProgressStep, HostProgressStepState> {
  const currentIndex = current
    ? HOST_PROGRESS_STEPS.findIndex((step) => step.id === current)
    : -1;
  return Object.fromEntries(HOST_PROGRESS_STEPS.map((step, index) => {
    let state: HostProgressStepState = "pending";
    if (status === "succeeded" || (currentIndex >= 0 && index < currentIndex)) {
      state = "complete";
    } else if (index === currentIndex && status === "running") {
      state = "active";
    } else if (index === currentIndex && status === "failed") {
      state = "failed";
    }
    return [step.id, state];
  })) as Record<HostProgressStep, HostProgressStepState>;
}

export function isTerminalHostOutput(stream: string): boolean {
  return stream === "error";
}

export function formatHostOperationError(raw: string): string {
  let message = raw.trim();
  try {
    const value = JSON.parse(message) as { message?: unknown };
    if (typeof value.message === "string") message = value.message.trim();
  } catch {
    // Plain process and runtime errors are already valid input.
  }

  const nonSymlink = message.match(/refusing to replace non-symlink ([^\r\n]+)/);
  if (nonSymlink) {
    return `${nonSymlink[1].trim()} is an existing file and cannot be replaced automatically. Move it out of the way and try again.`;
  }

  const cleaned = message
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => !/^tariboy(?:d|-shim|-tools)?: OK$/.test(line))
    .join("\n")
    .replace(/^Error:\s*/i, "")
    .replace(/^[a-z][a-z0-9_]*:\s+/i, "")
    .trim();

  return cleaned || "The host operation failed. Open Technical details for more information.";
}
