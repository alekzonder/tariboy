import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { ApiError } from "@/lib/api";
import * as sourceApi from "@/lib/imageSources";
import ImageSourceWizard from "./ImageSourceWizard";

vi.mock("@/lib/imageSources", async () => {
  const actual = await vi.importActual<typeof import("@/lib/imageSources")>("@/lib/imageSources");
  return { ...actual, createImageSource: vi.fn() };
});

const create = vi.mocked(sourceApi.createImageSource);

function renderWizard() {
  return render(
    <MemoryRouter initialEntries={["/images/new"]}>
      <Routes>
        <Route path="/images/new" element={<ImageSourceWizard />} />
        <Route path="/images/sources/:name" element={<div>source editor</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

afterEach(() => {
  create.mockReset();
  vi.restoreAllMocks();
});

describe("ImageSourceWizard", () => {
  it("starts with product defaults and sends one structured create request", async () => {
    create.mockResolvedValue({
      schema_version: 1,
      name: "reviewer",
      created_at: "2026-07-29T00:00:00Z",
      updated_at: "2026-07-29T00:00:00Z",
    });
    renderWizard();

    expect(screen.getByLabelText("harness")).toHaveValue("claude");
    expect(screen.getByLabelText("interactive default")).toBeChecked();
    expect(screen.getByRole("button", { name: "Create source" })).toBeDisabled();

    fireEvent.change(screen.getByLabelText("name"), { target: { value: "reviewer" } });
    fireEvent.change(screen.getByLabelText("parent image"), { target: { value: "base:latest" } });
    fireEvent.change(screen.getByLabelText("model"), { target: { value: "gpt-5" } });
    fireEvent.change(screen.getByLabelText("effort"), { target: { value: "high" } });
    fireEvent.change(screen.getByLabelText("capabilities"), { target: { value: "context, status" } });
    fireEvent.change(screen.getByLabelText("initial prompt"), { target: { value: "Review this." } });
    fireEvent.click(screen.getByRole("button", { name: "Create source" }));

    await waitFor(() => expect(create).toHaveBeenCalledWith({
      name: "reviewer",
      from: "base:latest",
      harness: "claude",
      model: "gpt-5",
      effort: "high",
      interactive: true,
      capabilities: ["context", "status"],
      prompt: "Review this.",
    }));
    expect(await screen.findByText("source editor")).toBeInTheDocument();
  });

  it.each([
    ["bad_source", "unsupported harness"],
    ["source_exists", "source reviewer already exists"],
  ])("shows server validation error %s", async (code, message) => {
    create.mockRejectedValue(new ApiError(code === "source_exists" ? 409 : 400, code, message));
    renderWizard();
    fireEvent.change(screen.getByLabelText("name"), { target: { value: "reviewer" } });
    fireEvent.click(screen.getByRole("button", { name: "Create source" }));
    expect(await screen.findByText(message)).toBeInTheDocument();
  });

  it("stays disabled while the create request is pending", async () => {
    create.mockReturnValue(new Promise(() => {}));
    renderWizard();
    fireEvent.change(screen.getByLabelText("name"), { target: { value: "reviewer" } });
    const button = screen.getByRole("button", { name: "Create source" });
    fireEvent.click(button);
    await waitFor(() => expect(screen.getByRole("button", { name: "Creating…" })).toBeDisabled());
  });
});
