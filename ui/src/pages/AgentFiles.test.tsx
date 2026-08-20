import { it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { AgentNameContext } from "@/lib/agent";
import { ThemeProvider } from "@/components/theme-provider";
import AgentFiles from "./AgentFiles";

afterEach(() => vi.restoreAllMocks());

interface Call { method: string; url: string; body: unknown }

// Route each request by its path to a canned {ok,result} envelope and record
// every call so mutation tests can assert method + body. The root listing
// carries a folder + a markdown file; the markdown file returns a text body.
function stubFetch(): Call[] {
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      calls.push({
        method: init?.method ?? "GET",
        url,
        body: init?.body ? JSON.parse(init.body as string) : undefined,
      });
      let result: unknown = {};
      if (url.includes("/file/list?path=&") || url.endsWith("/file/list?path=")) {
        result = {
          path: "",
          entries: [
            { name: "docs", isDir: true, size: 0, mtime: 1752278400 },
            { name: "README.md", isDir: false, size: 12, mtime: 1752278400 },
          ],
        };
      } else if (url.includes("/file/list?path=docs")) {
        result = {
          path: "docs",
          entries: [{ name: "guide.txt", isDir: false, size: 3, mtime: 1752278400 }],
        };
      } else if (url.includes("/file?path=README.md")) {
        result = { path: "README.md", kind: "text", content: "# Hello world\n\nbody text", size: 12 };
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        text: async () => JSON.stringify({ ok: true, result }),
      } as Response);
    }),
  );
  return calls;
}

function renderPage() {
  return render(
    <ThemeProvider>
      <AgentNameContext.Provider value="alpha">
        <AgentFiles />
      </AgentNameContext.Provider>
    </ThemeProvider>,
  );
}

it("renders the root listing and lazily expands a folder", async () => {
  stubFetch();
  renderPage();

  // Root entries appear.
  await waitFor(() => expect(screen.getByText("README.md")).toBeInTheDocument());
  expect(screen.getByText("docs")).toBeInTheDocument();

  // Folder children are not fetched until expanded.
  expect(screen.queryByText("guide.txt")).not.toBeInTheDocument();
  fireEvent.click(screen.getByText("docs"));
  await waitFor(() => expect(screen.getByText("guide.txt")).toBeInTheDocument());
});

