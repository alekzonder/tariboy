import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { LoopToggle } from "./LoopControls";

let urls: string[] = [];
beforeEach(() => {
  urls = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation((url: string) => {
    urls.push(url);
    return Promise.resolve({
      ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result: { loop_enabled: true } }),
    } as Response);
  }));
});
afterEach(() => vi.restoreAllMocks());

describe("LoopToggle", () => {
  it("enables the loop when disabled", async () => {
    const onChanged = vi.fn();
    render(<LoopToggle name="foo" enabled={false} onChanged={onChanged} />);
    await userEvent.click(screen.getByText("Start"));
    await waitFor(() => expect(urls.some((u) => u.includes("/loop/enable"))).toBe(true));
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
  });

  it("shows a confirm-guarded Pause when enabled", () => {
    render(<LoopToggle name="foo" enabled={true} onChanged={vi.fn()} />);
    expect(screen.getByText("Pause")).toBeInTheDocument();
  });
});
