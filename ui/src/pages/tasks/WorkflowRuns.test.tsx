import type { ReactElement } from "react";
import { act, cleanup, fireEvent, render as renderWith, screen, within } from "@/test/queryRender";
import { beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api";
import WorkflowRuns from "./WorkflowRuns";
import { checkRun, watchRun, remoteTarget } from "@/test/fixtures/workflowFixtures";
import { fakeApi } from "@/test/api";

const api = vi.hoisted(() => ({ getTaskScriptRunLog: vi.fn() }));
// The mocks are methods of a client bound to the remote host, so each call
// they record went to that host.
const remoteApi = fakeApi(
  { workflows: { getTaskScriptRunLog: api.getTaskScriptRunLog } },
  remoteTarget,
);
const render = (ui: ReactElement) => renderWith(ui, { api: remoteApi });

beforeEach(() => {
  vi.clearAllMocks();
});

it("lists runs newest first with kind, script, state, verdict and exit code", () => {
  render(<WorkflowRuns taskKey="REL-1" runs={[checkRun, watchRun]} />);
  const rows = screen.getAllByRole("listitem");
  expect(rows).toHaveLength(2);
  expect(rows[0]).toHaveTextContent("watch");
  expect(rows[0]).toHaveTextContent("./scripts/ci.sh");
  expect(rows[0]).toHaveTextContent("running");
  expect(rows[1]).toHaveTextContent("check");
  expect(rows[1]).toHaveTextContent("./checks/notes.sh");
  expect(rows[1]).toHaveTextContent("finished");
  expect(rows[1]).toHaveTextContent("reject");
  expect(rows[1]).toHaveTextContent("exit 112");
  expect(rows[1]).toHaveTextContent("the notes must start with a heading");
});

it("gives each run the tone of its verdict", () => {
  render(
    <WorkflowRuns
      taskKey="REL-1"
      runs={[
        checkRun,
        watchRun,
        { ...checkRun, id: 5, verdict: "pass", exit_code: 0 },
        { ...watchRun, id: 4, state: "finished", verdict: "quiet", exit_code: 111 },
      ]}
    />,
  );
  const rows = screen.getAllByRole("listitem");
  expect(rows.map((row) => row.getAttribute("data-tone"))).toEqual([
    "attention",
    "danger",
    "live",
    "faint",
  ]);
  expect(within(rows[1]).getByText("reject")).toHaveAttribute("data-tone", "danger");
});

it("says so when there are no runs", () => {
  render(<WorkflowRuns taskKey="REL-1" runs={[]} />);
  expect(screen.getByText("No script has run yet.")).toBeInTheDocument();
});

it("loads a log on demand and notes a truncated one", async () => {
  api.getTaskScriptRunLog.mockResolvedValue({ text: "<script>x</script>\nok", truncated: true });
  render(<WorkflowRuns taskKey="REL-1" runs={[checkRun]} />);
  expect(api.getTaskScriptRunLog).not.toHaveBeenCalled();
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Log" }));
  });
  expect(api.getTaskScriptRunLog).toHaveBeenCalledWith("REL-1", 7, undefined);
  const row = screen.getByRole("listitem");
  expect(row.querySelector("pre")!.textContent).toBe("<script>x</script>\nok");
  expect(row.querySelector("pre")).toHaveAttribute("tabindex", "0");
  expect(row.querySelector("script")).toBeNull();
  expect(within(row).getByText(/truncated/i)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Hide log" }));
  expect(row.querySelector("pre")).toBeNull();
});

it("says the list holds the latest 20 runs", () => {
  render(<WorkflowRuns taskKey="REL-1" runs={[checkRun]} />);
  expect(screen.getByText(/latest 20/)).toBeInTheDocument();
});

it("refreshes an open log while its run is still going, and not once it finished", async () => {
  api.getTaskScriptRunLog
    .mockResolvedValueOnce({ text: "line 1", truncated: false })
    .mockResolvedValueOnce({ text: "line 1\nline 2", truncated: false });
  render(<WorkflowRuns taskKey="REL-1" runs={[watchRun]} />);
  expect(screen.queryByRole("button", { name: "Refresh log" })).not.toBeInTheDocument();
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Log" }));
  });
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Refresh log" }));
  });
  expect(api.getTaskScriptRunLog).toHaveBeenCalledTimes(2);
  expect(screen.getByRole("listitem").querySelector("pre")!.textContent).toBe("line 1\nline 2");
  cleanup();
  api.getTaskScriptRunLog.mockResolvedValue({ text: "done", truncated: false });
  render(<WorkflowRuns taskKey="REL-1" runs={[checkRun]} />);
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Log" }));
  });
  expect(screen.queryByRole("button", { name: "Refresh log" })).not.toBeInTheDocument();
});

it("shows a forbidden log inline", async () => {
  api.getTaskScriptRunLog.mockRejectedValue(
    new ApiError(403, "forbidden", "you may not read this log"),
  );
  render(<WorkflowRuns taskKey="REL-1" runs={[checkRun]} />);
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Log" }));
  });
  expect(screen.getByRole("alert")).toHaveTextContent("you may not read this log");
});
