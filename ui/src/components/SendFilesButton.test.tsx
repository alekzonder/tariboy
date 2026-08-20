import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SendFilesButton } from "./SendFilesButton";
import * as api from "@/lib/api";

beforeEach(() => {
  vi.stubGlobal("fetch", vi.fn().mockImplementation((_url: string, init?: RequestInit) => {
    // Echo the uploaded relative path back as an absolute one.
    const rel = init?.body ? JSON.parse(init.body as string).path : "x";
    return Promise.resolve({
      ok: true,
      status: 200,
      text: async () => JSON.stringify({ ok: true, result: { path: rel, abs: `/cwd/${rel}`, bytes: 2 } }),
    } as Response);
  }));
});
afterEach(() => vi.restoreAllMocks());

describe("SendFilesButton", () => {
  it("renders a Send files button", () => {
    render(<SendFilesButton name="a1" onUploaded={() => {}} />);
    expect(screen.getByRole("button", { name: /send files/i })).toBeInTheDocument();
  });

  it("uploads every picked file and reports their absolute paths", async () => {
    const onUploaded = vi.fn();
    render(<SendFilesButton name="a1" onUploaded={onUploaded} />);

    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    const a = new File(["a"], "a.txt", { type: "text/plain" });
    const b = new File(["b"], "b.txt", { type: "text/plain" });
    await userEvent.upload(input, [a, b]);

    await waitFor(() =>
      expect(onUploaded).toHaveBeenCalledWith([
        "/cwd/.tariboy/files/a.txt",
        "/cwd/.tariboy/files/b.txt",
      ]),
    );

    const calls = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls;
    expect(calls).toHaveLength(2);
    expect(JSON.parse(calls[0][1].body as string).path).toBe(".tariboy/files/a.txt");
  });

  it("passes the target daemon to agentUploadFile", async () => {
    const d = { id: "d1", label: "x", baseURL: "https://h", token: "t" };
    const spy = vi
      .spyOn(api, "agentUploadFile")
      .mockResolvedValue({ path: "a.txt", abs: "/cwd/a.txt", bytes: 1 });
    const onUploaded = vi.fn();
    render(<SendFilesButton name="a1" daemon={d} onUploaded={onUploaded} />);

    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    const a = new File(["a"], "a.txt", { type: "text/plain" });
    await userEvent.upload(input, [a]);

    await waitFor(() => expect(onUploaded).toHaveBeenCalled());
    expect(spy).toHaveBeenCalledWith("a1", expect.anything(), d);
  });
});
