import { act, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import FileBrowser from "./FileBrowser";

// The viewer carries CodeMirror and its languages; the tree must not wait for it.
const gate = vi.hoisted(() => {
  let release!: () => void;
  const promise = new Promise<void>((resolve) => {
    release = resolve;
    setTimeout(resolve, 10_000);
  });
  return { promise, release };
});

vi.mock("./file-browser/FileViewer", async () => {
  await gate.promise;
  return { FileViewer: () => <p>Viewer ready</p> };
});

describe("FileBrowser loading", () => {
  it("shows the tree while the file viewer loads", async () => {
    render(<FileBrowser
      sourceKey="one"
      listDir={async (path) => ({ path, entries: [{ name: "readme.md", isDir: false, size: 1, mtime: 0 }] })}
      readFile={async (path) => ({ path, kind: "text", content: "", size: 0 })}
    />);

    expect(await screen.findByRole("button", { name: "readme.md" })).toBeInTheDocument();
    expect(screen.getByText("Loading…", { selector: "[role=status]" })).toBeInTheDocument();

    await act(async () => gate.release());
    expect(await screen.findByText("Viewer ready")).toBeInTheDocument();
  });
});
