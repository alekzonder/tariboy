import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import FileBrowser, { type FileBrowserProps } from "./FileBrowser";
import type { FileContent, FileEntry } from "@/lib/api";
import { toast } from "sonner";

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
vi.mock("@uiw/react-codemirror", () => ({
  default: ({ value, onChange, editable, readOnly }: {
    value: string; onChange?: (value: string) => void; editable?: boolean; readOnly?: boolean;
  }) => <textarea aria-label="File content" value={value} readOnly={readOnly || editable === false}
    onChange={(event) => onChange?.(event.target.value)} />,
}));

afterEach(() => { cleanup(); vi.clearAllMocks(); });

const entry = (name: string, isDir = false): FileEntry => ({ name, isDir, size: 1, mtime: 0 });
const content = (path: string, text = path): FileContent => ({ path, kind: "text", content: text, size: text.length });
function source(overrides: Partial<FileBrowserProps> = {}): FileBrowserProps {
  return {
    sourceKey: "one",
    listDir: vi.fn(async (path: string) => ({ path, entries: path ? [entry("child.md")] : [entry("docs", true), entry("readme.md"), entry("code.ts")] })),
    readFile: vi.fn(async (path: string) => content(path, `# ${path}`)),
    ...overrides,
  };
}
const click = (name: string) => fireEvent.click(screen.getByRole("button", { name }));
async function select(name = "readme.md") { await screen.findByRole("button", { name }); click(name); }
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}

