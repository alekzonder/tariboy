import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import type { WorkflowImageDefinition, WorkflowImageListing } from "@/lib/api";
import type { Daemon } from "@/lib/daemons";
import { apiWrapper, fakeApi } from "@/test/api";
import {
  boundTo,
  effectiveLimits,
  groupWorkflowImages,
  ownerLabel,
  useQueueBindings,
} from "./workflowImages";

const api = { listTaskQueues: vi.fn(), getQueueWorkflow: vi.fn() };
// The hook reads the host of the nearest ApiProvider.
const on = (target: Daemon | null) => ({
  wrapper: apiWrapper(
    fakeApi(
      {
        tasks: { listTaskQueues: api.listTaskQueues },
        workflows: { getQueueWorkflow: api.getQueueWorkflow },
      },
      target,
    ),
  ),
});

const remote = { id: "remote", label: "Remote", baseURL: "https://remote", token: "t" };
const image = (name: string, tag: string, digest = "d"): WorkflowImageListing => ({
  name,
  tag,
  version: tag,
  digest,
  built_at: "2026-10-01T00:00:00Z",
});

beforeEach(() => {
  api.listTaskQueues.mockReset();
  api.getQueueWorkflow.mockReset();
});

it("groups by name and puts latest first, then versions newest first", () => {
  const groups = groupWorkflowImages([
    image("research", "0.1.0"),
    image("development", "1.9.0"),
    image("development", "latest"),
    image("development", "1.10.0"),
    image("development", "0.2.0"),
  ]);
  expect(groups.map((group) => [group.name, group.tags.map((tag) => tag.tag)])).toEqual([
    ["development", ["latest", "1.10.0", "1.9.0", "0.2.0"]],
    ["research", ["0.1.0"]],
  ]);
});

it("collects the queues bound to each digest and marks the unreadable ones", async () => {
  api.listTaskQueues.mockResolvedValue({
    queues: [{ prefix: "DEV" }, { prefix: "OPS" }, { prefix: "RES" }, { prefix: "BAD" }],
    count: 4,
  });
  api.getQueueWorkflow.mockImplementation(async (queue: string) => {
    if (queue === "BAD") throw new Error("boom");
    if (queue === "RES") return null;
    return { queue, digest: "aaa" };
  });
  const { result } = renderHook(() => useQueueBindings(), on(remote));
  await waitFor(() => expect(result.current.loaded).toBe(true));
  expect(api.listTaskQueues).toHaveBeenCalledWith();
  for (const queue of ["DEV", "OPS", "RES", "BAD"])
    expect(api.getQueueWorkflow).toHaveBeenCalledWith(queue);
  expect(boundTo(result.current, "aaa")).toEqual({
    text: "DEV, OPS, unknown",
    title: "Could not read the binding of BAD",
  });
  expect(boundTo(result.current, "bbb")).toEqual({
    text: "unknown",
    title: "Could not read the binding of BAD",
  });
});

it("joins the queues bound to any digest of a name once each", () => {
  const bindings = {
    loaded: true,
    byDigest: new Map([
      ["aaa", ["DEV", "OPS"]],
      ["bbb", ["DEV", "RES"]],
    ]),
    unknown: [],
    failed: "",
  };
  expect(boundTo(bindings, ["aaa", "bbb", "aaa", "ccc"])).toEqual({
    text: "DEV, OPS, RES",
    title: undefined,
  });
});

it("shows unknown everywhere when the queue list fails", async () => {
  api.listTaskQueues.mockRejectedValue(new Error("offline"));
  const { result } = renderHook(() => useQueueBindings(), on(null));
  await waitFor(() => expect(result.current.loaded).toBe(true));
  expect(boundTo(result.current, "aaa")).toEqual({
    text: "unknown",
    title: "Could not list queues: offline",
  });
});

it("shows a dash for an image no queue binds", async () => {
  api.listTaskQueues.mockResolvedValue({ queues: [], count: 0 });
  const { result } = renderHook(() => useQueueBindings(), on(null));
  await waitFor(() => expect(result.current.loaded).toBe(true));
  expect(boundTo(result.current, "aaa")).toEqual({ text: "—", title: undefined });
});

it("labels owners", () => {
  expect(ownerLabel({ owner: { kind: "pool", pool: "devs" } })).toBe("pool:devs");
  expect(ownerLabel({ owner: { kind: "customer" } })).toBe("customer");
  expect(ownerLabel({ owner: { kind: "script" } })).toBe("script");
  expect(ownerLabel({ owner: { kind: "" }, terminal: true })).toBe("terminal");
});

it("resolves limits status over workflow over default", () => {
  const definition = {
    limits: { idle_iterations: 4 },
    statuses: [
      {
        id: "plan",
        owner: { kind: "pool", pool: "devs" },
        limits: { rejected_requests: 9, unavailable_grace: "0" },
      },
      { id: "done", owner: { kind: "" }, terminal: true },
    ],
  } as unknown as WorkflowImageDefinition;
  expect(effectiveLimits(definition)).toEqual([
    {
      scope: "workflow",
      values: [
        { key: "idle_iterations", value: "4", source: "workflow" },
        { key: "rejected_requests", value: "5", source: "default" },
        { key: "script_failures", value: "3", source: "default" },
        { key: "unavailable_grace", value: "5m", source: "default" },
      ],
    },
    {
      scope: "plan",
      values: [
        { key: "idle_iterations", value: "4", source: "workflow" },
        { key: "rejected_requests", value: "9", source: "status" },
        { key: "script_failures", value: "3", source: "default" },
        { key: "unavailable_grace", value: "0", source: "status" },
      ],
    },
  ]);
});
