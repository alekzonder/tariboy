import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import type { ReactNode } from "react";
import { ApiError } from "@/lib/api";
import { metricsSlice } from "@/test/fixtures/apiFixtures";
import { AboutNumbers } from "./AboutNumbers";
import { MetricsCards } from "./MetricsCards";
import { MetricsError, MetricsLoading } from "./MetricsStatus";
import { QualityBlock } from "./QualityBlock";
import { VersionGroups } from "./VersionGroups";
import { Unknown } from "./Unknown";

const inRouter = (node: ReactNode) => render(<MemoryRouter>{node}</MemoryRouter>);

/** The value and the second line of the card with this label. */
function card(label: string) {
  const el = document.querySelector(`[data-card="${label}"]`) as HTMLElement;
  expect(el, label).not.toBeNull();
  return {
    value: el.querySelector("[data-value]")?.textContent ?? "",
    detail: el.querySelector("[data-detail]")?.textContent ?? "",
    el,
  };
}

describe("Unknown", () => {
  it("shows a dash with the reason as a hint and for the screen reader", () => {
    render(<Unknown reason="No scores on this axis." />);
    const dash = screen.getByTitle("No scores on this axis.");
    expect(dash).toHaveTextContent("—");
    expect(within(dash).getByText("not known: No scores on this axis.")).toHaveClass("sr-only");
  });
});

describe("MetricsCards", () => {
  it("maps every card to its field of the slice", () => {
    render(<MetricsCards slice={metricsSlice()} />);
    expect(card("Tasks done").value).toBe("7");
    expect(card("Lead time")).toMatchObject({ value: "12h 30m", detail: "avg 14h 20m" });
    expect(card("Cost per task")).toMatchObject({ value: "$1.37", detail: "total $9.59" });
    expect(card("Runs per task")).toMatchObject({ value: "2.3", detail: "total 16" });
    expect(card("Rework per task")).toMatchObject({ value: "0.4", detail: "total 3" });
    expect(card("Rejected advances per task")).toMatchObject({ value: "0.1", detail: "total 1" });
    expect(card("Questions per task")).toMatchObject({ value: "0.6", detail: "total 4" });
    const tokens = card("Tokens per task");
    expect(tokens).toMatchObject({ value: "1.2M", detail: "total 8.4M" });
    expect(within(tokens.el).getByTitle(/^Input 400,000 · Output 50,000/)).toBeInTheDocument();
    expect(card("Runs").value).toBe("23");
    expect(card("Cost").value).toBe("$12.40");
    expect(card("Tokens").value).toBe("8.6M");
    expect(
      within(card("Tokens").el).getByTitle(
        "Input 3,000,000 · Output 400,000 · Cache write 200,000 · Cache read 5,000,000",
      ),
    ).toBeInTheDocument();
  });

  it("captions the two groups and explains the dash", () => {
    render(<MetricsCards slice={metricsSlice()} />);
    expect(screen.getByRole("region", { name: "Completed tasks" })).toHaveTextContent(
      "Tasks completed in this period. Runs and cost count each task's whole life.",
    );
    expect(screen.getByRole("region", { name: "AI usage in this period" })).toHaveTextContent(
      "Runs and cost of AI requests made in this period.",
    );
    expect(screen.getByText("— means the value is not known. It is not zero.")).toBeInTheDocument();
  });

  it("shows 0 done tasks as 0 and the per-task values as unknown, never as 0", () => {
    const slice = metricsSlice();
    slice.tasks = {
      ...slice.tasks,
      tasks_done: 0,
      avg: undefined,
      lead_time_median_s: null,
      lead_time_avg_s: null,
    };
    render(<MetricsCards slice={slice} />);
    expect(card("Tasks done").value).toBe("0");
    expect(within(card("Cost per task").el).getAllByTitle(/No completed tasks/)).toHaveLength(1);
    expect(
      within(card("Lead time").el).getAllByTitle(
        "No completed task with a start time in this period.",
      ),
    ).toHaveLength(2);
    expect(screen.getByText("No task was completed in this period.")).toBeInTheDocument();
    expect(card("Runs").value).toBe("23");
  });

  it("can leave out the AI usage group", () => {
    render(<MetricsCards slice={metricsSlice()} usage={false} />);
    expect(screen.queryByRole("region", { name: "AI usage in this period" })).toBeNull();
  });
});

