import { beforeEach, describe, expect, it, vi } from "vitest"

const apiOn = vi.hoisted(() => vi.fn())
vi.mock("./api", () => ({
  apiOn,
  resolveTarget: (target: unknown) => target,
}))

import {
  activateQueueWorkflow,
  getTaskWorkflow,
  listAgentPools,
  listWorkflowArtifacts,
  listWorkflowQuestions,
  listWorkflowVersions,
  rebindAgentPool,
  updateTask,
} from "./tasks"

describe("workflow task client", () => {
  beforeEach(() => apiOn.mockReset().mockResolvedValue({}))

  it("uses revisioned queue workflow and explicit pool mutations", async () => {
    await activateQueueWorkflow("DEV", 42, 3, "activate-42")
    expect(apiOn).toHaveBeenCalledWith(undefined, "PUT", "/api/task-queues/DEV/workflow", {
      workflow_version_id: 42,
      revision: 3,
      idempotency_key: "activate-42",
    })

    await rebindAgentPool("DEV", "developers", ["dev-a", "dev-b"], 4, "pool-4")
    expect(apiOn).toHaveBeenCalledWith(undefined, "PATCH", "/api/task-queues/DEV/pools/developers", {
      agents: ["dev-a", "dev-b"], revision: 4, idempotency_key: "pool-4",
    })
  })

  it("exposes typed workflow inspection endpoints", async () => {
    await listWorkflowVersions("development")
    await listAgentPools("DEV")
    await getTaskWorkflow("DEV-1")
    await listWorkflowArtifacts("DEV-1")
    await listWorkflowQuestions("DEV-1")
    expect(apiOn.mock.calls.map((call) => call.slice(1, 3))).toEqual([
      ["GET", "/api/workflows/development/versions"],
      ["GET", "/api/task-queues/DEV/pools"],
      ["GET", "/api/tasks/DEV-1/workflow"],
      ["GET", "/api/tasks/DEV-1/artifacts"],
      ["GET", "/api/tasks/DEV-1/questions"],
    ])
  })

  it("keeps wait-customer, pull request, revision, and explicit target in task updates", async () => {
    const target = { id: "remote", label: "Remote", baseURL: "https://remote.test", token: "secret" }
    await updateTask("TARI-43", {
      status: "wait_customer",
      pull_request: "https://example.test/pull/7",
      revision: 7,
    }, target)
    expect(apiOn).toHaveBeenCalledWith(target, "PATCH", "/api/tasks/TARI-43", {
      status: "wait_customer",
      pull_request: "https://example.test/pull/7",
      revision: 7,
    })
  })
})
