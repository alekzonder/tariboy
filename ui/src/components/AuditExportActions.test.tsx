import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AuditExportActions } from "@/components/AuditExportActions";
import { setActiveDaemon, setLocalBaseURL } from "@/lib/api";

describe("AuditExportActions", () => {
  const writeText = vi.fn();

  beforeEach(() => {
    setActiveDaemon(null);
    setLocalBaseURL("");
    writeText.mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
    vi.stubGlobal("fetch", vi.fn((url: string) => {
      if (url.includes("format=markdown")) return Promise.resolve(new Response("# Audit log\n\nCommand — rg --files", { status: 200 }));
      return Promise.resolve({ ok: true, status: 200, blob: () => Promise.resolve(new Blob(["zip"])) } as Response);
    }));
    vi.stubGlobal("URL", { ...URL, createObjectURL: vi.fn(() => "blob:audit"), revokeObjectURL: vi.fn() });
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
  });

  afterEach(() => vi.restoreAllMocks());

  it("copies readable Markdown for one iteration", async () => {
    render(<AuditExportActions name="alice" iteration="iter-1" />);
    await userEvent.click(screen.getByRole("button", { name: "Copy audit log" }));

    await waitFor(() => expect(writeText).toHaveBeenCalledWith("# Audit log\n\nCommand — rg --files"));
    expect(fetch).toHaveBeenCalledWith(
      "/api/agents/alice/audit-export?iteration=iter-1&format=markdown",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("warns before downloading the full sensitive ZIP", async () => {
    render(<AuditExportActions name="alice" />);
    await userEvent.click(screen.getByRole("button", { name: "Export audit log" }));

    expect(screen.getByRole("alertdialog")).toHaveTextContent(/prompts, reasoning, commands, tool arguments\/results/i);
    await userEvent.click(screen.getByRole("button", { name: "Download ZIP" }));

    await waitFor(() => expect(fetch).toHaveBeenCalledWith(
      "/api/agents/alice/audit-export",
      expect.objectContaining({ method: "GET" }),
    ));
    await waitFor(() => expect(HTMLAnchorElement.prototype.click).toHaveBeenCalled());
  });
});
