import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AttachButton } from "./AttachButton";

beforeEach(() => {
  const envelope = {
    ok: true,
    result: { path: "files/one/x.txt", abs: "/server/files/one/x.txt", bytes: 2 },
  };
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    text: async () => JSON.stringify(envelope),
    json: async () => envelope,
  } as Response));
});
afterEach(() => vi.restoreAllMocks());

describe("AttachButton", () => {
  it("uploads a picked file and reports its absolute host path", async () => {
    const onAttached = vi.fn();
    render(<AttachButton onAttached={onAttached} />);

    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    const file = new File(["hi"], "x.txt", { type: "text/plain" });
    await userEvent.upload(input, file);

    await waitFor(() =>
      expect(onAttached).toHaveBeenCalledWith("/server/files/one/x.txt"),
    );

    // The upload uses the shared server directory without agent context.
    const [url, init] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toBe("/api/files/raw?name=x.txt");
    expect(init.method).toBe("PUT");
    expect(init.body).toBe(file);
  });

  it("renders an Attach button", () => {
    render(<AttachButton onAttached={() => {}} />);
    expect(screen.getByRole("button", { name: /attach/i })).toBeInTheDocument();
  });
});
