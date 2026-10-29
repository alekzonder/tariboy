import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api";
import { apiWrapper, fakeApi } from "@/test/api";
import { FakeWebSocket } from "@/test/fakeWebSocket";
import { operation, operationStarted } from "@/test/fixtures/apiFixtures";
import WorkflowBuildFromDirectory from "./WorkflowBuildFromDirectory";

const api = { validateWorkflowDirectory: vi.fn(), buildWorkflowDirectory: vi.fn() };
const operations = { get: vi.fn() };
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("sonner", () => ({ toast }));

const remote = { id: "remote", label: "Remote", baseURL: "https://remote", token: "t" };
const remoteApi = fakeApi({ workflows: api, operations }, remote);

beforeEach(() => {
  vi.clearAllMocks();
  FakeWebSocket.install();
  operations.get.mockResolvedValue(operation({ id: 7, status: "running" }));
});
afterEach(() => vi.unstubAllGlobals());

function OperationRoute() {
  return <p>operation {useLocation().pathname}</p>;
}

function setup() {
  const onBuilt = vi.fn();
  const Api = apiWrapper(remoteApi);
  const wrapper = ({ children }: PropsWithChildren) => (
    <Api>
      <MemoryRouter initialEntries={["/servers/remote/workflows"]}>
        <Routes>
          <Route path="/servers/:hostId/workflows" element={children} />
          <Route path="/servers/:hostId/settings/operations/:id" element={<OperationRoute />} />
        </Routes>
      </MemoryRouter>
    </Api>
  );
  render(<WorkflowBuildFromDirectory onBuilt={onBuilt} />, { wrapper });
  fireEvent.change(screen.getByLabelText("Workflow source directory"), {
    target: { value: " /srv/wf " },
  });
  return onBuilt;
}

it("validates on the target and shows what the source declares", async () => {
  api.validateWorkflowDirectory.mockResolvedValue({
    valid: true,
    name: "dev",
    version: "1.0.0",
    pools: ["devs"],
    files: ["statuses/plan.md"],
    errors: [],
  });
  setup();
  fireEvent.click(screen.getByRole("button", { name: "Validate" }));
  expect(await screen.findByText(/dev 1\.0\.0/)).toBeInTheDocument();
  expect(screen.getByText(/devs/)).toBeInTheDocument();
  expect(api.validateWorkflowDirectory).toHaveBeenCalledWith("/srv/wf");
});

it("lists validation errors", async () => {
  api.validateWorkflowDirectory.mockResolvedValue({
    valid: false,
    name: "dev",
    version: "1.0.0",
    pools: [],
    files: [],
    errors: [{ code: "status_missing", path: "statuses", message: "no statuses" }],
  });
  setup();
  fireEvent.click(screen.getByRole("button", { name: "Validate" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "status_missing · statuses: no statuses",
  );
});

it("starts a build, links to its operation, and refreshes the list when it succeeds", async () => {
  api.buildWorkflowDirectory.mockResolvedValueOnce(
    operationStarted({ id: 7, kind: "workflow.build", title: "Build workflow dev" }),
  );
  const onBuilt = setup();
  fireEvent.click(screen.getByRole("button", { name: "Build" }));
  await waitFor(() => expect(toast.success).toHaveBeenCalled());
  expect(api.buildWorkflowDirectory).toHaveBeenCalledWith("/srv/wf");
  const [message, options] = toast.success.mock.calls[0];
  expect(message).toBe("Build started");
  expect(options.description).toBe("Build workflow dev");
  expect(onBuilt).not.toHaveBeenCalled();

  const socket = await FakeWebSocket.dialed("/api/operations/ws");
  socket.open();
  await waitFor(() => expect(operations.get).toHaveBeenCalledWith(7));
  socket.send({ id: 7, status: "running", ts: "t" });
  expect(onBuilt).not.toHaveBeenCalled();
  socket.send({ id: 7, status: "succeeded", ts: "t" });
  expect(onBuilt).toHaveBeenCalledTimes(1);

  expect(options.action.label).toBe("Open operation");
  act(() => options.action.onClick());
  expect(await screen.findByText("operation /servers/remote/settings/operations/7")).toBeTruthy();
});

it("shows the errors of a refused build and a version hint", async () => {
  api.buildWorkflowDirectory.mockRejectedValueOnce(
    new ApiError(400, "workflow_invalid", "invalid", {
      errors: [{ code: "owner_missing", path: "statuses[0].owner", message: "no owner" }],
    }),
  );
  setup();
  fireEvent.click(screen.getByRole("button", { name: "Build" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "owner_missing · statuses[0].owner: no owner",
  );

  api.buildWorkflowDirectory.mockRejectedValueOnce(
    new ApiError(409, "workflow_version_published", "dev 1.0.0 is taken"),
  );
  fireEvent.click(screen.getByRole("button", { name: "Build" }));
  expect(await screen.findByText(/dev 1\.0\.0 is taken/)).toBeInTheDocument();
  expect(screen.getByText(/tariboy workflow version update/)).toBeInTheDocument();
});
