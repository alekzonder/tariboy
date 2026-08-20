import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import * as sourceApi from "@/lib/imageSources";
import ImageSourceEditor from "./ImageSourceEditor";

vi.mock("@uiw/react-codemirror", () => ({
  default: ({
    value,
    onChange,
  }: {
    value: string;
    onChange?: (value: string) => void;
  }) => (
    <textarea
      aria-label="source editor"
      value={value}
      onChange={(event) => onChange?.(event.target.value)}
    />
  ),
}));

vi.mock("@/lib/imageSources", async () => {
  const actual = await vi.importActual<typeof import("@/lib/imageSources")>("@/lib/imageSources");
  return {
    ...actual,
    getImageSource: vi.fn(),
    listImageSourceFiles: vi.fn(),
    getImageSourceFile: vi.fn(),
    putImageSourceFile: vi.fn(),
    validateImageSource: vi.fn(),
    buildImageSource: vi.fn(),
  };
});

const getSource = vi.mocked(sourceApi.getImageSource);
const listFiles = vi.mocked(sourceApi.listImageSourceFiles);
const getFile = vi.mocked(sourceApi.getImageSourceFile);
const putFile = vi.mocked(sourceApi.putImageSourceFile);
const validate = vi.mocked(sourceApi.validateImageSource);
const build = vi.mocked(sourceApi.buildImageSource);

function seed() {
  getSource.mockResolvedValue({
    schema_version: 1,
    name: "reviewer",
    created_at: "2026-07-29T00:00:00Z",
    updated_at: "2026-07-29T00:00:00Z",
  });
  listFiles.mockResolvedValue({
    files: [
      { path: ".tariboy-source.json", size: 10 },
      { path: "PROMPT.md", size: 7 },
      { path: "Tariboyfile.yaml", size: 30 },
    ],
    count: 3,
  });
  getFile.mockImplementation(async (_name, path) => ({
    path,
    content: path === "PROMPT.md" ? "Review." : "schema_version: 1\n",
  }));
  putFile.mockResolvedValue({ path: "Tariboyfile.yaml", saved: true });
  validate.mockResolvedValue({ valid: true, diagnostics: [] });
  build.mockResolvedValue({
    ref: "reviewer:latest",
    digest: "sha256:deadbeef",
    built_at: "2026-07-29T00:00:00Z",
    layers: 3,
  });
}

function renderEditor() {
  return render(
    <MemoryRouter initialEntries={["/images/sources/reviewer"]}>
      <Routes>
        <Route path="/images/sources/:name" element={<ImageSourceEditor />} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(seed);
afterEach(() => {
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

describe("ImageSourceEditor", () => {
  it("hides metadata, does not autosave, and saves explicitly", async () => {
    renderEditor();
    expect(await screen.findByRole("button", { name: "Tariboyfile.yaml" })).toBeInTheDocument();
    expect(screen.queryByText(".tariboy-source.json")).not.toBeInTheDocument();
    const editor = await screen.findByLabelText("source editor");
    fireEvent.change(editor, { target: { value: "schema_version: 1\nharness:\n  type: codex\n" } });
    expect(putFile).not.toHaveBeenCalled();

    const unload = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(unload);
    expect(unload.defaultPrevented).toBe(true);

    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(putFile).toHaveBeenCalledWith(
      "reviewer",
      "Tariboyfile.yaml",
      "schema_version: 1\nharness:\n  type: codex\n",
    ));
  });

  it("selects the diagnostic file and builds with a visible result", async () => {
    validate.mockResolvedValue({
      valid: false,
      diagnostics: [{ path: "PROMPT.md", message: "prompt is empty" }],
    });
    const built = vi.fn();
    window.addEventListener("tariboy:image-built", built);
    renderEditor();
    await screen.findByLabelText("source editor");

    fireEvent.click(screen.getByRole("button", { name: "Validate" }));
    const diagnostic = await screen.findByRole("button", { name: /PROMPT.md.*prompt is empty/i });
    fireEvent.click(diagnostic);
    await waitFor(() => expect(getFile).toHaveBeenCalledWith("reviewer", "PROMPT.md"));

    fireEvent.click(screen.getByRole("button", { name: "Build" }));
    expect(await screen.findAllByText("reviewer:latest")).toHaveLength(2);
    expect(screen.getAllByText("sha256:deadbeef")).toHaveLength(2);
    expect(built).toHaveBeenCalledTimes(1);
    window.removeEventListener("tariboy:image-built", built);
  });
});
