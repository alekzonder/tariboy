import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FileContent } from "@/lib/api";
import { FileViewer } from "./FileViewer";

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
vi.mock("@uiw/react-codemirror", () => ({
  default: ({
    value,
    onChange,
    editable,
    readOnly,
  }: {
    value: string;
    onChange?: (value: string) => void;
    editable?: boolean;
    readOnly?: boolean;
  }) => (
    <textarea
      aria-label="File content"
      value={value}
      readOnly={readOnly || editable === false}
      onChange={(event) => onChange?.(event.target.value)}
    />
  ),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const text = (path: string, content: string): FileContent => ({
  path,
  kind: "text",
  content,
  size: content.length,
});
const click = (name: string) => fireEvent.click(screen.getByRole("button", { name }));
const editor = () => screen.getByRole("textbox", { name: "File content" });

function renderViewer(
  path: string | null,
  file: FileContent,
  writeFile?: (path: string, content: string) => Promise<void>,
) {
  const readFile = vi.fn(async () => file);
  const onSaved = vi.fn();
  render(<FileViewer path={path} readFile={readFile} writeFile={writeFile} onSaved={onSaved} />);
  return { readFile, onSaved };
}

describe("FileViewer placeholders", () => {
  it("asks for a file when no path is selected", () => {
    const { readFile } = renderViewer(null, text("a.ts", "x"));
    expect(screen.getByText("Select a file to view")).toBeInTheDocument();
    expect(readFile).not.toHaveBeenCalled();
  });

  it("shows loading until the file arrives", async () => {
    renderViewer("a.ts", text("a.ts", "const a = 1;"));
    expect(screen.getByText("Loading…")).toBeInTheDocument();
    expect(await screen.findByDisplayValue("const a = 1;")).toBeInTheDocument();
  });

  it.each([
    ["binary", "Binary file — not shown (42 bytes)"],
    ["too_large", "File too large to display (42 bytes)"],
  ] as const)("shows %s metadata without an editor", async (kind, message) => {
    renderViewer("a.bin", { path: "a.bin", kind, content: "", size: 42 }, vi.fn());
    expect(await screen.findByText(message)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Edit" })).not.toBeInTheDocument();
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  });
});

describe("FileViewer code mode", () => {
  it("is read-only without writeFile", async () => {
    renderViewer("a.ts", text("a.ts", "const a = 1;"));
    expect(await screen.findByDisplayValue("const a = 1;")).toHaveAttribute("readonly");
    expect(screen.queryByRole("button", { name: "Edit" })).not.toBeInTheDocument();
  });

  it("edits and saves through writeFile", async () => {
    const writeFile = vi.fn().mockResolvedValue(undefined);
    const { onSaved } = renderViewer("a.ts", text("a.ts", "old"), writeFile);
    await screen.findByDisplayValue("old");
    click("Edit");
    expect(editor()).not.toHaveAttribute("readonly");
    fireEvent.change(editor(), { target: { value: "new" } });
    expect(screen.getByTestId("dirty")).toBeInTheDocument();
    click("Save");
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith("a.ts"));
    expect(writeFile).toHaveBeenCalledWith("a.ts", "new");
    expect(screen.queryByTestId("dirty")).not.toBeInTheDocument();
  });

  it("restores the original text on Cancel", async () => {
    const writeFile = vi.fn();
    renderViewer("a.ts", text("a.ts", "old"), writeFile);
    await screen.findByDisplayValue("old");
    click("Edit");
    fireEvent.change(editor(), { target: { value: "discard" } });
    click("Cancel");
    expect(editor()).toHaveValue("old");
    expect(editor()).toHaveAttribute("readonly");
    expect(writeFile).not.toHaveBeenCalled();
  });
});

describe("FileViewer markdown mode", () => {
  it("switches between Preview and Source", async () => {
    renderViewer("doc.md", text("doc.md", "# Title"));
    expect(await screen.findByTestId("md-preview")).toHaveTextContent("Title");
    click("Source");
    expect(screen.getByTestId("md-source")).toHaveTextContent("# Title");
    click("Preview");
    expect(screen.getByTestId("md-preview")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Edit" })).not.toBeInTheDocument();
  });

  it("opens the editable source when editing starts from Preview", async () => {
    renderViewer("doc.md", text("doc.md", "# Title"), vi.fn());
    await screen.findByTestId("md-preview");
    click("Edit");
    expect(editor()).toHaveValue("# Title");
    fireEvent.change(editor(), { target: { value: "# Changed" } });
    click("Preview");
    expect(screen.getByTestId("md-preview")).toHaveTextContent("Changed");
  });
});
