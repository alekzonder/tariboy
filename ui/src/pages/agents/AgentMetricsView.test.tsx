import { describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { ApiError, type MetricsFilter, type MetricsReport } from "@/lib/api";
import { AgentNameContext } from "@/lib/agent";
import { apiWrapper, fakeApi } from "@/test/api";
import { metricsReport, metricsSlice } from "@/test/fixtures/apiFixtures";
import AgentMetricsView from "./AgentMetricsView";

type Answer = MetricsReport | Error | (() => Promise<MetricsReport>);

function Location() {
  const location = useLocation();
  return <output data-testid="location">{location.search}</output>;
}
const query = () => new URLSearchParams(screen.getByTestId("location").textContent ?? "");

function renderView(search = "", answer: Answer = metricsReport()) {
  const getMetrics = vi.fn((_filter: MetricsFilter, _signal?: AbortSignal) => {
    if (typeof answer === "function") return answer();
    return answer instanceof Error ? Promise.reject(answer) : Promise.resolve(answer);
  });
  const Wrapper = apiWrapper(fakeApi({ metrics: { getMetrics } }));
  render(
    <MemoryRouter initialEntries={[`/agents/local/alice/activity?view=metrics${search}`]}>
      <Wrapper>
        <AgentNameContext.Provider value="alice">
          <Routes>
            <Route
              path="/agents/:hostId/:agent/activity"
              element={
                <>
                  <Location />
                  <AgentMetricsView />
                </>
              }
            />
          </Routes>
        </AgentNameContext.Provider>
      </Wrapper>
    </MemoryRouter>,
  );
  return { getMetrics };
}

const ZERO_TOKENS = { input: 0, output: 0, cache_write: 0, cache_read: 0 };
const emptyReport = () =>
  metricsReport({
    total: metricsSlice({
      name: undefined,
      version: undefined,
      tasks: { ...metricsSlice().tasks, tasks_done: 0, avg: undefined },
      usage: { iterations: 0, cost_usd: 0, tokens: ZERO_TOKENS },
    }),
    agents: [],
    images: [],
    workflows: [],
  });

const metricsLink = () =>
  new URLSearchParams(
    (screen.getByRole("link", { name: "Open in Metrics" }).getAttribute("href") ?? "").split(
      "?",
    )[1],
  );

describe("AgentMetricsView", () => {
  it("asks for this agent's metrics from local midnight to now by default", async () => {
    const { getMetrics } = renderView();
    await screen.findByText(
      "Completed tasks of this agent in this period. Runs and cost count each task's whole life.",
    );
    const filter = getMetrics.mock.calls[0][0];
    expect(filter.agent).toBe("alice");
    const since = new Date(filter.since);
    expect([since.getHours(), since.getMinutes(), since.getSeconds()]).toEqual([0, 0, 0]);
    expect(Math.abs(new Date(filter.until).getTime() - Date.now())).toBeLessThan(5000);
  });

  it("shows the task cards without AI usage, Quality, Images and Workflows", async () => {
    renderView();
    await waitFor(() =>
      expect(document.querySelector('[data-card="Tasks done"] [data-value]')).toHaveTextContent(
        "7",
      ),
    );
    expect(screen.queryByText("AI usage in this period")).not.toBeInTheDocument();
    expect(document.querySelector('[data-card="Runs"]')).toBeNull();
    expect(screen.getByRole("listitem", { name: "Role" })).toHaveTextContent("74");
    expect(screen.getByRole("heading", { name: "Images" })).toBeInTheDocument();
    expect(screen.getByRole("table", { name: "developer" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Workflows" })).toBeInTheDocument();
    expect(screen.getByRole("table", { name: "development" })).toBeInTheDocument();
  });

  it("shows the empty Quality state when nothing is scored", async () => {
    const total = metricsSlice({ name: undefined, version: undefined });
    renderView(
      "",
      metricsReport({ total: { ...total, tasks: { ...total.tasks, quality: null } } }),
    );
    expect(await screen.findByText("No quality scores yet")).toBeInTheDocument();
    expect(screen.getByText("Scores appear here after tasks are assessed.")).toBeInTheDocument();
  });

  it("links Today to the Metrics page as a custom range on the same window", async () => {
    const { getMetrics } = renderView();
    await screen.findByRole("link", { name: "Open in Metrics" });
    const link = metricsLink();
    expect(link.get("host")).toBe("local");
    expect(link.get("agent")).toBe("alice");
    expect(link.get("period")).toBe("custom");
    const { since, until } = getMetrics.mock.calls[0][0];
    expect(new Date(link.get("since") ?? "").getTime()).toBe(new Date(since).getTime());
    // A datetime-local value keeps the minute of the window's end.
    const end = new Date(until);
    end.setSeconds(0, 0);
    expect(new Date(link.get("until") ?? "").getTime()).toBe(end.getTime());
  });

  it.each(["week", "month"])("links %s to the Metrics page as it is", async (period) => {
    renderView(`&period=${period}`);
    await screen.findByRole("link", { name: "Open in Metrics" });
    expect(metricsLink().get("period")).toBe(period);
    expect(metricsLink().get("since")).toBeNull();
  });

  it("links a custom range to the Metrics page with its own bounds", async () => {
    const { getMetrics } = renderView(
      "&period=custom&since=2026-10-01T00:00&until=2026-10-08T00:00",
    );
    await screen.findByRole("link", { name: "Open in Metrics" });
    expect(Object.fromEntries(metricsLink())).toEqual({
      host: "local",
      agent: "alice",
      period: "custom",
      since: "2026-10-01T00:00",
      until: "2026-10-08T00:00",
    });
    expect(getMetrics.mock.calls[0][0].since).toBe(new Date("2026-10-01T00:00").toISOString());
  });

  it("makes no request for an unusable custom range", () => {
    const { getMetrics } = renderView("&period=custom");
    expect(getMetrics).not.toHaveBeenCalled();
  });

  it("links a version pair to the compare screen on the same window", async () => {
    const report = metricsReport({
      images: [
        metricsSlice({ name: "developer", version: "0.23.2" }),
        metricsSlice({ name: "developer", version: "0.23.1" }),
      ],
    });
    renderView("&period=week", report);
    const link = await screen.findByRole("link", { name: "Compare versions" });
    expect(link.getAttribute("href")).toBe(
      "/metrics/compare?host=local&period=week&kind=image&name=developer&a=0.23.1&b=0.23.2",
    );
  });

  it("announces the first load", () => {
    renderView("", () => new Promise(() => {}));
    expect(screen.getByText("Loading metrics…")).toHaveAttribute("role", "status");
  });

  it("shows an error with Retry and loads again", async () => {
    const { getMetrics } = renderView("", new ApiError(500, "internal", "boom"));
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Could not load metrics.");
    expect(alert).toHaveTextContent("boom");
    fireEvent.click(within(alert).getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(getMetrics).toHaveBeenCalledTimes(2));
  });

  it("explains an older daemon", async () => {
    renderView("", new ApiError(404, "not_found", "unknown route GET /api/metrics"));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "This host runs an older daemon without metrics.",
    );
  });

  it("explains a timeout", async () => {
    renderView("", new ApiError(0, "network_error", "request timed out"));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The host did not answer in 30 seconds.",
    );
  });

  it.each(["", "&period=week"])("offers the month for an empty period (%s)", async (search) => {
    renderView(search, emptyReport());
    expect(
      await screen.findByText("No completed tasks of this agent in this period."),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Show month" }));
    await waitFor(() => expect(query().get("period")).toBe("month"));
    expect(query().get("view")).toBe("metrics");
  });

  it("does not offer the month for an empty month", async () => {
    renderView("&period=month", emptyReport());
    await screen.findByText("No completed tasks of this agent in this period.");
    expect(screen.queryByRole("button", { name: "Show month" })).not.toBeInTheDocument();
  });

  it("says so when the period has runs but no image or workflow", async () => {
    renderView("", metricsReport({ images: [], workflows: [] }));
    expect(await screen.findByText("No images in this period.")).toBeInTheDocument();
    expect(screen.getByText("No workflows in this period.")).toBeInTheDocument();
  });

  it("keeps the numbers, marked as updating, while Refresh loads", async () => {
    let calls = 0;
    const { getMetrics } = renderView("", () =>
      calls++ === 0 ? Promise.resolve(metricsReport()) : new Promise(() => {}),
    );
    await screen.findByText("Updated", { exact: false });
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Refresh" })));
    expect(getMetrics).toHaveBeenCalledTimes(2);
    expect(screen.getByRole("button", { name: "Updating…" })).toBeInTheDocument();
    expect(document.querySelector('[data-card="Tasks done"] [data-value]')).toHaveTextContent("7");
  });
});
