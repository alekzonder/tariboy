import { hostInstallBlocked, hostUpdateAvailable, type DaemonMeta } from "@/lib/daemons";

type HostState = NonNullable<DaemonMeta["state"]>;

type BadgeVariant = "default" | "destructive" | "secondary";

type HostStatusAction = {
  label: "Update" | "Authenticate" | "Connect";
  handler: "onUpdate" | "onConnect";
};

type ActionContext = {
  state: HostState;
  updateAvailable: boolean;
  canConnect: boolean;
};

const BADGE_VARIANTS: Partial<Record<HostState, BadgeVariant>> = {
  ready: "default",
  error: "destructive",
};

const CONNECTABLE_STATES: ReadonlySet<HostState> = new Set(["disconnected", "degraded", "failed"]);

// Order is the button order in the header.
const HOST_ACTIONS: ReadonlyArray<
  HostStatusAction & { show: (context: ActionContext) => boolean }
> = [
  { label: "Update", handler: "onUpdate", show: (context) => context.updateAvailable },
  { label: "Authenticate", handler: "onUpdate", show: (context) => context.state === "needs_auth" },
  { label: "Connect", handler: "onConnect", show: (context) => context.canConnect },
];

export function hostStatusView(host: DaemonMeta, appVersion: string, showUpdate: boolean) {
  const state = host.state ?? "disconnected";
  const unsupported = hostInstallBlocked(host.platform, host.arch, host.prerequisites);
  const keyMismatch = !!host.message?.includes("host_key_mismatch");
  const context: ActionContext = {
    state,
    updateAvailable: showUpdate && hostUpdateAvailable(host, appVersion),
    canConnect: CONNECTABLE_STATES.has(state) && !unsupported && !keyMismatch,
  };
  return {
    state,
    unsupported,
    badgeVariant: BADGE_VARIANTS[state] ?? "secondary",
    actions: HOST_ACTIONS.filter((action) => action.show(context)).map(
      ({ label, handler }): HostStatusAction => ({ label, handler }),
    ),
  };
}
