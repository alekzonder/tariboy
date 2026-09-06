import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { IterationJudgePanel } from "./IterationJudgePanel";
import type { IterationJudgeProjection } from "@/lib/types";

vi.mock("@/lib/judge", async (importOriginal) => ({
  ...await importOriginal<typeof import("@/lib/judge")>(),
  reviewIterationOn: vi.fn(),
  getIterationJudgeReviewsOn: vi.fn(),
}));

import { getIterationJudgeReviewsOn, reviewIterationOn } from "@/lib/judge";

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

  it("does not offer or start a review for an unfinished iteration", async () => {
    vi.mocked(getIterationJudgeReviewsOn).mockResolvedValue({ reviews: [] });
    renderPanel(empty, false);
    expect(await screen.findByText("Judge review is available after this iteration finishes.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Judge review/ })).not.toBeInTheDocument();
    expect(reviewIterationOn).not.toHaveBeenCalled();
  });
});
