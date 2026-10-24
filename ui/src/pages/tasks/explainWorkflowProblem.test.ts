import { describe, expect, it } from "vitest";
import type { WorkflowScriptRun, TransitionRequest, WorkflowView } from "@/lib/api";
import { customerView, poolView } from "@/test/fixtures/workflowFixtures";
import { explainProblem } from "./explainWorkflowProblem";

const request = (
  state: TransitionRequest["state"],
  extra: Partial<TransitionRequest> = {},
): TransitionRequest => ({
  id: 12,
  task_key: "REL-1",
  outcome: "drafted",
  actor: "agent:writer",
  state,
  created_at: "2026-10-01T10:05:00Z",
  ...extra,
});

const failedRun: WorkflowScriptRun = {
  id: 8,
  task_key: "REL-1",
  kind: "check",
  script: "./checks/notes.sh",
  run_as: "queue",
  state: "finished",
  verdict: "failure",
  exit_code: 3,
  message: "the script failed with exit 3",
  request_id: 12,
  created_at: "2026-10-01T10:05:00Z",
  started_at: "2026-10-01T10:05:01Z",
  finished_at: "2026-10-01T10:05:02Z",
  log_path: "/l",
};

const withCounters = (
  view: WorkflowView,
  counters: Partial<WorkflowView["visits"][number]>,
): WorkflowView => ({
  ...view,
  visits: view.visits.map((visit) => (visit.left_at ? visit : { ...visit, ...counters })),
  statuses: view.statuses.map((status) =>
    status.id === view.status
      ? {
          ...status,
          limits: {
            idle_iterations: 3,
            rejected_requests: 5,
            script_failures: 3,
            unavailable_grace: "5m0s",
          },
        }
      : status,
  ),
});

