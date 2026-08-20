import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { IterationAuditLog } from "./IterationAuditLog";
import * as api from "@/lib/api";
import * as transcript from "@/lib/transcript";

let urls: string[] = [];
const ITER = "dev-worker-20260709065606-3";
const EVENTS = [
  { seq: 1, kind: "iteration_started", source: "system", at: "t1", data: JSON.stringify({ trigger: "manual" }), iteration_id: ITER },
  { seq: 2, kind: "harness_output", source: "harness", at: "t2", data: JSON.stringify({ line: "hi" }), iteration_id: ITER },
];

beforeEach(() => {
  urls = [];
  vi.stubGlobal("EventSource", class { addEventListener() {} removeEventListener() {} close() {} } as unknown as typeof EventSource);
  vi.stubGlobal("fetch", vi.fn().mockImplementation((url: string) => {
    urls.push(url);
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result: { events: EVENTS } }) } as Response);
  }));
});
afterEach(() => vi.restoreAllMocks());

describe("IterationAuditLog", () => {
  it("fetches the iteration's events and shows a chip with id + status", async () => {
    const subSpy = vi.spyOn(api, "subscribeAgentEvents");
    render(<IterationAuditLog name="dev-worker" iterationId={ITER} iterationStatus="running" />);
    await waitFor(() => expect(screen.getByText("manual")).toBeInTheDocument());
    expect(urls.some((u) => u.includes(`iteration=${ITER}`))).toBe(true);
    // Chip shows the condensed id (HHMMSS-counter) and the status.
    expect(screen.getByText(/065606-3 · running/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Copy audit log" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Export audit log" })).toBeInTheDocument();
    // The live-update subscription must include "proxy" so proxy-call-only
    // updates (no new audit events) still trigger a refresh.
    expect(subSpy).toHaveBeenCalled();
    expect(subSpy.mock.calls[0][1]).toContain("proxy");
  });

  it("with no iteration id shows a placeholder and does not fetch logs", () => {
    render(<IterationAuditLog name="dev-worker" iterationId="" iterationStatus="" />);
    expect(screen.getByText(/no iterations yet/i)).toBeInTheDocument();
    expect(urls).toEqual([]);
  });
});

describe("IterationAuditLog transcript enrichment", () => {
  afterEach(() => vi.restoreAllMocks());

  it("renders a proxycall row from the fetched transcript", async () => {
    vi.spyOn(api, "agentGet").mockResolvedValue({
      events: [{ seq: 1, kind: "harness_output", source: "harness", data: "{}", at: "2026-07-10T00:00:03Z", iteration_id: "it" }],
    } as unknown as never);
    vi.spyOn(api, "subscribeAgentEvents").mockReturnValue(() => {});
    vi.spyOn(transcript, "fetchTranscript").mockResolvedValue([
      { seq: 0, ts: "2026-07-10T00:00:02Z", provider: "anthropic", model: "claude-opus-4", instructions: "S", instructions_changed: true, delta: [], response: { blocks: [{ type: "thinking", text: "hmm" }] } },
    ]);
    render(<IterationAuditLog name="a1" iterationId="it" iterationStatus="running" />);
    await waitFor(() => expect(screen.getByText(/claude-opus-4/)).toBeInTheDocument());
  });

  it("renders proxy-call content in full mode (no harness_output events)", async () => {
    vi.spyOn(api, "agentGet").mockResolvedValue({ events: [] } as unknown as never);
    vi.spyOn(api, "subscribeAgentEvents").mockReturnValue(() => {});
    vi.spyOn(transcript, "fetchTranscript").mockResolvedValue([
      {
        seq: 0,
        ts: "2026-07-10T00:00:02Z",
        provider: "anthropic",
        model: "claude-opus-4",
        instructions: "S",
        instructions_changed: false,
        delta: [],
        response: { blocks: [{ type: "text", text: "hello from assistant" }] },
      },
    ]);
    render(<IterationAuditLog name="a1" iterationId="it" iterationStatus="running" />);
    // No harness_output events → hasHarnessOutput is false → mode is "full",
    // so the proxy call's assistant text block must render directly from the
    // transcript fetch alone (the calls-only render path).
    await waitFor(() => expect(screen.getByText(/hello from assistant/)).toBeInTheDocument());
  });
});
