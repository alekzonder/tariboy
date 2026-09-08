import { it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import AgentMessages from "./AgentMessages";
import { AgentNameContext } from "@/lib/agent";

afterEach(() => vi.restoreAllMocks());

interface Post { path: string; body: unknown }
interface StubOptions {
  archive?: unknown[];
  dlq?: unknown[];
  pendingGate?: Promise<void>;
  postGate?: Promise<void>;
  clearError?: boolean;
}

// Stub fetch for the P5 inbox endpoints. `queue` rows back the pending view;
// POSTs are recorded so the tests can assert the operator actions fired.
function stubInbox(queue: unknown[], opts: StubOptions = {}) {
  const posts: Post[] = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string, init?: RequestInit) => {
    let result: unknown = {};
    let gate: Promise<void> | undefined;
    if (init?.method === "POST") {
      posts.push({ path, body: init?.body ? JSON.parse(init.body as string) : undefined });
      if (opts.clearError && path.endsWith("/inbox/clear")) {
        return Promise.resolve({
          ok: false,
          status: 500,
          text: async () => JSON.stringify({ ok: false, error: { code: "internal", message: "clear failed" } }),
        } as Response);
      }
      result = { ok: true };
      gate = opts.postGate;
    } else if (path.includes("/inbox")) {
      let rows = path.includes("status=processed") ? (opts.archive ?? [])
        : path.includes("status=dlq") ? (opts.dlq ?? [])
        : queue;
      const url = new URL(path, "http://localhost");
      const before = url.searchParams.get("before");
      if (before) rows = rows.slice(rows.findIndex((item) => (item as { id?: string }).id === before) + 1);
      rows = rows.slice(0, Number(url.searchParams.get("limit") ?? 100));
      result = { messages: rows, count: rows.length };
      if (url.searchParams.get("status") === "pending") gate = opts.pendingGate;
    }
    return (gate ?? Promise.resolve()).then(() =>
      ({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result }) } as Response),
    );
  }));
  return posts;
}

function renderPage() {
  return render(
    <AgentNameContext.Provider value="worker">
      <MemoryRouter initialEntries={["/agent/worker/messages"]}>
        <Routes>
          <Route path="/agent/:name/messages" element={<AgentMessages />} />
        </Routes>
      </MemoryRouter>
    </AgentNameContext.Provider>,
  );
}

const row = (id: string, ts: string, text: string, extra: Record<string, unknown> = {}) =>
  ({ id, ts, source: "op", type: "note", text, attempts: 1, dlq: false, ...extra });

it("renders the queue newest-first as returned by the backend", async () => {
  // Backend returns newest-first; the component must preserve that order.
  stubInbox([
    row("m2", "2026-07-12T10:02:00Z", "newer message"),
    row("m1", "2026-07-12T10:01:00Z", "older message"),
  ]);
  renderPage();

  await waitFor(() => expect(screen.getByText("newer message")).toBeInTheDocument());
  const texts = screen.getAllByText(/message$/).map((n) => n.textContent);
  expect(texts).toEqual(["newer message", "older message"]);
});

it("keeps authoritative queue rows when clear fails", async () => {
  stubInbox([row("m1", "2026-07-12T10:00:00Z", "keep me")], { clearError: true });
  renderPage();

  await screen.findByText("keep me");
  fireEvent.click(screen.getByRole("button", { name: "Clear queue" }));
  fireEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Clear queue" }));

  await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
  expect(screen.getByText("keep me")).toBeInTheDocument();
});

it("marks a queue row processed via the dialog (non-empty result required)", async () => {
  const posts = stubInbox([row("m1", "2026-07-12T10:00:00Z", "please ack")]);
  renderPage();

  await waitFor(() => expect(screen.getByText("please ack")).toBeInTheDocument());
  fireEvent.click(screen.getByText("Mark processed"));

  const dialog = await screen.findByRole("dialog");
  // Empty result must NOT submit.
  fireEvent.click(within(dialog).getByRole("button", { name: "Mark processed" }));
  expect(posts.some((p) => p.path.includes("/processed"))).toBe(false);

  // A non-empty result submits to the processed endpoint.
  fireEvent.change(within(dialog).getByPlaceholderText("result"), { target: { value: "handled" } });
  fireEvent.click(within(dialog).getByRole("button", { name: "Mark processed" }));
  await waitFor(() =>
    expect(posts.some((p) => p.path.endsWith("/inbox/m1/processed") && (p.body as { result?: string }).result === "handled")).toBe(true),
  );
});

