import type { ReactElement } from "react";
import { fireEvent, render as renderWith, screen, waitFor, within } from "@/test/queryRender";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, expect, it, vi } from "vitest";
import { ApiError, type AgentPool } from "@/lib/api";
import QueueWorkflowSettings from "./QueueWorkflowSettings";
import { agentPool, developmentManifest, remoteTarget } from "@/test/fixtures/workflowFixtures";
import { fakeApi } from "@/test/api";

const api = vi.hoisted(() => ({
  getQueueWorkflow: vi.fn(),
  setQueueWorkflow: vi.fn(),
  clearQueueWorkflow: vi.fn(),
  listWorkflowImages: vi.fn(),
  getWorkflowImage: vi.fn(),
}));
// The mocks are methods of a client bound to the remote host, so each call
// they record went to that host.
const remoteApi = fakeApi(
  {
    workflows: {
      getQueueWorkflow: api.getQueueWorkflow,
      setQueueWorkflow: api.setQueueWorkflow,
      clearQueueWorkflow: api.clearQueueWorkflow,
      listWorkflowImages: api.listWorkflowImages,
      getWorkflowImage: api.getWorkflowImage,
    },
  },
  remoteTarget,
);
const render = (ui: ReactElement) => renderWith(ui, { api: remoteApi });

const digest = "0123456789abcdef0123456789abcdef";
const binding = {
  queue: "REL",
  name: "release",
  version: "1.2.0",
  digest,
  tag: "",
  revision: 3,
  updated_at: "2026-10-02T10:00:00Z",
};
const images = [
  { name: "release", tag: "1.2.0", version: "1.2.0", digest, built_at: "" },
  { name: "release", tag: "latest", version: "1.2.0", digest, built_at: "" },
  { name: "audit", tag: "2", version: "2", digest: "ff", built_at: "" },
];
const handlers = {
  onOpenTab: vi.fn(),
  onPoolsMissing: vi.fn(),
  onSecretsMissing: vi.fn(),
  onChanged: vi.fn(),
};
const view = (pools: AgentPool[] = []) => (
  <MemoryRouter initialEntries={["/servers/h1/tasks"]}>
    <Routes>
      <Route
        path="/servers/:hostId/tasks"
        element={<QueueWorkflowSettings queue="REL" pools={pools} {...handlers} />}
      />
    </Routes>
  </MemoryRouter>
);
const renderIt = (pools: AgentPool[] = []) => render(view(pools));

beforeEach(() => {
  vi.resetAllMocks();
  api.listWorkflowImages.mockResolvedValue(images);
  api.getWorkflowImage.mockResolvedValue(developmentManifest);
});

it("renders No workflow when unbound and lists images sorted with latest first", async () => {
  api.getQueueWorkflow.mockResolvedValue(null);
  renderIt();
  expect(await screen.findByText("No workflow")).toBeInTheDocument();
  await screen.findByRole("option", { name: "audit:2" });
  const options = screen.getAllByRole("option").map((option) => option.textContent);
  expect(options).toEqual(["Choose an image", "audit:2", "release:latest", "release:1.2.0"]);
  expect(screen.getByRole("button", { name: "Clear" })).toBeDisabled();
  expect(api.getQueueWorkflow).toHaveBeenCalledWith("REL");
});

it("renders a binding with a short digest and the full digest in a title", async () => {
  api.getQueueWorkflow.mockResolvedValue(binding);
  renderIt();
  const short = await screen.findByText(digest.slice(0, 12));
  expect(short).toHaveAttribute("title", digest);
  expect(screen.getByTestId("binding")).toHaveTextContent("release 1.2.0");
  expect(screen.getByTestId("binding")).not.toHaveTextContent("follows");
});

it("says which tag a binding made by a tag follows", async () => {
  api.getQueueWorkflow.mockResolvedValue({ ...binding, tag: "latest" });
  renderIt();
  expect(await screen.findByTestId("binding")).toHaveTextContent(
    `release 1.2.0 ${digest.slice(0, 12)} follows latest`,
  );
});