describe("QualityBlock", () => {
  it("shows the empty state without scores", () => {
    render(<QualityBlock quality={null} />);
    expect(screen.getByText("No quality scores yet")).toBeInTheDocument();
    expect(screen.getByText("Scores appear here after tasks are assessed.")).toBeInTheDocument();
  });

  it("shows avg, min and count per axis and No scores for an axis without them", () => {
    render(
      <QualityBlock
        quality={{ image_process: { avg: 82, min: 61, count: 12 }, role: null, task_fit: null }}
      />,
    );
    expect(screen.getByText("Average assessment score from 0 to 100.")).toBeInTheDocument();
    const axis = screen.getByRole("listitem", { name: "Image and process" });
    expect(axis).toHaveTextContent("82");
    expect(axis).toHaveTextContent("min 61");
    expect(axis).toHaveTextContent("12 scores");
    expect(screen.getByRole("listitem", { name: "Role" })).toHaveTextContent("No scores");
    expect(screen.getByRole("listitem", { name: "Task fit" })).toHaveTextContent("No scores");
  });
});

describe("VersionGroups", () => {
  const images = [
    metricsSlice({ name: "developer", version: "0.9.0" }),
    metricsSlice({ name: "developer", version: "unknown" }),
    metricsSlice({ name: "developer", version: "0.10.0" }),
    metricsSlice({ name: "basic", version: "1.0.0" }),
  ];
  const compareHref = (name: string, a: string, b: string) => `/c?${name}&${a}&${b}`;

  it("groups by name, newest version first and unknown last, with Mixed tasks", () => {
    inRouter(<VersionGroups kind="image" slices={images} compareHref={compareHref} />);
    const groups = screen.getAllByRole("table");
    expect(groups.map((t) => t.getAttribute("aria-label"))).toEqual(["basic", "developer"]);
    const versions = within(groups[1])
      .getAllByRole("rowheader")
      .map((cell) => cell.textContent);
    expect(versions[0]).toContain("0.10.0");
    expect(versions[1]).toContain("0.9.0");
    expect(versions[2]).toContain("unknown version");
    expect(
      within(groups[1]).getByTitle(
        "The run's image version is not recorded, for example after data retention.",
      ),
    ).toBeInTheDocument();
    const mixed = within(groups[1]).getByRole("columnheader", { name: /Mixed tasks/ });
    expect(mixed.querySelector("[title]")?.getAttribute("title")).toBe(
      "Done tasks that ran on this and other versions. They are not counted in this row.",
    );
  });

  it("maps each column to its field", () => {
    inRouter(
      <VersionGroups kind="image" slices={[images[0], images[2]]} compareHref={compareHref} />,
    );
    const row = screen.getAllByRole("row")[1];
    const cells = within(row)
      .getAllByRole("cell")
      .map((c) => c.textContent);
    expect(cells).toEqual([
      "7",
      "3",
      "12h 30m",
      "$1.37",
      "2.3",
      "0.4",
      "0.1",
      "0.6",
      "74",
      "23",
      "$12.40",
    ]);
  });

  it("offers Compare versions only for a name with two versions, and ⇄ per row", () => {
    inRouter(<VersionGroups kind="image" slices={images} compareHref={compareHref} />);
    const links = screen.getAllByRole("link", { name: "Compare versions" });
    expect(links).toHaveLength(1);
    expect(links[0]).toHaveAttribute("href", "/c?developer&0.9.0&0.10.0");
    const rows = screen.getAllByRole("link", { name: "Compare with another version" });
    expect(rows[0]).toHaveAttribute("href", "/c?developer&0.9.0&0.10.0");
    expect(rows[1]).toHaveAttribute("href", "/c?developer&&0.9.0");
  });

  it("sorts inside each group, with unknown values last in either direction", () => {
    const slow = metricsSlice({ name: "developer", version: "0.1.0" });
    slow.tasks = { ...slow.tasks, lead_time_median_s: 90_000 };
    const none = metricsSlice({ name: "developer", version: "0.2.0" });
    none.tasks = { ...none.tasks, lead_time_median_s: null };
    const fast = metricsSlice({ name: "developer", version: "0.3.0" });
    fast.tasks = { ...fast.tasks, lead_time_median_s: 60 };
    inRouter(<VersionGroups kind="image" slices={[slow, none, fast]} compareHref={compareHref} />);
    const order = () =>
      screen.getAllByRole("rowheader").map((c) => c.textContent?.replace(/⇄|Compare.*/, ""));
    const header = screen.getByRole("columnheader", { name: /Lead time/ });
    fireEvent.click(within(header).getByRole("button"));
    expect(header).toHaveAttribute("aria-sort", "descending");
    expect(order()).toEqual(["0.1.0", "0.3.0", "0.2.0"]);
    fireEvent.click(within(header).getByRole("button"));
    expect(header).toHaveAttribute("aria-sort", "ascending");
    expect(order()).toEqual(["0.3.0", "0.1.0", "0.2.0"]);
  });

  it("names the special workflows and keeps none without versions or compare", () => {
    inRouter(
      <VersionGroups
        kind="workflow"
        slices={[
          metricsSlice({ name: "none", version: "" }),
          metricsSlice({ name: "unknown", version: "" }),
          metricsSlice({ name: "development", version: "0.10.1" }),
          metricsSlice({ name: "development", version: "0.9.0" }),
        ]}
        compareHref={compareHref}
      />,
    );
    expect(
      screen.getByText("A task counts for a version only when all its visits used that version."),
    ).toBeInTheDocument();
    const none = screen.getByRole("table", { name: "No workflow" });
    expect(within(none).queryByRole("columnheader", { name: /Version/ })).toBeNull();
    expect(screen.getByTitle("Runs and tasks without a workflow.")).toBeInTheDocument();
    expect(screen.getByRole("table", { name: "Unknown workflow" })).toBeInTheDocument();
    expect(
      screen.getByTitle("The workflow of this digest is not on the host any more."),
    ).toBeInTheDocument();
    expect(screen.getAllByRole("link", { name: "Compare versions" })).toHaveLength(1);
  });

  it("says when the view is empty", () => {
    inRouter(<VersionGroups kind="workflow" slices={[]} compareHref={compareHref} />);
    expect(screen.getByText("No workflows in this period.")).toBeInTheDocument();
  });
});

