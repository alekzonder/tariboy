import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useParams } from "react-router-dom";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { OperationList } from "@/lib/api";
import { apiWrapper, fakeApi } from "@/test/api";
import { FakeWebSocket } from "@/test/fakeWebSocket";
import { operation } from "@/test/fixtures/apiFixtures";
import OperationsPage from "./OperationsPage";

const list = vi.fn<(page: number, limit: number) => Promise<OperationList>>();

beforeEach(() => {
  list.mockReset();
  FakeWebSocket.install();
});
afterEach(() => vi.unstubAllGlobals());

function OperationStub() {
  return <p>operation {useParams().id}</p>;
}

function renderPage() {
  const api = fakeApi({ operations: { list } });
  render(
    <MemoryRouter initialEntries={["/servers/local/settings/operations"]}>
      <Routes>
        <Route path="/servers/:hostId/settings/operations" element={<OperationsPage />} />
        <Route path="/servers/:hostId/settings/operations/:id" element={<OperationStub />} />
      </Routes>
    </MemoryRouter>,
    { wrapper: apiWrapper(api) },
  );
}

const page = (operations: OperationList["operations"], total: number, number = 1) => ({
  operations,
  total,
  page: number,
  limit: 50,
});

it("lists operations in the order the daemon returns, newest first, with links", async () => {
  list.mockResolvedValue(
    page(
      [
        operation({
          id: 2,
          kind: "workflow.build",
          title: "Build workflow dev",
          status: "failed",
          created_at: "2026-10-29T10:05:00Z",
          finished_at: "2026-10-29T10:06:30Z",
        }),
        operation({
          id: 1,
          title: "Build image reviewer:latest",
          status: "succeeded",
          created_at: "2026-10-29T10:00:00Z",
          finished_at: "2026-10-29T10:00:42Z",
        }),
      ],
      2,
    ),
  );
  renderPage();

  const rows = (await screen.findAllByRole("row")).slice(1);
  expect(rows).toHaveLength(2);
  expect(rows[0]).toHaveTextContent("Workflow build");
  expect(rows[0]).toHaveTextContent("Build workflow dev");
  expect(rows[0]).toHaveTextContent("failed");
  expect(rows[0]).toHaveTextContent("1m 30s");
  expect(rows[1]).toHaveTextContent("Image build");
  expect(rows[1]).toHaveTextContent("42s");
  expect(list).toHaveBeenCalledWith(1, 50);
  expect(screen.getByText("Page 1 of 1")).toBeInTheDocument();

  fireEvent.click(within(rows[0]).getByRole("link", { name: "Build workflow dev" }));
  expect(await screen.findByText("operation 2")).toBeInTheDocument();
});

it("pages through operations with Previous and Next", async () => {
  list.mockImplementation(async (number) =>
    page(
      [operation({ id: 200 - number, title: `On page ${number}`, status: "succeeded" })],
      120,
      number,
    ),
  );
  renderPage();

  expect(await screen.findByText("On page 1")).toBeInTheDocument();
  expect(screen.getByText("Page 1 of 3")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();

  fireEvent.click(screen.getByRole("button", { name: "Next" }));
  expect(await screen.findByText("On page 2")).toBeInTheDocument();
  expect(screen.getByText("Page 2 of 3")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Next" }));
  expect(await screen.findByText("On page 3")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "Previous" }));
  expect(await screen.findByText("On page 2")).toBeInTheDocument();
  expect(list).toHaveBeenLastCalledWith(2, 50);
});

it("refetches on socket hints while a listed operation runs", async () => {
  list.mockResolvedValueOnce(page([operation({ id: 1, status: "running" })], 1));
  list.mockResolvedValue(page([operation({ id: 1, status: "succeeded" })], 1));
  renderPage();
  expect(await screen.findByText("running")).toBeInTheDocument();

  (await FakeWebSocket.dialed("/api/operations/ws")).send({ id: 1, status: "succeeded", ts: "t" });
  expect(await screen.findByText("succeeded")).toBeInTheDocument();
  const calls = list.mock.calls.length;

  // Nothing listed runs any more: a hint for a listed operation changes nothing.
  (await FakeWebSocket.dialed("/api/operations/ws")).send({ id: 1, status: "succeeded", ts: "t" });
  await waitFor(() => expect(list.mock.calls.length).toBe(calls));
});
