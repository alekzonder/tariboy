import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SupportBundle } from "./SupportBundle";
import { supportBundleExport } from "@/lib/desktop";
import { getActiveId, listDaemons } from "@/lib/daemons";

vi.mock("@/lib/desktop", () => ({
  isDesktop: () => true,
  supportBundleExport: vi.fn(),
}));

vi.mock("@/lib/daemons", () => ({
  getActiveId: vi.fn(),
  listDaemons: vi.fn(),
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(listDaemons).mockResolvedValue([
    { id: "ssh-1", label: "Build box", baseURL: "", kind: "ssh" },
    { id: "https-1", label: "Prod API", baseURL: "https://prod", kind: "https" },
  ]);
  vi.mocked(getActiveId).mockResolvedValue("ssh-1");
});

describe("SupportBundle", () => {
  it("requires one host and keeps sensitive agent data unchecked by default", async () => {
    vi.mocked(supportBundleExport).mockResolvedValue({
      path: "/tmp/tariboy-support-build-box-20260729-103806.zip",
    });
    render(<SupportBundle />);

    const selector = await screen.findByRole("combobox", { name: "Host" });
    expect(selector).toHaveValue("ssh-1");
    expect(screen.getByRole("option", { name: "Local" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Build box" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Prod API" })).toBeInTheDocument();
    const consent = screen.getByRole("checkbox", { name: "Include agent data (sensitive)" });
    expect(consent).not.toBeChecked();
    expect(screen.getByText(/newest 10 iterations for each agent/i)).toBeInTheDocument();
    expect(screen.getByText(/never includes PROMPT\.md, transcripts, secrets, environment values, workdirs, or user files/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Export support bundle" }));

    await waitFor(() =>
      expect(supportBundleExport).toHaveBeenCalledWith("ssh-1", false),
    );
    expect(await screen.findByRole("status")).toHaveTextContent("build-box");
  });

  it("passes the selected host and explicit consent to native export", async () => {
    vi.mocked(supportBundleExport).mockResolvedValue(null);
    render(<SupportBundle />);

    const selector = await screen.findByRole("combobox", { name: "Host" });
    fireEvent.change(selector, { target: { value: "https-1" } });
    fireEvent.click(screen.getByRole("checkbox", { name: "Include agent data (sensitive)" }));
    fireEvent.click(screen.getByRole("button", { name: "Export support bundle" }));

    await waitFor(() =>
      expect(supportBundleExport).toHaveBeenCalledWith("https-1", true),
    );
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("falls back to local and shows an export error without claiming a path", async () => {
    vi.mocked(getActiveId).mockResolvedValue("removed-host");
    vi.mocked(supportBundleExport).mockRejectedValue(new Error("disk full"));
    render(<SupportBundle />);

    const selector = await screen.findByRole("combobox", { name: "Host" });
    expect(selector).toHaveValue("local");
    fireEvent.click(screen.getByRole("button", { name: "Export support bundle" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("disk full");
    expect(screen.queryByRole("status")).toBeNull();
  });
});
