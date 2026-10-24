import type { ReactElement } from "react";
import { fireEvent, render as renderWith, screen, waitFor, within } from "@/test/queryRender";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api";
import QueueSettingsDialog from "./QueueSettingsDialog";
import { agentPool, developmentManifest, remoteTarget } from "@/test/fixtures/workflowFixtures";
import { fakeApi } from "@/test/api";

const api = vi.hoisted(() => ({
  getQueueWorkflow: vi.fn(),
  getWorkflowImage: vi.fn(),
  listWorkflowImages: vi.fn(),
  listQueueSecrets: vi.fn(),
  listQueueEnv: vi.fn(),
  listAgentPools: vi.fn(),
  listTaskPrincipals: vi.fn(),
  listQueueSources: vi.fn(),
  setQueueWorkflow: vi.fn(),
  rebindAgentPool: vi.fn(),
}));
// The mocks are methods of a client bound to the remote host, so each call
// they record went to that host.
const remoteApi = fakeApi(
  {
    tasks: {
      listAgentPools: api.listAgentPools,
      listTaskPrincipals: api.listTaskPrincipals,
      rebindAgentPool: api.rebindAgentPool,
    },
    workflows: {
      getQueueWorkflow: api.getQueueWorkflow,
      getWorkflowImage: api.getWorkflowImage,
      listWorkflowImages: api.listWorkflowImages,
      listQueueSecrets: api.listQueueSecrets,
      listQueueEnv: api.listQueueEnv,
      listQueueSources: api.listQueueSources,
      setQueueWorkflow: api.setQueueWorkflow,
    },
  },
  remoteTarget,
);
const render = (ui: ReactElement) => renderWith(ui, { api: remoteApi });
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const queue = (prefix: string, name: string) => ({
  prefix,
  name,
  description: "",
  owners: [],
  responsible_agent: "",
  revision: 1,
  created_at: "",
  updated_at: "",
});
const queues = [queue("DEV", "Development"), queue("OPS", "Operations")];
const binding = {
  queue: "DEV",
  name: "development",
  version: "0.3.0",
  digest: developmentManifest.digest,
  revision: 2,
  updated_at: "",
};

const renderIt = (props: Partial<Parameters<typeof QueueSettingsDialog>[0]> = {}) => {
  const onCreate = vi.fn().mockResolvedValue(undefined);
  const onUpdate = vi.fn().mockResolvedValue(undefined);
  render(
    <MemoryRouter>
      <QueueSettingsDialog
        open
        onOpenChange={vi.fn()}
        queues={queues}
        onCreate={onCreate}
        onUpdate={onUpdate}
        {...props}
      />
    </MemoryRouter>,
  );
  return { onCreate, onUpdate };
};

beforeEach(() => {
  vi.resetAllMocks();
  api.getQueueWorkflow.mockImplementation(async (prefix: string) =>
    prefix === "DEV" ? binding : null,
  );
  api.getWorkflowImage.mockResolvedValue(developmentManifest);
  api.listWorkflowImages.mockResolvedValue([
    {
      name: "development",
      tag: "latest",
      version: "0.3.0",
      digest: developmentManifest.digest,
      built_at: "",
    },
  ]);
  api.listQueueSecrets.mockResolvedValue({ secrets: [], global_env: [] });
  api.listQueueEnv.mockResolvedValue([]);
  api.listQueueSources.mockResolvedValue([]);
  api.listAgentPools.mockImplementation(async (prefix: string) => ({
    items: prefix === "DEV" ? [agentPool("developers", ["dev-a"])] : [],
  }));
  api.listTaskPrincipals.mockResolvedValue({
    customer: "user:owner",
    agents: ["dev-a", "rev-a"],
    groups: [],
  });
});

it("lists each queue with its workflow and warns about a required pool without agents", async () => {
  renderIt();
  const dev = await screen.findByRole("button", { name: /^Queue DEV/ });
  await waitFor(() => expect(dev).toHaveTextContent("development@0.3.0"));
  expect(within(dev).getByLabelText("A pool has no agents")).toBeInTheDocument();
  const ops = screen.getByRole("button", { name: /^Queue OPS/ });
  await waitFor(() => expect(ops).toHaveTextContent("flexible"));
  expect(within(ops).queryByLabelText("A pool has no agents")).not.toBeInTheDocument();
  expect(api.getQueueWorkflow).toHaveBeenCalledWith("OPS");
  expect(api.getWorkflowImage).toHaveBeenCalledWith("development", developmentManifest.digest);
});

it("shows Env and Secrets blocks in the Environment tab", async () => {
  renderIt();
  expect(screen.queryByRole("tab", { name: "Secrets" })).not.toBeInTheDocument();
  await userEvent.click(await screen.findByRole("tab", { name: "Environment" }));
  expect(await screen.findByRole("region", { name: "Env DEV" })).toBeInTheDocument();
  expect(screen.getByRole("region", { name: "Secrets DEV" })).toBeInTheDocument();
  expect(api.listQueueEnv).toHaveBeenCalledWith("DEV");
});

