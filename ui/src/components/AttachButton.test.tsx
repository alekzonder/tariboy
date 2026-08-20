import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AgentNameContext } from "@/lib/agent";
import { AttachButton } from "./AttachButton";

beforeEach(() => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    text: async () =>
      JSON.stringify({
        ok: true,
        result: { path: ".tariboy/files/x.txt", abs: "/cwd/.tariboy/files/x.txt", bytes: 2 },
      }),
  } as Response));
});
afterEach(() => vi.restoreAllMocks());

describe("AttachButton", () => {
  it("uploads a picked file and reports its absolute host path", async () => {
    const onAttached = vi.fn();
    render(
      <AgentNameContext.Provider value="a1">
        <AttachButton onAttached={onAttached} />
      </AgentNameContext.Provider>,
    );

    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    const file = new File(["hi"], "x.txt", { type: "text/plain" });
    await userEvent.upload(input, file);

    await waitFor(() =>
      expect(onAttached).toHaveBeenCalledWith("/cwd/.tariboy/files/x.txt"),
    );

    // The upload lands under the agent's cwd `.tariboy/files/`.
    const [, init] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(init.method).toBe("PUT");
    expect(JSON.parse(init.body as string).path).toBe(".tariboy/files/x.txt");
  });

  it("renders an Attach button", () => {
    render(
      <AgentNameContext.Provider value="a1">
        <AttachButton onAttached={() => {}} />
      </AgentNameContext.Provider>,
    );
    expect(screen.getByRole("button", { name: /attach/i })).toBeInTheDocument();
  });
});
