import { describe, expect, it } from "vitest";
import {
  HOST_PROGRESS_STEPS,
  formatHostOperationError,
  hostStepForPhase,
  hostStepStates,
  isTerminalHostOutput,
} from "./hostProgress";

describe("host progress model", () => {
  it("maps technical phases into one six-step user flow", () => {
    expect(HOST_PROGRESS_STEPS.map((step) => step.id)).toEqual([
      "connect",
      "check",
      "upload",
      "install",
      "service",
      "reconnect",
    ]);
    expect([
      hostStepForPhase("resolve"),
      hostStepForPhase("preflight"),
      hostStepForPhase("stage"),
      hostStepForPhase("stage_release"),
      hostStepForPhase("restart"),
      hostStepForPhase("connect"),
    ]).toEqual([
      "connect",
      "check",
      "upload",
      "install",
      "service",
      "reconnect",
    ]);
    expect(hostStepForPhase("rollback")).toBe("install");
    expect(hostStepForPhase("status")).toBe("service");
    expect(hostStepForPhase("unknown")).toBeNull();
  });

  it("derives accessible step states from the current terminal result", () => {
    expect(hostStepStates("install", "running")).toEqual({
      connect: "complete",
      check: "complete",
      upload: "complete",
      install: "active",
      service: "pending",
      reconnect: "pending",
    });
    expect(hostStepStates("install", "failed").install).toBe("failed");
    expect(hostStepStates("reconnect", "succeeded")).toEqual({
      connect: "complete",
      check: "complete",
      upload: "complete",
      install: "complete",
      service: "complete",
      reconnect: "complete",
    });
  });

  it("does not confuse process stderr with a terminal operation error", () => {
    expect(isTerminalHostOutput("stderr")).toBe(false);
    expect(isTerminalHostOutput("stdout")).toBe(false);
    expect(isTerminalHostOutput("error")).toBe(true);
  });

  it("turns a non-symlink installer refusal into one safe action", () => {
    expect(formatHostOperationError(
      JSON.stringify({
        code: "ssh_failed",
        message: [
          "tariboyd: OK",
          "tariboy: OK",
          "tariboy-shim: OK",
          "tariboy-tools: OK",
          "refusing to replace non-symlink /home/me/.local/bin/tariboyd",
        ].join("\n"),
      }),
    )).toBe(
      "/home/me/.local/bin/tariboyd is an existing file and cannot be replaced automatically. Move it out of the way and try again.",
    );
  });

  it("extracts generic JSON messages without checksum noise or error prefixes", () => {
    expect(formatHostOperationError(JSON.stringify({
      code: "ssh_failed",
      message: "tariboyd: OK\npermission denied while activating release",
    }))).toBe("permission denied while activating release");
    expect(formatHostOperationError(
      "Error: unsupported_host: remote installation requires Linux x86_64",
    )).toBe("remote installation requires Linux x86_64");
  });
});