describe("FileBrowser public integration", () => {
  it("loads the root, caches collapsed directories and selects nested files", async () => {
    const props = source();
    render(<FileBrowser {...props} />);
    await select("docs");
    await screen.findByRole("button", { name: "child.md" });
    click("docs");
    expect(screen.queryByRole("button", { name: "child.md" })).not.toBeInTheDocument();
    click("docs");
    click("child.md");
    await screen.findByTestId("md-preview");
    expect(props.listDir).toHaveBeenCalledTimes(2);
    expect(props.readFile).toHaveBeenCalledWith("docs/child.md");
  });

  it("resets selection on source changes and ignores a late previous read", async () => {
    const late = deferred<FileContent>();
    const first = source({ readFile: vi.fn(() => late.promise) });
    const view = render(<FileBrowser {...first} />);
    await select();
    const second = source({ sourceKey: "two" });
    view.rerender(<FileBrowser {...second} />);
    expect(screen.getByText("Select a file to view")).toBeInTheDocument();
    await select("code.ts");
    await screen.findByDisplayValue("# code.ts");
    await act(async () => late.resolve(content("readme.md", "old source")));
    expect(screen.queryByText("old source")).not.toBeInTheDocument();
    expect(screen.getByDisplayValue("# code.ts")).toBeInTheDocument();
  });

  it("ignores late reads after choosing a different path", async () => {
    const late = deferred<FileContent>();
    render(<FileBrowser {...source({ readFile: vi.fn((path) => path === "readme.md" ? late.promise : Promise.resolve(content(path))) })} />);
    await select();
    click("code.ts");
    await screen.findByDisplayValue("code.ts");
    await act(async () => late.resolve(content("readme.md", "late text")));
    expect(screen.getByDisplayValue("code.ts")).toBeInTheDocument();
  });

  it("switches markdown preview/source and preserves drafts after save errors", async () => {
    const writeFile = vi.fn().mockRejectedValueOnce(new Error("write failed")).mockResolvedValue(undefined);
    render(<FileBrowser {...source({ writeFile })} />);
    await select();
    await screen.findByTestId("md-preview");
    click("Source");
    expect(screen.getByTestId("md-source")).toHaveTextContent("# readme.md");
    click("Preview");
    click("Edit");
    fireEvent.change(screen.getByRole("textbox", { name: "File content" }), { target: { value: "# changed" } });
    click("Preview");
    expect(screen.getByTestId("md-preview")).toHaveTextContent("changed");
    click("Save");
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Error: write failed"));
    expect(screen.getByTestId("dirty")).toBeInTheDocument();
    click("Source");
    expect(screen.getByDisplayValue("# changed")).toBeInTheDocument();
    click("Save");
    await waitFor(() => expect(screen.queryByTestId("dirty")).not.toBeInTheDocument());
    expect(writeFile).toHaveBeenLastCalledWith("readme.md", "# changed");
    fireEvent.change(screen.getByRole("textbox", { name: "File content" }), { target: { value: "discard" } });
    click("Cancel");
    expect(screen.getByTestId("md-source")).toHaveTextContent("# changed");
  });

  it.each([false, true])("keeps the code viewer read-only without write access (forced=%s)", async (readOnly) => {
    const callbacks = { writeFile: vi.fn(), createFile: vi.fn(), renameFile: vi.fn(), deleteFile: vi.fn() };
    render(<FileBrowser {...source(readOnly ? { ...callbacks, readOnly } : {})} />);
    await select("code.ts");
    expect(await screen.findByDisplayValue("# code.ts")).toHaveAttribute("readonly");
    for (const name of ["Edit", "New file", "New folder", "Rename code.ts", "Delete code.ts"]) {
      expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
    }
  });

  it.each(["create", "rename", "delete"] as const)("enables %s independently of writing and other callbacks", async (operation) => {
    const callbacks = { createFile: vi.fn(), renameFile: vi.fn(), deleteFile: vi.fn() };
    render(<FileBrowser {...source({ [`${operation}File`]: callbacks[`${operation}File`] })} />);
    await screen.findByRole("button", { name: "code.ts" });
    expect(!!screen.queryByRole("button", { name: "New file" })).toBe(operation === "create");
    expect(!!screen.queryByRole("button", { name: "Rename code.ts" })).toBe(operation === "rename");
    expect(!!screen.queryByRole("button", { name: "Delete code.ts" })).toBe(operation === "delete");
  });

  it.each(["binary", "too_large"] as const)("shows %s metadata without an editor", async (kind) => {
    render(<FileBrowser {...source({ readFile: vi.fn(async (path) => ({ ...content(path), kind, size: 123 })), writeFile: vi.fn() })} />);
    await select();
    await screen.findByText(kind === "binary" ? "Binary file — not shown (123 bytes)" : "File too large to display (123 bytes)");
    expect(screen.queryByRole("button", { name: "Edit" })).not.toBeInTheDocument();
  });

  it.each([false, true])("requires confirmation for existing or unreadable rename targets (unreadable=%s)", async (unreadable) => {
    const renameFile = vi.fn().mockResolvedValue(undefined);
    const props = source({ renameFile });
    const rootList = props.listDir;
    props.listDir = vi.fn((path) => path === "hidden" ? (unreadable ? Promise.reject(new Error("denied")) : Promise.resolve({ path, entries: [entry("target")] })) : rootList(path));
    render(<FileBrowser {...props} />);
    await screen.findByRole("button", { name: "Rename code.ts" });
    click("Rename code.ts");
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "hidden/target" } });
    click("Rename");
    await screen.findByRole("alertdialog");
    expect(renameFile).not.toHaveBeenCalled();
    click("Cancel");
    expect(renameFile).not.toHaveBeenCalled();
    click("Rename code.ts");
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "hidden/target" } });
    click("Rename");
    await screen.findByRole("alertdialog");
    click("Overwrite");
    await waitFor(() => expect(renameFile).toHaveBeenCalledWith("code.ts", "hidden/target"));
  });

  it("cancels deletion without invoking the callback", async () => {
    const deleteFile = vi.fn();
    render(<FileBrowser {...source({ deleteFile })} />);
    await screen.findByRole("button", { name: "Delete code.ts" });
    click("Delete code.ts");
    click("Cancel");
    expect(deleteFile).not.toHaveBeenCalled();
  });

  it("refreshes the new parent and loaded ancestor after nested creation", async () => {
    const createFile = vi.fn().mockResolvedValue(undefined);
    const props = source({ createFile });
    render(<FileBrowser {...props} />);
    await screen.findByRole("button", { name: "New file" });
    await screen.findByRole("button", { name: "code.ts" });
    click("New file");
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "new/nested/file.txt" } });
    click("Create");
    await waitFor(() => expect(props.listDir).toHaveBeenCalledWith("new/nested"));
    expect(props.listDir).toHaveBeenNthCalledWith(3, "");
    expect(createFile).toHaveBeenCalledWith("new/nested/file.txt", "file");
  });
});