describe("explainProblem", () => {
  it("explains a failed request with its run, the counter and a retry-minded advice", () => {
    const view = withCounters(
      {
        ...poolView,
        runs: [failedRun],
        last_request: request("failed", {
          result_message: "the script failed with exit 3\nlog: /l",
        }),
      },
      { script_failures: 1 },
    );
    const problem = explainProblem(view)!;
    expect(problem.kind).toBe("failed");
    expect(problem.tone).toBe("danger");
    expect(problem.title).toBe("A check could not run: drafted");
    expect(problem.detail).toBe("the script failed with exit 3");
    expect(problem.run?.id).toBe(8);
    expect(problem.request?.id).toBe(12);
    expect(problem.counter).toBe("1 of 3 script failures");
    expect(problem.advice).toMatch(/Retry/);
  });

  it("names an interrupted check and a timed out one", () => {
    const interrupted = {
      ...poolView,
      runs: [{ ...failedRun, state: "interrupted" as const, verdict: undefined }],
      last_request: request("failed", { result_message: "the daemon restarted during check" }),
    };
    expect(explainProblem(interrupted)?.detail).toBe("The daemon restarted during the check.");
    const timedOut: WorkflowView = {
      ...poolView,
      runs: [failedRun],
      outcomes: [
        {
          on: "drafted",
          to: "review",
          checks: ["./checks/notes.sh"],
          check_details: [{ script: "./checks/notes.sh", run_as: "queue", timeout: "30s" }],
        },
      ],
      last_request: request("failed", { result_message: "the script timed out\nlog: /l" }),
    };
    expect(explainProblem(timedOut)?.detail).toBe(
      "the script timed out (the check's timeout is 30s)",
    );
  });

  it("explains a rejection by who should fix it", () => {
    const rejected = request("rejected", { result_message: "the notes must start with a heading" });
    const inPool = explainProblem(
      withCounters({ ...poolView, last_request: rejected }, { rejected_requests: 2 }),
    )!;
    expect(inPool.kind).toBe("rejected");
    expect(inPool.tone).toBe("attention");
    expect(inPool.detail).toBe("the notes must start with a heading");
    expect(inPool.counter).toBe("2 of 5 rejections");
    expect(inPool.advice).toMatch(/agent sees this in its goal/);
    const forCustomer = explainProblem({ ...customerView, last_request: rejected })!;
    expect(forCustomer.advice).toMatch(/send the task back/);
  });

  it("ignores a request from an earlier visit and says nothing on a closed or paused task", () => {
    const old = request("rejected", { created_at: "2026-10-01T09:00:00Z", result_message: "old" });
    expect(explainProblem({ ...poolView, last_request: old })).toBeNull();
    expect(
      explainProblem({ ...poolView, category: "done", last_request: request("failed") }),
    ).toBeNull();
    expect(
      explainProblem({
        ...poolView,
        waiting_on: "pause",
        paused_reason: "idle_iterations",
        last_request: request("failed"),
      }),
    ).toBeNull();
  });

  it("reports a pending request as checking", () => {
    expect(explainProblem({ ...poolView, last_request: request("pending") })?.kind).toBe(
      "checking",
    );
  });

  it("explains a failed watch with its run", () => {
    const watch: WorkflowScriptRun = {
      id: 9,
      task_key: "REL-1",
      kind: "watch",
      script: "./scripts/ci.sh",
      run_as: "queue",
      state: "finished",
      verdict: "failure",
      exit_code: 1,
      message: "GitHub answered 502\nmore",
      created_at: "2026-10-01T10:09:00Z",
      started_at: "2026-10-01T10:09:01Z",
      finished_at: "2026-10-01T10:09:02Z",
    };
    const view = withCounters(
      {
        ...customerView,
        status: "publish",
        owner: "script",
        waiting_on: "script",
        outcomes: [],
        runs: [watch],
        visits: [
          {
            id: 3,
            sequence: 3,
            status: "publish",
            entered_at: "2026-10-01T10:08:00Z",
            entered_by: "u",
          },
        ],
      },
      { script_failures: 2 },
    );
    const problem = explainProblem(view)!;
    expect(problem.kind).toBe("watch_failed");
    expect(problem.detail).toBe("GitHub answered 502");
    expect(problem.run?.id).toBe(9);
    expect(problem.counter).toBe("2 of 3 script failures");
  });

  it("says a pool status waits for an agent when nobody holds it", () => {
    const problem = explainProblem({ ...poolView, holder: undefined, category: "open" })!;
    expect(problem.kind).toBe("no_agent");
    expect(problem.advice).toMatch(/pool writers/);
  });

  it("counts idle iterations quietly and stays silent otherwise", () => {
    const idle = explainProblem(withCounters(poolView, { idle_iterations: 2 }))!;
    expect(idle.kind).toBe("idle");
    expect(idle.tone).toBe("quiet");
    expect(idle.counter).toBe("2 of 3 idle iterations");
    expect(explainProblem(poolView)).toBeNull();
    expect(explainProblem(customerView)).toBeNull();
  });

  const watchRun: WorkflowScriptRun = {
    ...failedRun,
    id: 9,
    kind: "watch",
    request_id: undefined,
    created_at: "2026-10-01T10:06:00Z",
    started_at: "2026-10-01T10:06:01Z",
  };
  const unheld = { ...poolView, holder: undefined, category: "open" } as WorkflowView;

  it.each([
    ["a failed request", { ...poolView, last_request: request("failed") }, "failed", "danger"],
    [
      "a rejected request",
      { ...poolView, last_request: request("rejected") },
      "rejected",
      "attention",
    ],
    ["a pending request", { ...poolView, last_request: request("pending") }, "checking", "quiet"],
    ["a failed watch", { ...poolView, runs: [watchRun] }, "watch_failed", "danger"],
    ["an unheld pool status", unheld, "no_agent", "attention"],
    ["idle iterations", withCounters(poolView, { idle_iterations: 1 }), "idle", "quiet"],
    [
      "a request before a failed watch",
      { ...poolView, last_request: request("rejected"), runs: [watchRun] },
      "rejected",
      "attention",
    ],
    ["a failed watch before no agent", { ...unheld, runs: [watchRun] }, "watch_failed", "danger"],
    [
      "no agent before idle iterations",
      withCounters(unheld, { idle_iterations: 1 }),
      "no_agent",
      "attention",
    ],
  ] as const)("reads %s", (_name, view, kind, tone) => {
    const problem = explainProblem(view as WorkflowView);
    expect([problem?.kind, problem?.tone]).toEqual([kind, tone]);
  });
});
