import { describe, expect, it } from "vitest";
import type { Daemon } from "@/lib/daemons";
import { routeHostState } from "./routeHostState";

const daemon = (patch: Partial<Daemon>): Daemon => ({
  id: "prod",
  label: "prod",
  baseURL: "http://127.0.0.1:1",
  token: "",
  ...patch,
});

describe("routeHostState", () => {
  it("treats the implicit local host as known and ready", () => {
    expect(routeHostState("", [])).toEqual({ knownHost: true, hostReady: true });
  });

  it("does not know a host missing from the registry", () => {
    expect(routeHostState("prod", [])).toEqual({ knownHost: false, hostReady: false });
  });

  it("needs a base URL before an HTTPS host is ready", () => {
    expect(routeHostState("prod", [daemon({ baseURL: "" })]).hostReady).toBe(false);
    expect(routeHostState("prod", [daemon({})]).hostReady).toBe(true);
  });

  it("needs a connected tunnel before an SSH host is ready", () => {
    expect(routeHostState("prod", [daemon({ kind: "ssh", state: "connecting" })]).hostReady).toBe(
      false,
    );
    expect(routeHostState("prod", [daemon({ kind: "ssh", state: "ready" })])).toEqual({
      knownHost: true,
      hostReady: true,
    });
  });
});
