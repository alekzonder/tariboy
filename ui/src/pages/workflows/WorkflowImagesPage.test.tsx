import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, expect, it, vi } from "vitest";
import { ApiProvider } from "@/components/ApiProvider";
import type { Daemon } from "@/lib/daemons";
import { fakeApi } from "@/test/api";
import WorkflowImagesPage from "./WorkflowImagesPage";

const api = { listWorkflowImages: vi.fn(), listTaskQueues: vi.fn(), getQueueWorkflow: vi.fn() };
vi.mock("@/components/daemonContext", () => ({ useOptionalDaemons: () => ({ daemons: [] }) }));

// A client for `target` whose registry calls go to `mocks`.
const clientFor = (target: Daemon, mocks: typeof api) =>
  fakeApi(
    {
      workflows: {
        listWorkflowImages: mocks.listWorkflowImages,
        getQueueWorkflow: mocks.getQueueWorkflow,
      },
      tasks: { listTaskQueues: mocks.listTaskQueues },
    },
    target,
  );

const remote = { id: "remote", label: "Remote", baseURL: "https://remote", token: "t" };
const remoteApi = clientFor(remote, api);
const digestA = "a".repeat(64);
const digestB = "b".repeat(64);

beforeEach(() => {
  vi.clearAllMocks();
  api.listWorkflowImages.mockResolvedValue([
    {
      name: "development",
      tag: "0.9.0",
      version: "0.9.0",
      digest: digestB,
      built_at: "2026-09-01T00:00:00Z",
    },
    {
      name: "development",
      tag: "latest",
      version: "0.10.0",
      digest: digestA,
      built_at: "2026-10-01T00:00:00Z",
    },
    {
      name: "development",
      tag: "0.10.0",
      version: "0.10.0",
      digest: digestA,
      built_at: "2026-10-01T00:00:00Z",
    },
    {
      name: "research",
      tag: "0.1.0",
      version: "0.1.0",
      digest: digestB,
      built_at: "2026-09-02T00:00:00Z",
    },
  ]);
  api.listTaskQueues.mockResolvedValue({
    queues: [{ prefix: "DEV" }, { prefix: "BAD" }],
    count: 2,
  });
  api.getQueueWorkflow.mockImplementation(async (queue: string) => {
    if (queue === "BAD") throw new Error("boom");
    return { queue, digest: digestA };
  });
});

const renderPage = () =>
  render(
    <MemoryRouter>
      <ApiProvider client={remoteApi}>
        <WorkflowImagesPage basePath="/library/workflows" />
      </ApiProvider>
    </MemoryRouter>,
  );

it("lists one row per name with its newest version and every bound queue", async () => {
  renderPage();
  const table = await screen.findByRole("table");
  // development has digests A (bound to DEV) and B; research only B.
  await waitFor(() => expect(within(table).getByText("DEV, unknown")).toBeInTheDocument());
  expect(
    within(table)
      .getAllByRole("columnheader")
      .map((cell) => cell.textContent),
  ).toEqual(["Name", "Version", "Bound to"]);
  const rows = within(table).getAllByRole("row").slice(1);
  expect(rows.map((row) => within(row).getAllByRole("cell")[0].textContent)).toEqual([
    "development",
    "research",
  ]);
  expect(rows[0]).toHaveTextContent("0.10.0");
  expect(within(rows[0]).getByRole("link", { name: "development" })).toHaveAttribute(
    "href",
    "/library/workflows/development/latest",
  );
  expect(within(rows[0]).getByText("DEV, unknown")).toBeInTheDocument();
  expect(within(rows[1]).getByText("unknown")).toHaveAttribute(
    "title",
    "Could not read the binding of BAD",
  );
  expect(api.listWorkflowImages).toHaveBeenCalledWith();
  expect(api.listTaskQueues).toHaveBeenCalledWith();
  expect(screen.queryByRole("button", { name: /Upload to servers/ })).toBeNull();
  expect(screen.queryByLabelText("Workflow source directory")).toBeNull();
});

it("opens the build form in a dialog from Add workflow", async () => {
  renderPage();
  await screen.findByRole("table");
  fireEvent.click(screen.getByRole("button", { name: "Add workflow" }));
  expect(
    within(await screen.findByRole("dialog")).getByLabelText("Workflow source directory"),
  ).toBeInTheDocument();
});

it("shows an empty registry", async () => {
  api.listWorkflowImages.mockResolvedValue([]);
  renderPage();
  expect(await screen.findByText("No workflows on this host.")).toBeInTheDocument();
});

it("shows a list failure with Retry", async () => {
  api.listWorkflowImages.mockRejectedValueOnce(new Error("daemon down"));
  renderPage();
  expect(await screen.findByRole("alert")).toHaveTextContent("daemon down");
  fireEvent.click(screen.getByRole("button", { name: "Retry" }));
  expect(await screen.findByRole("table")).toBeInTheDocument();
});

it("keeps the table and skips loads while the host reconnects", async () => {
  const view = renderPage();
  await screen.findByRole("table");
  const offline = { ...remote, baseURL: "" };
  const offlineCalls = {
    listWorkflowImages: vi.fn(),
    listTaskQueues: vi.fn(),
    getQueueWorkflow: vi.fn(),
  };
  view.rerender(
    <MemoryRouter>
      <ApiProvider client={clientFor(offline, offlineCalls)}>
        <WorkflowImagesPage basePath="/library/workflows" />
      </ApiProvider>
    </MemoryRouter>,
  );
  await waitFor(() => expect(screen.getByRole("table")).toBeInTheDocument());
  expect(screen.queryByRole("alert")).toBeNull();
  expect(offlineCalls.listWorkflowImages).not.toHaveBeenCalled();
  expect(offlineCalls.listTaskQueues).not.toHaveBeenCalled();
});
