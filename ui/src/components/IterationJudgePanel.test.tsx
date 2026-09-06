import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { IterationJudgePanel } from "./IterationJudgePanel";
import type { IterationJudgeProjection } from "@/lib/types";

vi.mock("@/lib/judge", async (importOriginal) => ({
  ...await importOriginal<typeof import("@/lib/judge")>(),
  reviewIterationOn: vi.fn(),
  getIterationJudgeReviewsOn: vi.fn(),
  getJudgeAutomation: vi.fn(),
}));

vi.mock("@/lib/api", async (importOriginal) => ({
  ...await importOriginal<typeof import("@/lib/api")>(),
  agentGetOn: vi.fn(),
}));

vi.mock("@/lib/terminalsHost", async (importOriginal) => ({
  ...await importOriginal<typeof import("@/lib/terminalsHost")>(),
  targetFor: vi.fn((hostId: string) => hostId ? { id: hostId, label: hostId, baseURL: "", token: "" } : null),
}));

import { getIterationJudgeReviewsOn, getJudgeAutomation, reviewIterationOn } from "@/lib/judge";
import { agentGetOn } from "@/lib/api";
import { targetFor } from "@/lib/terminalsHost";

const completed = {
  run_id: "run 1", target_id: "target 1", created_at: "2026-09-06T12:00:00Z",
  state: "done", verdict: "pass", score: 0.8, completed: 2, failed: 0, pending: 0,
};
const empty: IterationJudgeProjection = { latest_completed: null, active: null };

function renderPanel(judge: IterationJudgeProjection = empty, terminal = true) {
  return render(
    <MemoryRouter initialEntries={["/agents/local/worker/activity"]}>
      <Routes><Route path="/agents/:hostId/:agent/activity" element={
        <IterationJudgePanel agentName="worker" iterationId="iteration-1" terminal={terminal} judge={judge} />
      } /></Routes>
    </MemoryRouter>,
  );
}

function renderRemotePanel() {
  return render(
    <MemoryRouter initialEntries={["/agents/remote-a/worker/activity"]}>
      <Routes><Route path="/agents/:hostId/:agent/activity" element={
        <IterationJudgePanel agentName="worker" iterationId="iteration-1" terminal judge={empty} />
      } /></Routes>
    </MemoryRouter>,
  );
}

afterEach(() => vi.clearAllMocks());

beforeEach(() => {
  vi.mocked(targetFor).mockImplementation((hostId) => hostId ? { id: hostId, label: hostId, baseURL: "", token: "" } : null);
  vi.mocked(getJudgeAutomation).mockResolvedValue({ configured: false });
  vi.mocked(agentGetOn).mockResolvedValue({ loop_enabled: true } as never);
});

