import { describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { ApiError, type MetricsFilter, type MetricsReport } from "@/lib/api";
import { apiWrapper, fakeApi } from "@/test/api";
import { imageSummary, metricsReport, metricsSlice } from "@/test/fixtures/apiFixtures";
import MetricsPage from "./MetricsPage";

function Location() {
  const location = useLocation();
  return <output data-testid="location">{location.pathname + location.search}</output>;
}
const location = () => screen.getByTestId("location").textContent ?? "";
const query = () => new URLSearchParams(location().split("?")[1] ?? "");

type Answer = MetricsReport | Error | ((filter: MetricsFilter) => Promise<MetricsReport>);

function renderPage(path = "/metrics", answer: Answer = metricsReport()) {
  const getMetrics = vi.fn((filter: MetricsFilter, _signal?: AbortSignal) => {
    if (typeof answer === "function") return answer(filter);
    return answer instanceof Error ? Promise.reject(answer) : Promise.resolve(answer);
  });
  const client = fakeApi({
    metrics: { getMetrics },
    images: {
      listImages: async () => ({
        count: 2,
        images: [
          imageSummary({ name: "developer", tag: "a", image_version: "0.23.2" }),
          imageSummary({ name: "developer", tag: "b", image_version: "0.23.1" }),
        ],
      }),
    },
    workflows: {
      listWorkflowImages: async () => [
        { name: "development", tag: "latest", version: "0.10.1", digest: "d", built_at: "" },
      ],
    },
  });
  const Wrapper = apiWrapper(client);
  render(
    <MemoryRouter initialEntries={[path]}>
      <Wrapper>
        <Location />
        <MetricsPage hostId="" agents={["alice", "bob"]} />
      </Wrapper>
    </MemoryRouter>,
  );
  return { getMetrics };
}

const card = (label: string) =>
  (document.querySelector(`[data-card="${label}"] [data-value]`) as HTMLElement).textContent;

describe("MetricsPage", () => {
  it("shows the total cards, Quality and the Agents table of the full report", async () => {
    renderPage();
    expect(await screen.findByRole("heading", { name: "Metrics" })).toBeInTheDocument();
    await waitFor(() => expect(card("Tasks done")).toBe("7"));
    expect(card("Cost")).toBe("$12.40");
    expect(screen.getByRole("listitem", { name: "Role" })).toHaveTextContent("74");
    const table = screen.getByRole("table", { name: "Agents" });
    const row = within(table).getAllByRole("row")[1];
    expect(within(row).getByRole("rowheader")).toHaveTextContent("alice");
    expect(
      within(row)
        .getAllByRole("cell")
        .map((c) => c.textContent),
    ).toEqual(["7", "12h 30m", "$1.37", "2.3", "0.4", "0.1", "0.6", "74", "23", "$12.40"]);
    expect(
      screen.getByText("A task with several agents counts for each of them. Total counts it once."),
    ).toBeInTheDocument();
    expect(within(table).getByRole("columnheader", { name: /Cost in period/ })).toHaveAttribute(
      "aria-sort",
      "descending",
    );
  });

  it("asks for the month before now in RFC 3339 by default", async () => {
    const { getMetrics } = renderPage();
    await waitFor(() => expect(getMetrics).toHaveBeenCalledTimes(1));
    const { since, until } = getMetrics.mock.calls[0][0];
    expect(new Date(until).getTime() - new Date(since).getTime()).toBe(30 * 24 * 3600e3);
    expect(since).toMatch(/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z$/);
    expect(screen.getByRole("button", { name: "Month" })).toHaveAttribute("aria-pressed", "true");
  });

  it("reopens a link with its period, view and filters", async () => {
    const { getMetrics } = renderPage(
      "/metrics?period=quarter&view=workflows&agent=alice&image=developer:0.23.2&workflow=none",
    );
    await waitFor(() => expect(getMetrics).toHaveBeenCalled());
    expect(getMetrics.mock.calls[0][0]).toMatchObject({
      agent: "alice",
      image: "developer:0.23.2",
      workflow: "none",
    });
    expect(screen.getByRole("button", { name: "Quarter" })).toHaveAttribute("aria-pressed", "true");
    expect(await screen.findByRole("table", { name: "development" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Remove filter Agent: alice" })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Remove filter Image: developer:0.23.2" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Remove filter Workflow: No workflow" }),
    ).toBeInTheDocument();
  });

  it("switches the view and the period in the URL, replacing the entry", async () => {
    renderPage();
    await screen.findByRole("table", { name: "Agents" });
    fireEvent.click(screen.getByRole("radio", { name: "Images" }));
    expect(query().get("view")).toBe("images");
    expect(await screen.findByRole("table", { name: "developer" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Week" }));
    expect(query().get("period")).toBe("week");
  });

  it("sets a filter from its list and removes it with its chip or Clear filters", async () => {
    const { getMetrics } = renderPage();
    await screen.findByRole("table", { name: "Agents" });
    fireEvent.change(screen.getByLabelText("Agent"), { target: { value: "bob" } });
    expect(query().get("agent")).toBe("bob");
    await waitFor(() => expect(getMetrics.mock.calls.at(-1)?.[0].agent).toBe("bob"));
    fireEvent.click(screen.getByRole("button", { name: "Remove filter Agent: bob" }));
    expect(query().has("agent")).toBe(false);

    fireEvent.change(await screen.findByLabelText("Image"), { target: { value: "developer" } });
    expect(query().get("image")).toBe("developer");
    const version = await screen.findByLabelText("Image version");
    expect(
      within(version)
        .getAllByRole("option")
        .map((o) => o.textContent),
    ).toEqual(["All versions", "0.23.2", "0.23.1", "unknown version"]);
    fireEvent.change(version, { target: { value: "0.23.1" } });
    expect(query().get("image")).toBe("developer:0.23.1");
    fireEvent.change(screen.getByLabelText("Workflow"), { target: { value: "none" } });
    expect(query().get("workflow")).toBe("none");
    expect(screen.getByRole("button", { name: "Filters (2)" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Clear filters" }));
    expect(query().has("image")).toBe(false);
    expect(query().has("workflow")).toBe(false);
  });

  it("keeps a filter for an agent that is not on the host any more", async () => {
    renderPage("/metrics?agent=gone");
    expect(await screen.findByRole("button", { name: "Remove filter Agent: gone" })).toBeVisible();
    expect(
      within(screen.getByLabelText("Agent")).getByRole("option", { name: "gone" }),
    ).toBeTruthy();
  });

  it("links an agent to its Metrics view on the same window, and not a removed one", async () => {
    renderPage(
      "/metrics",
      metricsReport({
        agents: [
          metricsSlice({ name: "alice", version: undefined }),
          metricsSlice({ name: "gone", version: undefined }),
        ],
      }),
    );
    const link = await screen.findByRole("link", { name: "alice" });
    expect(link.getAttribute("href")).toBe(
      "/agents/local/alice/activity?view=metrics&period=month",
    );
    expect(screen.queryByRole("link", { name: "gone" })).toBeNull();
    expect(screen.getByTitle("This agent is not on the host any more.")).toHaveTextContent("gone");
  });

  it("opens the compare screen from an image group with the period and versions", async () => {
    renderPage(
      "/metrics?view=images&period=week",
      metricsReport({
        images: [
          metricsSlice({ name: "developer", version: "0.23.1" }),
          metricsSlice({ name: "developer", version: "0.23.2" }),
        ],
      }),
    );
    const link = await screen.findByRole("link", { name: "Compare versions" });
    expect(link.getAttribute("href")).toBe(
      "/metrics/compare?period=week&kind=image&name=developer&a=0.23.1&b=0.23.2",
    );
  });

  it("announces the first load", async () => {
    renderPage("/metrics", () => new Promise(() => {}));
    expect(
      await screen.findByText("Loading metrics…", { selector: "[role=status]" }),
    ).toBeInTheDocument();
  });

  it("keeps the last numbers while Refresh loads new ones", async () => {
    let calls = 0;
    let release!: (r: MetricsReport) => void;
    renderPage("/metrics", () => {
      calls += 1;
      return calls === 1
        ? Promise.resolve(metricsReport())
        : new Promise<MetricsReport>((resolve) => (release = resolve));
    });
    await waitFor(() => expect(card("Tasks done")).toBe("7"));
    expect(screen.getByText(/^Updated \d\d:\d\d$/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    expect(await screen.findByRole("button", { name: "Updating…" })).toBeInTheDocument();
    expect(card("Tasks done")).toBe("7");
    await act(async () => release(metricsReport()));
    expect(screen.getByRole("button", { name: "Refresh" })).toBeInTheDocument();
  });

  it("shows a failure with Retry", async () => {
    let fail = true;
    const { getMetrics } = renderPage("/metrics", () =>
      fail
        ? Promise.reject(new ApiError(500, "internal", "disk full"))
        : Promise.resolve(metricsReport()),
    );
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Could not load metrics.");
    expect(alert).toHaveTextContent("disk full");
    fail = false;
    fireEvent.click(within(alert).getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(card("Tasks done")).toBe("7"));
    expect(getMetrics).toHaveBeenCalledTimes(2);
  });

  it("explains a timeout", async () => {
    renderPage("/metrics", new ApiError(0, "network_error", "request timed out"));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The host did not answer in 30 seconds.",
    );
  });

  it("explains an older host", async () => {
    renderPage("/metrics", new ApiError(404, "not_found", "unknown route GET /api/metrics"));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "This host runs an older daemon without metrics. Update the host to see them.",
    );
    expect(screen.getByRole("link", { name: "Settings → Hosts" })).toBeInTheDocument();
  });

  it("shows a rejected time in the period bar", async () => {
    renderPage("/metrics", new ApiError(400, "bad_time", "since: not RFC 3339"));
    expect(await screen.findByRole("alert")).toHaveTextContent("Enter a valid start time.");
  });

  it("shows a rejected filter at its chips", async () => {
    renderPage("/metrics?image=a:b:c", new ApiError(400, "invalid_param", "bad image"));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "This filter is not valid. Use NAME or NAME:VERSION.",
    );
  });

  it("asks nothing for a custom period without a start", async () => {
    const { getMetrics } = renderPage("/metrics?period=custom");
    expect(await screen.findByRole("alert")).toHaveTextContent("Enter a valid start time.");
    expect(getMetrics).not.toHaveBeenCalled();
  });

  it("offers 90 days for an empty period", async () => {
    const empty = metricsSlice();
    empty.usage = { ...empty.usage, iterations: 0 };
    empty.tasks = { ...empty.tasks, tasks_done: 0 };
    renderPage("/metrics", metricsReport({ total: empty, agents: [] }));
    expect(
      await screen.findByText("No runs and no completed tasks in this period."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Show 90 days" }));
    expect(query().get("period")).toBe("quarter");
  });

  it("offers Clear filters for an empty filtered period", async () => {
    const empty = metricsSlice();
    empty.usage = { ...empty.usage, iterations: 0 };
    empty.tasks = { ...empty.tasks, tasks_done: 0 };
    renderPage("/metrics?agent=alice", metricsReport({ total: empty, agents: [] }));
    const note = await screen.findByText("No data for these filters in this period.");
    fireEvent.click(within(note.parentElement!).getByRole("button", { name: "Clear filters" }));
    expect(query().has("agent")).toBe(false);
  });

  it("says when no task was completed but runs were made", async () => {
    const slice = metricsSlice();
    slice.tasks = { ...slice.tasks, tasks_done: 0, avg: undefined };
    renderPage("/metrics", metricsReport({ total: slice }));
    expect(await screen.findByText("No task was completed in this period.")).toBeInTheDocument();
    expect(card("Runs")).toBe("23");
  });

  it("says when a view has no rows", async () => {
    renderPage("/metrics", metricsReport({ agents: [] }));
    expect(await screen.findByText("No agents in this period.")).toBeInTheDocument();
  });
});
