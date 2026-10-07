import { describe, it, expect, vi, afterEach } from "vitest";
import { eligibleImageTransferTargets } from "./imageTransferTargets";
import type { Daemon } from "@/lib/daemons";

const remote: Daemon = { id: "r", label: "remote", baseURL: "https://r", token: "", state: "ready" };
const other: Daemon = { id: "o", label: "other", baseURL: "https://o", token: "", state: "ready" };

afterEach(() => vi.unstubAllEnvs());

describe("eligibleImageTransferTargets", () => {
  it("offers the local daemon when the source is remote", () => {
    expect(eligibleImageTransferTargets(remote, [remote, other]).map((t) => t.id)).toEqual(["", "o"]);
  });

  it("never offers a local daemon in the Android app", () => {
    vi.stubEnv("VITE_TARIBOY_SHELL", "android");
    expect(eligibleImageTransferTargets(remote, [remote, other]).map((t) => t.id)).toEqual(["o"]);
  });
});
