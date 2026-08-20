import { describe, expect, it, vi, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IterationTimeoutControl } from "./IterationTimeoutControl";
import type { AgentStatus } from "@/lib/types";

const toast = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }));
vi.mock("sonner", () => ({ toast }));

const now = Date.parse("2026-07-14T10:00:00Z");
function status(overrides: Partial<AgentStatus["active_iteration"]> = {}, serverNow = "2026-07-14T10:00:00Z"): AgentStatus {
  return {
    name: "foo", state: "running", loop_enabled: true, iterations: 1,
    last_iteration: null, last_iteration_id: null, status_message: "", status_updated: "", server_now: serverNow,
    active_iteration: { id: "i1", started_at: serverNow, timeout_period_s: 300,
      timeout_deadline: "2026-07-14T10:05:00Z", effective_deadline: "2026-07-14T10:05:00Z",
      timeout_extensions: 0, ...overrides },
  };
}
function response(result: unknown, ok = true, statusCode = 200) {
  return Promise.resolve({ ok, status: statusCode, text: async () => JSON.stringify(ok ? { ok, result } : { ok, error: { code: "conflict", message: "changed" } }) } as Response);
}

afterEach(() => { vi.restoreAllMocks(); toast.success.mockReset(); toast.warning.mockReset(); toast.error.mockReset(); });

describe("IterationTimeoutControl", () => {
  it("accepts the server's success shim-sync response and renders the extension", async () => {
    vi.spyOn(Date, "now").mockReturnValue(now);
    vi.stubGlobal("fetch", vi.fn(() => response({ id: "i1", timeout_deadline: "2026-07-14T10:10:00Z", hard_timeout_deadline: "", timeout_extensions: 1, shim_sync: "success" })));
    render(<IterationTimeoutControl name="foo" status={status()} refresh={vi.fn()} />);
    expect(screen.getByText(/in 5m/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "+5m" }));
    await waitFor(() => expect(screen.getByText(/in 10m/)).toBeInTheDocument());
    expect(toast.success).toHaveBeenCalledWith("timeout extended by 5m");
  });

  it("covers under-minute, expired, zero-timeout, inactive, and server-skew snapshots", () => {
    vi.spyOn(Date, "now").mockReturnValue(now);
    const { rerender } = render(<IterationTimeoutControl name="foo" status={status({ effective_deadline: "2026-07-14T10:00:30Z" })} refresh={vi.fn()} />);
    expect(screen.getByText(/in <1m/)).toBeInTheDocument();
    rerender(<IterationTimeoutControl name="foo" status={status({ effective_deadline: "2026-07-14T09:59:59Z" })} refresh={vi.fn()} />);
    expect(screen.getByText("timeout firing…")).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    rerender(<IterationTimeoutControl name="foo" status={status({ timeout_period_s: 0, timeout_deadline: undefined, effective_deadline: undefined })} refresh={vi.fn()} />);
    expect(screen.getByText("No timeout")).toBeInTheDocument();
    rerender(<IterationTimeoutControl name="foo" status={null} refresh={vi.fn()} />);
    expect(screen.queryByLabelText("Iteration timeout")).not.toBeInTheDocument();
    // Browser time says 10:00 but the daemon says 10:10, so 10:05 is expired.
    rerender(<IterationTimeoutControl name="foo" status={status({}, "2026-07-14T10:10:00Z")} refresh={vi.fn()} />);
    expect(screen.getByText("timeout firing…")).toBeInTheDocument();
  });

  it("disables while extending, refreshes on 409, and warns when shim sync is pending", async () => {
    vi.spyOn(Date, "now").mockReturnValue(now);
    let resolve!: (value: Response) => void;
    vi.stubGlobal("fetch", vi.fn(() => new Promise<Response>((r) => { resolve = r; })));
    const refresh = vi.fn();
    const { rerender } = render(<IterationTimeoutControl name="foo" status={status()} refresh={refresh} />);
    await userEvent.click(screen.getByRole("button", { name: "+5m" }));
    expect(screen.getByRole("button", { name: "Extending…" })).toBeDisabled();
    resolve(await response({}, false, 409));
    await waitFor(() => expect(refresh).toHaveBeenCalled());
    expect(toast.warning).toHaveBeenCalledWith("timeout changed; refreshed current status");
    vi.stubGlobal("fetch", vi.fn(() => response({ id: "i1", timeout_deadline: "2026-07-14T10:10:00Z", hard_timeout_deadline: "", timeout_extensions: 1, shim_sync: "pending" })));
    rerender(<IterationTimeoutControl name="foo" status={status()} refresh={refresh} />);
    await userEvent.click(screen.getByRole("button", { name: "+5m" }));
    await waitFor(() => expect(toast.warning).toHaveBeenCalledWith("timeout saved; shim sync is pending"));
  });
});