it("opens a markdown file as a preview and toggles to source", async () => {
  stubFetch();
  renderPage();

  await waitFor(() => expect(screen.getByText("README.md")).toBeInTheDocument());
  fireEvent.click(screen.getByText("README.md"));

  // Preview renders markdown → an <h1> from the heading, not the raw "#".
  await waitFor(() => expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Hello world"));

  // Toggle to Source shows the raw markdown text.
  fireEvent.click(screen.getByText("Source"));
  await waitFor(() => expect(screen.getByTestId("md-source")).toHaveTextContent("# Hello world"));
});

it("saves the open file with PUT {path, content}", async () => {
  const calls = stubFetch();
  renderPage();

  await waitFor(() => expect(screen.getByText("README.md")).toBeInTheDocument());
  fireEvent.click(screen.getByText("README.md"));
  await waitFor(() => expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Hello world"));

  // Enter edit mode (Preview stays mounted — no CodeMirror needed) and save.
  fireEvent.click(screen.getByText("Edit"));
  fireEvent.click(screen.getByText("Save"));

  await waitFor(() => {
    const put = calls.find((c) => c.method === "PUT");
    expect(put).toBeTruthy();
    expect(put?.url).toContain("/file");
    expect(put?.body).toEqual({ path: "README.md", content: "# Hello world\n\nbody text" });
  });
});

it("creates a file with POST {path, type}", async () => {
  const calls = stubFetch();
  renderPage();

  await waitFor(() => expect(screen.getByText("README.md")).toBeInTheDocument());
  fireEvent.click(screen.getByLabelText("New file"));

  const input = await screen.findByRole("textbox");
  fireEvent.change(input, { target: { value: "notes.txt" } });
  fireEvent.click(screen.getByText("Create"));

  await waitFor(() => {
    const post = calls.find((c) => c.method === "POST" && c.url.includes("/file"));
    expect(post).toBeTruthy();
    expect(post?.body).toEqual({ path: "notes.txt", type: "file" });
  });
});

it("confirms before overwriting when renaming into a collapsed folder", async () => {
  const calls = stubFetch();
  renderPage();

  await waitFor(() => expect(screen.getByText("README.md")).toBeInTheDocument());

  // "docs" is never expanded, so its listing is unknown to the loaded tree.
  // Renaming into docs/guide.txt (which already exists there) must still be
  // caught: resolveExists fetches the parent and the Overwrite confirm fires.
  fireEvent.click(screen.getByLabelText("Rename README.md"));
  const input = await screen.findByRole("textbox");
  fireEvent.change(input, { target: { value: "docs/guide.txt" } });
  fireEvent.click(screen.getByRole("button", { name: /^Rename$/ }));

  // Confirm dialog appears and NO rename request has fired yet.
  await screen.findByText("Overwrite?");
  expect(calls.some((c) => c.url.includes("/file/rename"))).toBe(false);

  // Confirming issues the rename with the from/to pair.
  fireEvent.click(screen.getByRole("button", { name: /^Overwrite$/ }));
  await waitFor(() => {
    const rn = calls.find((c) => c.url.includes("/file/rename"));
    expect(rn?.body).toEqual({ from: "README.md", to: "docs/guide.txt" });
  });
});

it("reveals a newly created nested directory by refreshing the nearest loaded ancestor", async () => {
  const calls: Call[] = [];
  let created = false;
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      calls.push({
        method: init?.method ?? "GET",
        url,
        body: init?.body ? JSON.parse(init.body as string) : undefined,
      });
      let result: unknown = {};
      if (url.includes("/file/list?path=&") || url.endsWith("/file/list?path=")) {
        // Root gains the new "sub" folder only once it has been created.
        result = { path: "", entries: created ? [{ name: "sub", isDir: true, size: 0, mtime: 1752278400 }] : [] };
      } else if (url.includes("/file/list?path=sub")) {
        result = { path: "sub", entries: created ? [{ name: "a.txt", isDir: false, size: 0, mtime: 1752278400 }] : [] };
      } else if ((init?.method ?? "GET") === "POST") {
        created = true;
        result = { path: "sub/a.txt", created: true };
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        text: async () => JSON.stringify({ ok: true, result }),
      } as Response);
    }),
  );

  renderPage();
  await waitFor(() => expect(screen.getByText("Working directory is empty")).toBeInTheDocument());

  // Create a nested path whose intermediate "sub" dir does not yet exist.
  fireEvent.click(screen.getByLabelText("New file"));
  const input = await screen.findByRole("textbox");
  fireEvent.change(input, { target: { value: "sub/a.txt" } });
  fireEvent.click(screen.getByText("Create"));

  // The nearest already-loaded ancestor (root) is refetched, so the new "sub"
  // directory becomes visible without a full page reload.
  await waitFor(() => expect(screen.getByText("sub")).toBeInTheDocument());
});

it("deletes a file only after confirming, sending DELETE", async () => {
  const calls = stubFetch();
  renderPage();

  await waitFor(() => expect(screen.getByText("README.md")).toBeInTheDocument());

  // Clicking delete opens a confirm dialog; no DELETE yet.
  fireEvent.click(screen.getByLabelText("Delete README.md"));
  expect(calls.some((c) => c.method === "DELETE")).toBe(false);

  // Confirm → DELETE with the path in the query string. Match the bare
  // "Delete" action, not the row buttons ("Delete README.md" / "Delete docs").
  const confirmBtn = await screen.findByRole("button", { name: /^Delete$/ });
  fireEvent.click(confirmBtn);

  await waitFor(() => {
    const del = calls.find((c) => c.method === "DELETE");
    expect(del).toBeTruthy();
    expect(del?.url).toContain("/file?path=README.md");
  });
});