describe("IterationJudgePanel", () => {
  it("renders a completed score of zero as a number", async () => {
    vi.mocked(getIterationJudgeReviewsOn).mockResolvedValue({ reviews: [{ ...completed, score: 0 }] });
    renderPanel({ latest_completed: { ...completed, score: 0 }, active: null });
    expect(await screen.findByText("Score 0")).toBeInTheDocument();
  });

  it("keeps the previous score visible while a newer review is pending", async () => {
    const active = { ...completed, run_id: "run 2", target_id: "target 2", state: "pending", verdict: "", score: null, completed: 0, pending: 2 };
    vi.mocked(getIterationJudgeReviewsOn).mockResolvedValue({ reviews: [active, completed] });
    renderPanel({ latest_completed: completed, active });
    expect(await screen.findByText("Score 0.8")).toBeInTheDocument();
    expect(screen.getByText("Queued")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open active review" })).toHaveAttribute(
      "href", "/servers/local/settings/advanced/judges/run%202?target=target+2",
    );
    expect(screen.queryByRole("button", { name: "Run Judge review" })).not.toBeInTheDocument();
  });

  it("sends only one POST for a double click", async () => {
    let resolvePost!: (value: { id: string; status: string; targets: number }) => void;
    vi.mocked(reviewIterationOn).mockReturnValue(new Promise((resolve) => { resolvePost = resolve; }));
    vi.mocked(getIterationJudgeReviewsOn).mockResolvedValue({ reviews: [] });
    renderPanel();
    const button = await screen.findByRole("button", { name: "Run Judge review" });
    fireEvent.click(button);
    fireEvent.click(button);
    expect(reviewIterationOn).toHaveBeenCalledTimes(1);
    resolvePost({ id: "run", status: "running", targets: 1 });
  });

  it("shows a POST error and allows retry", async () => {
    vi.mocked(getIterationJudgeReviewsOn).mockResolvedValue({ reviews: [] });
    vi.mocked(reviewIterationOn)
      .mockRejectedValueOnce(new Error("workers unavailable"))
      .mockResolvedValueOnce({ id: "run", status: "running", targets: 1 });
    renderPanel();
    fireEvent.click(await screen.findByRole("button", { name: "Run Judge review" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("workers unavailable");
    fireEvent.click(screen.getByRole("button", { name: "Retry Judge review" }));
    await waitFor(() => expect(reviewIterationOn).toHaveBeenCalledTimes(2));
  });

  it("captures the route host and shows queued until history confirms the target", async () => {
    let resolveHistory!: (value: { reviews: [] }) => void;
    vi.mocked(getIterationJudgeReviewsOn)
      .mockResolvedValueOnce({ reviews: [] })
      .mockReturnValueOnce(new Promise((resolve) => { resolveHistory = resolve; }));
    vi.mocked(reviewIterationOn).mockResolvedValue({ id: "run", status: "running", targets: 1 });
    renderRemotePanel();

    fireEvent.click(await screen.findByRole("button", { name: "Run Judge review" }));
    await waitFor(() => expect(screen.getByText("Queued — waiting for the review target to be confirmed.")).toBeInTheDocument());
    expect(reviewIterationOn).toHaveBeenCalledWith(expect.objectContaining({ id: "remote-a" }), "iteration-1");
    resolveHistory({ reviews: [] });
  });

  it("disables the button while the POST is pending", async () => {
    vi.mocked(getIterationJudgeReviewsOn).mockResolvedValue({ reviews: [] });
    vi.mocked(reviewIterationOn).mockReturnValue(new Promise(() => {}));
    renderPanel();
    fireEvent.click(await screen.findByRole("button", { name: "Run Judge review" }));
    expect(await screen.findByRole("button", { name: "Queuing Judge review…" })).toBeDisabled();
  });

  it("does not turn an accepted POST into a retry when its history refresh fails", async () => {
    vi.mocked(getIterationJudgeReviewsOn)
      .mockResolvedValueOnce({ reviews: [] })
      .mockRejectedValueOnce(new Error("history offline"));
    vi.mocked(reviewIterationOn).mockResolvedValue({ id: "run", status: "running", targets: 1 });
    renderPanel();
    fireEvent.click(await screen.findByRole("button", { name: "Run Judge review" }));
    expect(await screen.findByText("Queued — waiting for the review target to be confirmed.")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Retry Judge review" })).not.toBeInTheDocument();
  });

  it("drops stale active history when a refresh reports the review terminal", async () => {
    const pending = { ...completed, run_id: "run 2", target_id: "target 2", state: "pending", verdict: "", score: null, completed: 0, pending: 1 };
    vi.mocked(getIterationJudgeReviewsOn)
      .mockResolvedValueOnce({ reviews: [pending, completed] })
      .mockResolvedValue({ reviews: [{ ...pending, state: "terminal", pending: 0, failed: 1 }, completed] });
    renderPanel({ latest_completed: completed, active: pending });
    expect(await screen.findByRole("link", { name: "Open active review" })).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole("button", { name: "Run Judge review" })).toBeInTheDocument(), { timeout: 4000 });
    expect(screen.getByText("Score 0.8")).toBeInTheDocument();
  });

  it("reports disabled configured workers without enabling them", async () => {
    const pending = { ...completed, state: "pending", verdict: "", score: null, completed: 0, pending: 1 };
    vi.mocked(getIterationJudgeReviewsOn).mockResolvedValue({ reviews: [pending] });
    vi.mocked(getJudgeAutomation).mockResolvedValue({ configured: true, revision: { revision: 1, hash: "h", created_at: "now", canonical_json: JSON.stringify({ judge: { workers: ["judge-a"] } }) } });
    vi.mocked(agentGetOn).mockResolvedValue({ loop_enabled: false } as never);
    renderPanel({ latest_completed: null, active: pending });
    expect(await screen.findByText("Waiting: Judge worker judge-a has Autopilot disabled.")).toBeInTheDocument();
  });

  it("ignores an in-flight history response from an obsolete same-host descriptor", async () => {
    let resolveOld!: (value: { reviews: (typeof completed)[] }) => void;
    const oldTarget = { id: "remote-a", label: "old", baseURL: "https://old.example", token: "old" };
    const newTarget = { id: "remote-a", label: "new", baseURL: "https://new.example", token: "new" };
    vi.mocked(targetFor).mockReturnValue(oldTarget);
    vi.mocked(getIterationJudgeReviewsOn).mockReturnValueOnce(new Promise((resolve) => { resolveOld = resolve; }));
    const view = renderRemotePanel();

    vi.mocked(targetFor).mockReturnValue(newTarget);
    vi.mocked(getIterationJudgeReviewsOn).mockResolvedValue({ reviews: [{ ...completed, run_id: "new run" }] });
    view.rerender(
      <MemoryRouter initialEntries={["/agents/remote-a/worker/activity"]}>
        <Routes><Route path="/agents/:hostId/:agent/activity" element={
          <IterationJudgePanel agentName="worker" iterationId="iteration-1" terminal judge={empty} />
        } /></Routes>
      </MemoryRouter>,
    );
    expect(await screen.findByRole("link", { name: /pass · 0.8/ })).toHaveAttribute("href", expect.stringContaining("new%20run"));
    resolveOld({ reviews: [completed] });
    await waitFor(() => expect(screen.getByRole("link", { name: /pass · 0.8/ })).toHaveAttribute("href", expect.stringContaining("new%20run")));
  });

  it("does not offer or start a review for an unfinished iteration", async () => {
    vi.mocked(getIterationJudgeReviewsOn).mockResolvedValue({ reviews: [] });
    renderPanel(empty, false);
    expect(await screen.findByText("Judge review is available after this iteration finishes.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Judge review/ })).not.toBeInTheDocument();
    expect(reviewIterationOn).not.toHaveBeenCalled();
  });
});
