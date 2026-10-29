import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { Operation, OperationLog } from "@/lib/api";
import { apiWrapper, fakeApi } from "@/test/api";
import { FakeWebSocket } from "@/test/fakeWebSocket";
import { operation } from "@/test/fixtures/apiFixtures";
import OperationRoute from "./OperationPage";

const get = vi.fn<(id: number) => Promise<Operation>>();
const log = vi.fn<(id: number, offset: number) => Promise<OperationLog>>();

beforeEach(() => {
  get.mockReset();
  log.mockReset();
  FakeWebSocket.install();
});
afterEach(() => vi.unstubAllGlobals());

const chunk = (offset: number, content: string, size: number, finished = false): OperationLog => ({
  id: 3,
  offset,
  next: offset + content.length,
  size,
  content,
  finished,
  available: true,
});

function renderPage() {
  const api = fakeApi({ operations: { get, log } });
  render(
    <MemoryRouter initialEntries={["/servers/local/settings/operations/3"]}>
      <Routes>
        <Route path="/servers/:hostId/settings/operations" element={<p>operations list</p>} />
        <Route path="/servers/:hostId/settings/operations/:id" element={<OperationRoute />} />
      </Routes>
    </MemoryRouter>,
    { wrapper: apiWrapper(api) },
  );
}

it("reads the whole log of a finished operation and shows its result", async () => {
  get.mockResolvedValue(
    operation({
      id: 3,
      title: "Build image reviewer:v3",
      status: "succeeded",
      params: { path: "/srv/reviewer", tag: "v3" },
      finished_at: "2026-10-29T10:01:00Z",
      image_result: { name: "reviewer", tag: "v3", digest: "sha256:abc", layers: 2 },
    }),
  );
  log.mockImplementation(async (_id, offset) =>
    offset === 0 ? chunk(0, "step 1\n", 14, true) : chunk(7, "step 2\n", 14, true),
  );
  renderPage();

  await waitFor(() =>
    expect(screen.getByLabelText("Operation log").textContent).toBe("step 1\nstep 2\n"),
  );
  expect(log.mock.calls.map(([, offset]) => offset)).toEqual([0, 7]);
  expect(screen.getByRole("heading", { name: "Build image reviewer:v3" })).toBeInTheDocument();
  expect(screen.getByText("path=/srv/reviewer")).toBeInTheDocument();
  expect(screen.getByText("sha256:abc")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Download log" })).toBeEnabled();
  expect(screen.getByRole("link", { name: /Operations/ })).toHaveAttribute(
    "href",
    "/servers/local/settings/operations",
  );
  // A finished operation does not watch the socket.
  expect(FakeWebSocket.last("/api/operations/ws")).toBeUndefined();
});

it("appends new log lines on each hint for a running operation", async () => {
  get.mockResolvedValue(operation({ id: 3, status: "running" }));
  log.mockResolvedValueOnce(chunk(0, "start\n", 6));
  renderPage();
  await waitFor(() => expect(screen.getByLabelText("Operation log").textContent).toBe("start\n"));

  const socket = FakeWebSocket.last("/api/operations/ws")!;
  log.mockResolvedValueOnce(chunk(6, "line\n", 11));
  socket.send({ id: 3, status: "running", ts: "t" });
  await waitFor(() =>
    expect(screen.getByLabelText("Operation log").textContent).toBe("start\nline\n"),
  );

  // A hint for another operation is not this page's business.
  socket.send({ id: 99, status: "running", ts: "t" });

  get.mockResolvedValue(
    operation({
      id: 3,
      status: "failed",
      error_code: "build_failed",
      error_message: "missing Tariboyfile",
    }),
  );
  log.mockResolvedValueOnce(chunk(11, "boom\n", 16, true));
  socket.send({ id: 3, status: "failed", ts: "t" });
  await waitFor(() =>
    expect(screen.getByLabelText("Operation log").textContent).toBe("start\nline\nboom\n"),
  );
  expect(screen.getByRole("alert")).toHaveTextContent("build_failed: missing Tariboyfile");
  expect(log.mock.calls.map(([, offset]) => offset)).toEqual([0, 6, 11]);
});

it("says so when the log file is gone", async () => {
  get.mockResolvedValue(operation({ id: 3, status: "interrupted" }));
  log.mockResolvedValue({ ...chunk(0, "", 0, true), available: false });
  renderPage();
  expect(await screen.findByText("Log unavailable")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Download log" })).toBeDisabled();
});