it("physically clears the pending queue with one guarded request", async () => {
  const pending = [row("m1", "2026-07-12T10:00:00Z", "message 1")];
  let releasePosts!: () => void;
  const posts = stubInbox(pending, { postGate: new Promise((resolve) => { releasePosts = resolve; }) });
  renderPage();

  await waitFor(() => expect(screen.getByText("message 1")).toBeInTheDocument());
  fireEvent.click(screen.getByRole("button", { name: "Clear queue" }));

  const dialog = await screen.findByRole("dialog");
  expect(dialog).toHaveTextContent("cannot be recovered");
  expect(dialog).toHaveTextContent("Archive and DLQ are not changed");
  fireEvent.click(within(dialog).getByRole("button", { name: "Clear queue" }));

  await waitFor(() => expect(posts).toHaveLength(1));
  expect(within(dialog).getByRole("button", { name: "Cancel" })).toBeDisabled();
  fireEvent.keyDown(document, { key: "Escape" });
  expect(dialog).toBeInTheDocument();
  releasePosts();
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(posts[0].path).toMatch(/\/inbox\/clear$/);
});

it("does not offer the bulk action for stale rows from another view", async () => {
  stubInbox([], {
    archive: [row("a1", "2026-07-12T09:00:00Z", "archived")],
    pendingGate: new Promise(() => {}),
  });
  renderPage();

  await userEvent.click(screen.getByRole("tab", { name: "Archive" }));
  await waitFor(() => expect(screen.getByText("archived")).toBeInTheDocument());
  await userEvent.click(screen.getByRole("tab", { name: "Queue" }));

  expect(screen.queryByRole("button", { name: "Clear queue" })).not.toBeInTheDocument();
});

it("replies to a queue row via the dialog", async () => {
  const posts = stubInbox([row("m1", "2026-07-12T10:00:00Z", "question?", { kind: "request" })]);
  renderPage();

  await waitFor(() => expect(screen.getByText("question?")).toBeInTheDocument());
  // The request kind renders a badge.
  expect(screen.getByText("request")).toBeInTheDocument();
  fireEvent.click(screen.getByText("Reply"));

  const dialog = await screen.findByRole("dialog");
  fireEvent.change(within(dialog).getByPlaceholderText("reply text"), { target: { value: "yes" } });
  fireEvent.click(within(dialog).getByRole("button", { name: "Reply" }));
  await waitFor(() =>
    expect(posts.some((p) => p.path.endsWith("/inbox/m1/reply") && (p.body as { text?: string }).text === "yes")).toBe(true),
  );
});

it("shows archive rows with result + processed time and no actions", async () => {
  stubInbox([], {
    archive: [row("a1", "2026-07-12T09:00:00Z", "done one", {
      processed_at: "2026-07-12T09:05:00Z", result: "operator: ok",
    })],
  });
  renderPage();

  await userEvent.click(screen.getByRole("tab", { name: "Archive" }));
  await waitFor(() => expect(screen.getByText("done one")).toBeInTheDocument());
  expect(screen.getByText(/operator: ok/)).toBeInTheDocument();
  // Archive rows are read-only.
  expect(screen.queryByText("Mark processed")).toBeNull();
  expect(screen.queryByText("Reply")).toBeNull();
});

it("requeues a DLQ row", async () => {
  const posts = stubInbox([], {
    dlq: [row("d1", "2026-07-12T08:00:00Z", "failed msg", { dlq: true, attempts: 5 })],
  });
  renderPage();

  await userEvent.click(screen.getByRole("tab", { name: "DLQ" }));
  await waitFor(() => expect(screen.getByText("failed msg")).toBeInTheDocument());
  fireEvent.click(screen.getByText("Requeue"));
  await waitFor(() =>
    expect(posts.some((p) => p.path.endsWith("/inbox/d1/requeue"))).toBe(true),
  );
});