it("saves General with the queue revision", async () => {
  const { onUpdate } = renderIt();
  await userEvent.click(await screen.findByRole("button", { name: /^Queue OPS/ }));
  fireEvent.change(screen.getByLabelText("Queue name OPS"), { target: { value: "Ops renamed" } });
  fireEvent.click(screen.getByRole("button", { name: "Save OPS" }));
  expect(onUpdate).toHaveBeenCalledWith(
    "OPS",
    expect.objectContaining({ name: "Ops renamed", revision: 1 }),
  );
});

it("creates a queue from the New queue form", async () => {
  const { onCreate } = renderIt();
  fireEvent.click(screen.getByRole("button", { name: "New queue" }));
  fireEvent.change(screen.getByLabelText("Queue prefix"), { target: { value: "qa" } });
  fireEvent.change(screen.getByLabelText("New queue name"), { target: { value: "Quality" } });
  fireEvent.click(screen.getByRole("button", { name: "Add queue" }));
  await waitFor(() =>
    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({ prefix: "QA", name: "Quality" }),
    ),
  );
});

it("opens Pools from an empty pool chip in the workflow statuses, then clears the warning once it has agents", async () => {
  api.rebindAgentPool.mockResolvedValue(agentPool("reviewers", ["rev-a"]));
  renderIt();
  await userEvent.click(await screen.findByRole("tab", { name: /Workflow/ }));
  await userEvent.click(await screen.findByRole("button", { name: "Pool reviewers, no agents" }));
  const reviewers = await screen.findByRole("group", { name: "Pool reviewers" });
  expect(screen.getByRole("tab", { name: /Pools/ })).toHaveAttribute("aria-selected", "true");
  fireEvent.change(within(reviewers).getByLabelText("Add agent to reviewers"), {
    target: { value: "rev-a" },
  });
  fireEvent.click(within(reviewers).getByRole("button", { name: "Add" }));
  fireEvent.click(within(reviewers).getByRole("button", { name: "Save pool reviewers" }));
  await waitFor(() =>
    expect(
      within(screen.getByRole("button", { name: /^Queue DEV/ })).queryByLabelText(
        "A pool has no agents",
      ),
    ).not.toBeInTheDocument(),
  );
});

it("clears Pools needed after a refused bind once the pool gains agents", async () => {
  api.setQueueWorkflow.mockRejectedValue(
    new ApiError(409, "workflow_pool_empty", "workflow pools are missing or have no agents: dev", {
      pools: ["dev"],
    }),
  );
  api.rebindAgentPool.mockResolvedValue(agentPool("dev", ["dev-a"], 1, "OPS"));
  renderIt();
  await userEvent.click(await screen.findByRole("button", { name: /^Queue OPS/ }));
  await userEvent.click(screen.getByRole("tab", { name: /Workflow/ }));
  await screen.findByRole("option", { name: "development:latest" });
  fireEvent.change(screen.getByLabelText("Workflow image"), {
    target: { value: "development:latest" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Bind" }));
  const needed = await screen.findByRole("alert", { name: "Pools needed" });
  await userEvent.click(within(needed).getByRole("button", { name: "Open Pools" }));
  // The refused bind named the pool, so its row is waiting.
  const dev = await screen.findByRole("group", { name: "Pool dev" });
  expect(dev).toHaveTextContent("needed to bind the workflow");
  fireEvent.change(within(dev).getByLabelText("Add agent to dev"), { target: { value: "dev-a" } });
  fireEvent.click(within(dev).getByRole("button", { name: "Add" }));
  fireEvent.click(within(dev).getByRole("button", { name: "Save pool dev" }));
  await waitFor(() =>
    expect(api.rebindAgentPool).toHaveBeenCalledWith(
      "OPS",
      "dev",
      ["dev-a"],
      0,
      expect.any(String),
    ),
  );
  await userEvent.click(screen.getByRole("tab", { name: /Workflow/ }));
  await screen.findByLabelText("Workflow image");
  expect(screen.queryByRole("alert", { name: "Pools needed" })).not.toBeInTheDocument();
});

it("shows Sources only for a bound queue", async () => {
  renderIt();
  await screen.findByRole("tab", { name: "General" });
  expect(await screen.findByRole("tab", { name: "Sources" })).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: /^Queue OPS/ }));
  await waitFor(() =>
    expect(screen.queryByRole("tab", { name: "Sources" })).not.toBeInTheDocument(),
  );
});

it("offers a retry when a queue fails to load", async () => {
  api.listAgentPools.mockRejectedValueOnce(new ApiError(403, "forbidden", "permission denied"));
  renderIt();
  const alert = await screen.findByRole("alert", { name: "Queue DEV error" });
  expect(alert).toHaveTextContent("permission denied");
  fireEvent.click(within(alert).getByRole("button", { name: "Retry" }));
  await waitFor(() =>
    expect(screen.queryByRole("alert", { name: "Queue DEV error" })).not.toBeInTheDocument(),
  );
});
