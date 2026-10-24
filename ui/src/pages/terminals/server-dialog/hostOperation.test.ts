import { describe, expect, it } from "vitest";
import type { DesktopHostView, HostOutputEvent } from "@/lib/desktop";
import {
  idleOperation,
  operationAfterHostState,
  operationAfterOutput,
  type HostOperation,
} from "./hostOperation";

const view = (patch: Partial<DesktopHostView>) =>
  ({ id: "h", state: "connecting", phase: "", message: "", ...patch }) as DesktopHostView;

const running = (patch: Partial<HostOperation> = {}): HostOperation => ({
  kind: "provision",
  status: "running",
  currentStep: "connect",
  error: null,
  ...patch,
});

const output = (patch: Partial<HostOutputEvent>) =>
  ({ host_id: "h", operation_id: "op", stream: "stdout", text: "", ...patch }) as HostOutputEvent;

describe("operationAfterHostState", () => {
  it("leaves an idle operation alone, even when the host fails", () => {
    const idle = idleOperation("provision");
    expect(operationAfterHostState(idle, view({ state: "failed" }))).toBe(idle);
  });

  it("fails at the host's phase step with a readable error", () => {
    expect(
      operationAfterHostState(
        running(),
        view({ state: "failed", phase: "install", message: '{"message":" boom "}' }),
      ),
    ).toEqual(running({ status: "failed", currentStep: "install", error: "boom" }));
  });

  it("fails a connect at the connect step when the host needs authentication", () => {
    expect(
      operationAfterHostState(
        running({ kind: "connect", currentStep: "reconnect" }),
        view({ state: "needs_auth", phase: "install", message: "auth" }),
      ),
    ).toEqual(
      running({ kind: "connect", status: "failed", currentStep: "connect", error: "auth" }),
    );
  });

  it("reports an unavailable existing daemon while a connect waits for health", () => {
    expect(
      operationAfterHostState(
        running({ kind: "connect" }),
        view({ message: "remote daemon health timed out: 30s" }),
      ),
    ).toEqual(
      running({
        kind: "connect",
        status: "failed",
        currentStep: "reconnect",
        error: "Existing Tariboy is unavailable at 127.0.0.1:9990.",
      }),
    );
  });

  it("succeeds once the host connects, also after a failed connect", () => {
    const done = { status: "succeeded", currentStep: "reconnect", error: null } as const;
    expect(operationAfterHostState(running(), view({ state: "ready" }))).toEqual(running(done));
    expect(
      operationAfterHostState(
        running({ kind: "connect", status: "failed", error: "x" }),
        view({ state: "ready" }),
      ),
    ).toEqual(running({ kind: "connect", ...done }));
  });

  it("follows the host's phase while running and otherwise keeps the operation", () => {
    expect(operationAfterHostState(running(), view({ phase: "upload" }))).toEqual(
      running({ currentStep: "upload" }),
    );
    const current = running();
    expect(operationAfterHostState(current, view({ phase: "unknown" }))).toBe(current);
  });
});

describe("operationAfterOutput", () => {
  it("moves to the step a phase line names and ignores unknown phases", () => {
    expect(operationAfterOutput(running(), output({ stream: "phase", text: "start" }))).toEqual(
      running({ currentStep: "service" }),
    );
    const current = running();
    expect(operationAfterOutput(current, output({ stream: "phase", text: "nope" }))).toBe(current);
    expect(operationAfterOutput(current, output({ text: "plain" }))).toBe(current);
  });

  it("fails on an error line, keeping the step it reached", () => {
    expect(
      operationAfterOutput(
        running({ currentStep: "upload" }),
        output({ stream: "error", text: "disk full" }),
      ),
    ).toEqual(running({ status: "failed", currentStep: "upload", error: "disk full" }));
    expect(
      operationAfterOutput(running({ currentStep: null }), output({ stream: "error", text: "x" }))
        .currentStep,
    ).toBe("connect");
  });
});
