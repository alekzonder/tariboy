import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { ApiError, type MetricsFilter, type MetricsReport, type MetricsSlice } from "@/lib/api";
import { apiWrapper, fakeApi } from "@/test/api";
import { metricsReport, metricsSlice } from "@/test/fixtures/apiFixtures";
import MetricsComparePage from "./MetricsComparePage";

function Location() {
  const location = useLocation();
  return <output data-testid="location">{location.pathname + location.search}</output>;
}
const location = () => screen.getByTestId("location").textContent ?? "";
const query = () => new URLSearchParams(location().split("?")[1] ?? "");

const version = (v: string, tasks: Partial<MetricsSlice["tasks"]> = {}, cost = 12.4) => {
  const s = metricsSlice({ name: "developer", version: v });
  s.tasks = { ...s.tasks, ...tasks };
  s.usage = { ...s.usage, cost_usd: cost };
  return s;
};

const IMAGES = [
  version("0.9.0", { tasks_done: 8, mixed_tasks: 3 }, 2),
  version(
    "0.10.0",
    { tasks_done: 9, mixed_tasks: 1, avg: { ...metricsSlice().tasks.avg!, cost_usd: 1 } },
    3,
  ),
  version("unknown"),
];

function renderPage(
  path: string,
  answer: MetricsReport | Error | (() => Promise<MetricsReport>) = metricsReport({
    images: IMAGES,
  }),
) {
  const getMetrics = vi.fn((_filter: MetricsFilter, _signal?: AbortSignal) => {
    if (typeof answer === "function") return answer();
    return answer instanceof Error ? Promise.reject(answer) : Promise.resolve(answer);
  });
  const Wrapper = apiWrapper(fakeApi({ metrics: { getMetrics } }));
  render(
    <MemoryRouter initialEntries={[path]}>
      <Wrapper>
        <Location />
        <Routes>
          <Route path="/metrics/compare" element={<MetricsComparePage />} />
          <Route path="/metrics" element={<p>Metrics page</p>} />
        </Routes>
      </Wrapper>
    </MemoryRouter>,
  );
  return { getMetrics };
}

const table = () => screen.getByRole("table", { name: "Version comparison" });
const row = (label: string) =>
  within(table())
    .getAllByRole("row")
    .find((r) => within(r).queryByRole("rowheader")?.textContent === label)!;
const cells = (label: string) =>
  within(row(label))
    .getAllByRole("cell")
    .map((c) => c.textContent);