it("binds NAME:TAG with revision 0 when unbound", async () => {
  api.getQueueWorkflow.mockResolvedValue(null);
  api.setQueueWorkflow.mockResolvedValue(binding);
  renderIt();
  await screen.findByText("No workflow");
  await screen.findByRole("option", { name: "audit:2" });
  fireEvent.change(screen.getByLabelText("Workflow image"), {
    target: { value: "release:latest" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Bind" }));
  await waitFor(() =>
    expect(api.setQueueWorkflow).toHaveBeenCalledWith("REL", "release:latest", 0),
  );
  expect(await screen.findByText(digest.slice(0, 12))).toBeInTheDocument();
  expect(handlers.onChanged).toHaveBeenCalled();
});

it("sends the current revision and reloads on a revision conflict", async () => {
  api.getQueueWorkflow
    .mockResolvedValueOnce(binding)
    .mockResolvedValue({ ...binding, revision: 4, name: "audit" });
  api.setQueueWorkflow.mockRejectedValue(
    new ApiError(409, "revision_conflict", "changed", { current_revision: 4 }),
  );
  renderIt();
  await screen.findByText(digest.slice(0, 12));
  await screen.findByRole("option", { name: "audit:2" });
  fireEvent.change(screen.getByLabelText("Workflow image"), { target: { value: "audit:2" } });
  fireEvent.click(screen.getByRole("button", { name: "Bind" }));
  await waitFor(() => expect(api.setQueueWorkflow).toHaveBeenCalledWith("REL", "audit:2", 3));
  expect(await screen.findByText(/changed elsewhere/i)).toBeInTheDocument();
  expect(api.getQueueWorkflow).toHaveBeenCalledTimes(2);
  expect(screen.getByTestId("binding")).toHaveTextContent("audit");
});

it("lists empty pools with a way to the Pools tab", async () => {
  api.getQueueWorkflow.mockResolvedValue(null);
  api.setQueueWorkflow.mockRejectedValue(
    new ApiError(
      409,
      "workflow_pool_empty",
      "workflow pools are missing or have no agents: dev, qa",
      { pools: ["dev", "qa"] },
    ),
  );
  renderIt();
  await screen.findByRole("option", { name: "audit:2" });
  fireEvent.change(screen.getByLabelText("Workflow image"), { target: { value: "audit:2" } });
  fireEvent.click(screen.getByRole("button", { name: "Bind" }));
  const alert = await screen.findByRole("alert", { name: "Pools needed" });
  expect(alert).toHaveTextContent("dev");
  expect(alert).toHaveTextContent("qa");
  expect(alert).toHaveTextContent("workflow pools are missing or have no agents");
  expect(handlers.onPoolsMissing).toHaveBeenCalledWith(["dev", "qa"]);
  fireEvent.click(within(alert).getByRole("button", { name: "Open Pools" }));
  expect(handlers.onOpenTab).toHaveBeenCalledWith("pools");
});

it("names only the listed pools that still have no agents", async () => {
  api.getQueueWorkflow.mockResolvedValue(null);
  api.setQueueWorkflow.mockRejectedValue(
    new ApiError(
      409,
      "workflow_pool_empty",
      "workflow pools are missing or have no agents: dev, qa",
      { pools: ["dev", "qa"] },
    ),
  );
  const { rerender } = renderIt();
  await screen.findByRole("option", { name: "audit:2" });
  fireEvent.change(screen.getByLabelText("Workflow image"), { target: { value: "audit:2" } });
  fireEvent.click(screen.getByRole("button", { name: "Bind" }));
  await screen.findByRole("alert", { name: "Pools needed" });
  rerender(view([agentPool("dev", ["dev-a"])]));
  // The daemon's message stays; the list names only what is still empty.
  expect(
    screen.getByRole("alert", { name: "Pools needed" }).querySelector(".font-mono"),
  ).toHaveTextContent(/^qa$/);
  rerender(view([agentPool("dev", ["dev-a"]), agentPool("qa", [])]));
  expect(screen.getByRole("alert", { name: "Pools needed" })).toBeInTheDocument();
  rerender(view([agentPool("dev", ["dev-a"]), agentPool("qa", ["qa-a"])]));
  expect(screen.queryByRole("alert", { name: "Pools needed" })).not.toBeInTheDocument();
});
it("lists missing secrets with a way to the Environment tab", async () => {
  api.getQueueWorkflow.mockResolvedValue(null);
  api.setQueueWorkflow.mockRejectedValue(
    new ApiError(409, "workflow_secret_missing", "workflow secrets have no value: TOKEN", {
      secrets: ["TOKEN"],
    }),
  );
  renderIt();
  await screen.findByRole("option", { name: "audit:2" });
  fireEvent.change(screen.getByLabelText("Workflow image"), { target: { value: "audit:2" } });
  fireEvent.click(screen.getByRole("button", { name: "Bind" }));
  const alert = await screen.findByRole("alert", { name: "Secrets needed" });
  expect(alert).toHaveTextContent("TOKEN");
  expect(handlers.onSecretsMissing).toHaveBeenCalledWith({
    names: ["TOKEN"],
    message: "workflow secrets have no value: TOKEN",
  });
  fireEvent.click(within(alert).getByRole("button", { name: "Open Environment" }));
  expect(handlers.onOpenTab).toHaveBeenCalledWith("environment");
});

it("asks before clearing and does nothing when declined", async () => {
  api.getQueueWorkflow.mockResolvedValue(binding);
  api.clearQueueWorkflow.mockResolvedValue(undefined);
  renderIt();
  await screen.findByText(digest.slice(0, 12));
  fireEvent.click(screen.getByRole("button", { name: "Clear" }));
  fireEvent.click(await screen.findByRole("button", { name: "Go back" }));
  expect(api.clearQueueWorkflow).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Clear" }));
  fireEvent.click(await screen.findByRole("button", { name: "Clear binding" }));
  await waitFor(() => expect(api.clearQueueWorkflow).toHaveBeenCalledWith("REL", 3));
  expect(await screen.findByText("No workflow")).toBeInTheDocument();
});

it("disables Bind and Clear while a clear is running and enables them afterwards", async () => {
  api.getQueueWorkflow.mockResolvedValue(binding);
  let finish: () => void = () => {};
  api.clearQueueWorkflow.mockReturnValue(
    new Promise<void>((resolve) => {
      finish = resolve;
    }),
  );
  renderIt();
  await screen.findByText(digest.slice(0, 12));
  await screen.findByRole("option", { name: "audit:2" });
  fireEvent.change(screen.getByLabelText("Workflow image"), { target: { value: "audit:2" } });
  fireEvent.click(screen.getByRole("button", { name: "Clear" }));
  fireEvent.click(await screen.findByRole("button", { name: "Clear binding" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "Bind" })).toBeDisabled());
  expect(screen.getByRole("button", { name: "Clear" })).toBeDisabled();
  finish();
  await waitFor(() => expect(screen.getByRole("button", { name: "Bind" })).toBeEnabled());
});

it("reloads and shows the notice when a clear hits a revision conflict", async () => {
  api.getQueueWorkflow
    .mockResolvedValueOnce(binding)
    .mockResolvedValue({ ...binding, revision: 4 });
  api.clearQueueWorkflow.mockRejectedValue(
    new ApiError(409, "revision_conflict", "changed", { current_revision: 4 }),
  );
  renderIt();
  await screen.findByText(digest.slice(0, 12));
  fireEvent.click(screen.getByRole("button", { name: "Clear" }));
  fireEvent.click(await screen.findByRole("button", { name: "Clear binding" }));
  expect(await screen.findByText(/changed elsewhere/i)).toBeInTheDocument();
  expect(api.getQueueWorkflow).toHaveBeenCalledTimes(2);
  expect(screen.getByTestId("binding")).toBeInTheDocument();
});

it("does not claim No workflow when the binding failed to load, and retries", async () => {
  api.getQueueWorkflow
    .mockRejectedValueOnce(new ApiError(500, "internal", "boom"))
    .mockResolvedValue(binding);
  renderIt();
  expect(await screen.findByRole("alert")).toHaveTextContent("boom");
  expect(screen.queryByText("No workflow")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Bind" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Clear" })).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "Retry" }));
  expect(await screen.findByText(digest.slice(0, 12))).toBeInTheDocument();
  expect(screen.queryByText("boom")).not.toBeInTheDocument();
});

