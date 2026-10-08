import type { Daemon } from "@/lib/daemons";
import { hasLocalDaemon, hostConnected } from "@/lib/desktop";

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
    ...(source === null || !hasLocalDaemon() ? [] : [localTarget]),
    ...daemons
      .filter((host) => host.id !== "" && hostConnected(host.state) && (source === null || host.id !== source.id))
      .map((host) => ({ id: host.id, label: host.label, target: host })),
  ];
}