describe("MetricsComparePage", () => {
  it("asks for one name over the period and compares the newest version with the one before", async () => {
    const { getMetrics } = renderPage("/metrics/compare?kind=image&name=developer&period=week");
    expect(await screen.findByRole("heading", { name: "Compare image versions" })).toBeVisible();
    await waitFor(() => expect(getMetrics).toHaveBeenCalled());
    const filter = getMetrics.mock.calls[0][0];
    expect(filter.image).toBe("developer");
    expect(new Date(filter.until).getTime() - new Date(filter.since).getTime()).toBe(7 * 864e5);
    await screen.findByRole("table", { name: "Version comparison" });
    expect(screen.getByLabelText("Version A")).toHaveValue("0.9.0");
    expect(screen.getByLabelText("Version B")).toHaveValue("0.10.0");
    expect(
      within(table()).getByRole("columnheader", { name: /^Version A\s*0\.9\.0$/ }),
    ).toBeInTheDocument();
    expect(cells("Tasks done")).toEqual(["8", "9", "+1 (+13%)"]);
    expect(cells("Cost per task")).toEqual(["$1.37", "$1.00", "−$0.37 (−27%) better"]);
    expect(cells("Cost in period")).toEqual(["$2.00", "$3.00", "+$1.00 (+50%)"]);
    expect(screen.getByText("Change shows version B minus version A.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "← Metrics" })).toHaveAttribute(
      "href",
      "/metrics?period=week",
    );
  });

  it("compares a workflow's versions", async () => {
    const { getMetrics } = renderPage(
      "/metrics/compare?kind=workflow&name=development",
      metricsReport({
        workflows: [
          metricsSlice({ name: "development", version: "0.10.1" }),
          metricsSlice({ name: "development", version: "0.10.0" }),
        ],
      }),
    );
    expect(await screen.findByRole("heading", { name: "Compare workflow versions" })).toBeVisible();
    await waitFor(() => expect(getMetrics.mock.calls[0][0].workflow).toBe("development"));
    expect(await screen.findByLabelText("Version B")).toHaveValue("0.10.1");
  });

  it("uses the versions of the URL and swaps them in place", async () => {
    renderPage("/metrics/compare?kind=image&name=developer&a=0.10.0&b=0.9.0");
    expect(await screen.findByLabelText("Version A")).toHaveValue("0.10.0");
    fireEvent.click(screen.getByRole("button", { name: "Swap" }));
    expect(query().get("a")).toBe("0.9.0");
    expect(query().get("b")).toBe("0.10.0");
    fireEvent.change(screen.getByLabelText("Version A"), { target: { value: "unknown" } });
    expect(query().get("a")).toBe("unknown");
  });

  it("hides the table when both sides are one version", async () => {
    renderPage("/metrics/compare?kind=image&name=developer&a=0.9.0&b=0.9.0");
    expect(await screen.findByText("Choose a different version.")).toBeInTheDocument();
    expect(screen.queryByRole("table", { name: "Version comparison" })).toBeNull();
  });

  it("says which version of the URL has no data and leaves its column unknown", async () => {
    renderPage("/metrics/compare?kind=image&name=developer&a=0.3.0&b=0.10.0");
    expect(await screen.findByText("Version 0.3.0 has no data in this period.")).toBeVisible();
    const [a, , change] = within(row("Tasks done")).getAllByRole("cell");
    expect(within(a).getByTitle("No data for this version in this period.")).toHaveTextContent("—");
    expect(within(change).getByTitle("Not enough data to compare.")).toHaveTextContent("—");
  });

  it("asks for a second version when the period has one", async () => {
    renderPage(
      "/metrics/compare?kind=image&name=developer",
      metricsReport({ images: [version("0.9.0")] }),
    );
    expect(await screen.findByText("Choose a version to compare with.")).toBeVisible();
    expect(screen.queryByRole("table", { name: "Version comparison" })).toBeNull();
  });

  it("offers 90 days when the name has no runs in the period", async () => {
    renderPage("/metrics/compare?kind=image&name=ghost", metricsReport({ images: [] }));
    expect(await screen.findByText("No runs of ghost in this period.")).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Show 90 days" }));
    expect(query().get("period")).toBe("quarter");
  });

  it("opens Metrics without a kind or a name", async () => {
    renderPage("/metrics/compare?name=developer&period=week&host=h");
    expect(await screen.findByText("Metrics page")).toBeInTheDocument();
    expect(location()).toBe("/metrics?host=h&period=week");
  });

  it("warns about a small sample", async () => {
    renderPage(
      "/metrics/compare?kind=image&name=developer",
      metricsReport({
        images: [version("0.1.0", { tasks_done: 2 }), version("0.2.0", { tasks_done: 3 })],
      }),
    );
    expect(
      await screen.findByText("Few completed tasks (A: 2, B: 3). The change can be random."),
    ).toBeVisible();
  });

  it("shows the mixed tasks of both versions", async () => {
    renderPage("/metrics/compare?kind=image&name=developer");
    const block = await screen.findByRole("region", { name: "Mixed tasks" });
    expect(block).toHaveTextContent(
      "Tasks that ran on more than one version are not counted above.",
    );
    expect(block).toHaveTextContent("Version A: 3");
    expect(block).toHaveTextContent("Version B: 1");
  });

  it("says when no task ran on two versions", async () => {
    renderPage(
      "/metrics/compare?kind=image&name=developer",
      metricsReport({
        images: [version("0.1.0", { mixed_tasks: 0 }), version("0.2.0", { mixed_tasks: 0 })],
      }),
    );
    expect(await screen.findByText("No tasks ran on more than one version.")).toBeVisible();
  });

  it("shows one Quality row when neither version has scores", async () => {
    renderPage(
      "/metrics/compare?kind=image&name=developer",
      metricsReport({
        images: [version("0.1.0", { quality: null }), version("0.2.0", { quality: null })],
      }),
    );
    await screen.findByRole("table", { name: "Version comparison" });
    expect(cells("Quality").slice(0, 2)).toEqual([
      "No quality scores yet",
      "No quality scores yet",
    ]);
  });

  it("announces the first load", async () => {
    renderPage("/metrics/compare?kind=image&name=developer", () => new Promise(() => {}));
    expect(
      await screen.findByText("Loading metrics…", { selector: "[role=status]" }),
    ).toBeInTheDocument();
  });

  it("shows a failure, a timeout and an older host", async () => {
    renderPage("/metrics/compare?kind=image&name=d", new ApiError(500, "internal", "disk full"));
    expect(await screen.findByRole("alert")).toHaveTextContent("disk full");
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
  });

  it("explains a timeout", async () => {
    renderPage(
      "/metrics/compare?kind=image&name=d",
      new ApiError(0, "network_error", "request timed out"),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The host did not answer in 30 seconds.",
    );
  });

  it("explains an older host", async () => {
    renderPage(
      "/metrics/compare?kind=image&name=d",
      new ApiError(404, "not_found", "unknown route GET /api/metrics"),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "This host runs an older daemon without metrics.",
    );
  });
});
