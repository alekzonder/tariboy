import { fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { CodexApprovalCard } from "./CodexApprovalSettings";
import { render } from "@/test/queryRender";
import { fakeApi } from "@/test/api";
import type { CodexApprovalMode } from "@/lib/api";

const target = { id: "r", label: "R", baseURL: "https://r.test", token: "t" };

const getCodexApproval = vi.fn<() => Promise<{ mode: CodexApprovalMode }>>();
const setCodexApproval = vi.fn<(mode: CodexApprovalMode) => Promise<{ mode: CodexApprovalMode }>>();
// The client of the explicit host: every request goes through it.
const hostApi = () => fakeApi({ harnesses: { getCodexApproval, setCodexApproval } }, target);

beforeEach(() => {
  vi.clearAllMocks();
  getCodexApproval.mockResolvedValue({ mode: "yolo" });
  setCodexApproval.mockImplementation(async (mode) => ({ mode }));
});

describe("CodexApprovalCard", () => {
  it("shows the host's mode", async () => {
    getCodexApproval.mockResolvedValue({ mode: "approve-for-me" });
    const api = hostApi();
    render(<CodexApprovalCard />, { api });
    expect(await screen.findByRole("radio", { name: "Approve for me" })).toBeChecked();
    expect(screen.getByRole("radio", { name: "Yolo" })).not.toBeChecked();
    expect(getCodexApproval).toHaveBeenCalledWith();
    expect(api.target).toBe(target);
  });

  it("saves a new mode to the explicit target", async () => {
    const api = hostApi();
    render(<CodexApprovalCard />, { api });
    fireEvent.click(await screen.findByRole("radio", { name: "Approve for me" }));
    await waitFor(() => expect(setCodexApproval).toHaveBeenCalledWith("approve-for-me"));
    expect(api.target).toBe(target);
    await waitFor(() => expect(screen.getByRole("radio", { name: "Approve for me" })).toBeChecked());
  });

  it("keeps the previous mode and reports a failed save", async () => {
    setCodexApproval.mockRejectedValue(new Error("daemon down"));
    render(<CodexApprovalCard />, { api: hostApi() });
    fireEvent.click(await screen.findByRole("radio", { name: "Approve for me" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("daemon down");
    expect(screen.getByRole("radio", { name: "Yolo" })).toBeChecked();
  });

  it("reports a failed load", async () => {
    getCodexApproval.mockRejectedValue(new Error("unreachable"));
    render(<CodexApprovalCard />, { api: hostApi() });
    expect(await screen.findByRole("alert")).toHaveTextContent("unreachable");
    expect(screen.queryByRole("radio")).not.toBeInTheDocument();
  });
});