it("links the bound image to its detail page by its version tag, else by digest", async () => {
  api.getQueueWorkflow.mockResolvedValue(binding);
  const view = renderIt();
  await waitFor(() =>
    expect(screen.getByRole("link", { name: "release" })).toHaveAttribute(
      "href",
      "/servers/h1/workflows/release/1.2.0",
    ),
  );
  view.unmount();

  api.listWorkflowImages.mockResolvedValue([
    { name: "release", tag: "latest", version: "1.2.0", digest: "other", built_at: "" },
  ]);
  renderIt();
  await waitFor(() =>
    expect(screen.getByRole("link", { name: "release" })).toHaveAttribute(
      "href",
      `/servers/h1/workflows/release/${digest}`,
    ),
  );
});

it("shows the statuses of the bound workflow with its pools", async () => {
  api.getQueueWorkflow.mockResolvedValue(binding);
  renderIt([agentPool("developers", ["dev-a"])]);
  expect(await screen.findByRole("table", { name: "Statuses" })).toBeInTheDocument();
  expect(api.getWorkflowImage).toHaveBeenCalledWith("release", digest);
  expect(screen.getByRole("button", { name: "Pool developers, 1 agent" })).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Pool reviewers, no agents" }));
  expect(handlers.onOpenTab).toHaveBeenCalledWith("pools");
});

it("previews the statuses of a chosen image before it is bound", async () => {
  api.getQueueWorkflow.mockResolvedValue(null);
  renderIt();
  await screen.findByRole("option", { name: "audit:2" });
  expect(screen.queryByRole("table", { name: "Statuses" })).not.toBeInTheDocument();
  fireEvent.change(screen.getByLabelText("Workflow image"), { target: { value: "audit:2" } });
  expect(await screen.findByRole("table", { name: "Statuses" })).toBeInTheDocument();
  expect(api.getWorkflowImage).toHaveBeenCalledWith("audit", "2");
});