describe("AboutNumbers", () => {
  it("opens and closes its text", () => {
    render(<AboutNumbers />);
    const button = screen.getByRole("button", { name: "About these numbers" });
    expect(button).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(button);
    expect(button).toHaveAttribute("aria-expanded", "true");
    const panel = document.getElementById(button.getAttribute("aria-controls")!);
    expect(panel).toHaveTextContent("unknown version: the image version of a run is not recorded.");
  });
});

describe("MetricsStatus", () => {
  it("announces the first load", () => {
    render(<MetricsLoading />);
    expect(screen.getByRole("status")).toHaveTextContent("Loading metrics…");
  });

  it("shows a failure with its text and Retry", () => {
    const retry = vi.fn();
    inRouter(<MetricsError error={new ApiError(500, "internal", "disk full")} onRetry={retry} />);
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Could not load metrics.");
    expect(alert).toHaveTextContent("disk full");
    fireEvent.click(within(alert).getByRole("button", { name: "Retry" }));
    expect(retry).toHaveBeenCalled();
  });

  it("explains a timeout", () => {
    inRouter(
      <MetricsError
        error={new ApiError(0, "network_error", "request timed out")}
        onRetry={vi.fn()}
      />,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("The host did not answer in 30 seconds.");
  });

  it("explains an older host and links to Settings → Hosts", () => {
    inRouter(
      <MetricsError
        error={new ApiError(404, "not_found", "unknown route GET /api/metrics")}
        onRetry={vi.fn()}
      />,
    );
    expect(screen.getByRole("alert")).toHaveTextContent(
      "This host runs an older daemon without metrics. Update the host to see them.",
    );
    expect(screen.getByRole("link", { name: "Settings → Hosts" })).toHaveAttribute(
      "href",
      "/settings/hosts",
    );
  });
});
