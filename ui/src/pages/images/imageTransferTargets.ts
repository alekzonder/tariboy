import type { Daemon } from "@/lib/daemons";

export interface ImageTransferTarget {
  id: string;
  label: string;
  target: Daemon | null;
}

const localTarget: ImageTransferTarget = {
  id: "",
  label: "This daemon (local)",
  target: null,
};

export function eligibleImageTransferTargets(source: Daemon | null, daemons: Daemon[]): ImageTransferTarget[] {
  return [
    ...(source === null ? [] : [localTarget]),
    ...daemons
      .filter((host) => host.id !== "" && host.state === "ready" && (source === null || host.id !== source.id))
      .map((host) => ({ id: host.id, label: host.label, target: host })),
  ];
}
