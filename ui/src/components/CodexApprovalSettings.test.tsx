import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { CodexApprovalCard } from "./CodexApprovalSettings";
import { getCodexApprovalOn, setCodexApprovalOn } from "@/lib/api";

vi.mock("@/lib/api", async (importOriginal) => ({
  ...await importOriginal<typeof import("@/lib/api")>(),
  getCodexApprovalOn: vi.fn(),
  setCodexApprovalOn: vi.fn(),
}));

const target = { id: "r", label: "R", baseURL: "https://r.test", token: "t" };

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getCodexApprovalOn).mockResolvedValue({ mode: "yolo" });
  vi.mocked(setCodexApprovalOn).mockImplementation(async (_t, mode) => ({ mode }));
});

describe("CodexApprovalCard", () => {
  it("shows the host's mode", async () => {
    vi.mocked(getCodexApprovalOn).mockResolvedValue({ mode: "approve-for-me" });
    render(<CodexApprovalCard target={target} />);
    expect(await screen.findByRole("radio", { name: "Approve for me" })).toBeChecked();
    expect(screen.getByRole("radio", { name: "Yolo" })).not.toBeChecked();
    expect(getCodexApprovalOn).toHaveBeenCalledWith(target);
  });

  it("saves a new mode to the explicit target", async () => {
    render(<CodexApprovalCard target={target} />);
    fireEvent.click(await screen.findByRole("radio", { name: "Approve for me" }));
    await waitFor(() => expect(setCodexApprovalOn).toHaveBeenCalledWith(target, "approve-for-me"));
    await waitFor(() => expect(screen.getByRole("radio", { name: "Approve for me" })).toBeChecked());
  });

  it("keeps the previous mode and reports a failed save", async () => {
    vi.mocked(setCodexApprovalOn).mockRejectedValue(new Error("daemon down"));
    render(<CodexApprovalCard target={target} />);
    fireEvent.click(await screen.findByRole("radio", { name: "Approve for me" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("daemon down");
    expect(screen.getByRole("radio", { name: "Yolo" })).toBeChecked();
  });

  it("reports a failed load", async () => {
    vi.mocked(getCodexApprovalOn).mockRejectedValue(new Error("unreachable"));
    render(<CodexApprovalCard target={target} />);
    expect(await screen.findByRole("alert")).toHaveTextContent("unreachable");
    expect(screen.queryByRole("radio")).not.toBeInTheDocument();
  });
});
