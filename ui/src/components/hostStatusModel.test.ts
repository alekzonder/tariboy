import { describe, expect, it } from "vitest";
import type { DaemonMeta } from "@/lib/daemons";
import { hostStatusView } from "./hostStatusModel";

function host(overrides: Partial<DaemonMeta> = {}): DaemonMeta {
  return {
    id: "ssh-1",
    label: "gpu",
    baseURL: "",
    kind: "ssh",
    state: "disconnected",
    prerequisites: [],
    ...overrides,
  };
}

describe("hostStatusView badge", () => {
  it.each([
    ["disconnected", "secondary"],
    ["connecting", "secondary"],
    ["provisioning", "secondary"],
    ["ready", "default"],
    ["error", "destructive"],
    ["degraded", "secondary"],
    ["needs_auth", "secondary"],
    ["failed", "secondary"],
  ] as const)("shows %s as a %s badge", (state, variant) => {
    expect(hostStatusView(host({ state }), "", true).badgeVariant).toBe(variant);
  });

  it("treats a host without a state as disconnected", () => {
    const view = hostStatusView(host({ state: undefined }), "", true);
    expect(view.state).toBe("disconnected");
    expect(view.badgeVariant).toBe("secondary");
    expect(view.actions.map((action) => action.label)).toEqual(["Connect"]);
  });
});

describe("hostStatusView buttons", () => {
  it.each([
    ["disconnected", ["Connect"]],
    ["connecting", []],
    ["provisioning", []],
    ["ready", []],
    ["error", []],
    ["degraded", ["Connect"]],
    ["needs_auth", ["Authenticate"]],
    ["failed", ["Connect"]],
  ] as const)("offers %s hosts %j", (state, labels) => {
    const view = hostStatusView(host({ state }), "0.11.5", true);
    expect(view.actions.map((action) => action.label)).toEqual(labels);
  });

  it.each([
    ["ready", true, ["Update"]],
    ["error", true, ["Update"]],
    ["ready", false, []],
    ["disconnected", true, ["Connect"]],
  ] as const)(
    "offers a %s host with a newer app (showUpdate %s) %j",
    (state, showUpdate, labels) => {
      const view = hostStatusView(
        host({ state, lastDaemonVersion: "0.11.4" }),
        "0.11.5",
        showUpdate,
      );
      expect(view.actions.map((action) => action.label)).toEqual(labels);
    },
  );

  it("routes Update and Authenticate to the update handler and Connect to connect", () => {
    expect(
      hostStatusView(host({ state: "needs_auth" }), "", true).actions.map((a) => a.handler),
    ).toEqual(["onUpdate"]);
    expect(
      hostStatusView(host({ state: "ready", lastDaemonVersion: "0.1.0" }), "0.2.0", true).actions,
    ).toEqual([{ label: "Update", handler: "onUpdate" }]);
    expect(hostStatusView(host(), "", true).actions).toEqual([
      { label: "Connect", handler: "onConnect" },
    ]);
  });

  it("hides Connect for an unsupported host or a host key mismatch", () => {
    expect(hostStatusView(host({ platform: "plan9", arch: "amd64" }), "", true).actions).toEqual(
      [],
    );
    expect(hostStatusView(host({ message: "ssh: host_key_mismatch" }), "", true).actions).toEqual(
      [],
    );
  });

  it("reports an unsupported host so the notes can say so", () => {
    expect(hostStatusView(host({ platform: "plan9", arch: "amd64" }), "", true).unsupported).toBe(
      true,
    );
    expect(hostStatusView(host({ platform: "Linux", arch: "x86_64" }), "", true).unsupported).toBe(
      false,
    );
  });
});
