import { it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { AgentNameContext } from "@/lib/agent";
import AgentUsagePage from "./AgentUsagePage";

afterEach(() => vi.restoreAllMocks());

// Capture every GET path so we can assert the window/grouping params reach the
// backend, and reply with a small canned report.
function stubFetch(): string[] {
  const paths: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((path: string) => {
      paths.push(path);
      const groupBy = new URL(path, "http://x").searchParams.get("group_by") ?? "iteration";
      const report = {
        agent: "alice",
        group_by: groupBy,
        bucket: "1h",
        totals: {
          requests: 3,
          input_tokens: 1200,
          output_tokens: 800,
          cache_write_tokens: 0,
          cache_read_tokens: 500,
          cost_usd: 0.35,
        },
        rows:
          groupBy === "epic"
            ? [{ key: "dev-t-3e1", title: "Per-agent Usage epic", requests: 3, input_tokens: 1200, output_tokens: 800, cache_write_tokens: 0, cache_read_tokens: 500, cost_usd: 0.35 }]
            : [{ key: "iter-123", requests: 3, input_tokens: 1200, output_tokens: 800, cache_write_tokens: 0, cache_read_tokens: 500, cost_usd: 0.35 }],
        series: [{ bucket_start: "2026-07-12T10:00:00Z", requests: 3, tokens: 2000, cost_usd: 0.35 }],
      };
      return Promise.resolve({
        ok: true,
        status: 200,
        text: async () => JSON.stringify({ ok: true, result: report }),
      } as Response);
    }),
  );
  return paths;
}

function renderPage() {
  return render(
    <MemoryRouter>
      <AgentNameContext.Provider value="alice">
        <AgentUsagePage />
      </AgentNameContext.Provider>
    </MemoryRouter>,
  );
}

it("renders totals and an iteration row linking to the audit log", async () => {
  stubFetch();
  renderPage();
  // Cost appears in both the totals card and the table row.
  await waitFor(() => expect(screen.getAllByText("$0.3500").length).toBeGreaterThanOrEqual(2));
  const link = await screen.findByRole("link", { name: "iter-123" });
  // The iteration row deep-links into the Audit Log preselecting that iteration
  // (AuditLogPage reads ?iteration=), not the bare Full-log view.
  expect(link.getAttribute("href")).toBe("/agent/alice/logs?iteration=iter-123");
});

it("widens the bucket for a from-only custom range instead of leaving it at 1h", async () => {
  const paths = stubFetch();
  renderPage();
  await waitFor(() => expect(screen.getByText("iter-123")).toBeInTheDocument());
  fireEvent.click(screen.getByText("Custom"));
  // Only the 'from' bound set, spanning weeks — the bucket must coarsen (1d)
  // via closestBucket rather than staying at the 1h default (spec §4: never
  // render thousands of bars).
  fireEvent.change(screen.getByLabelText("from"), { target: { value: "2026-01-01T00:00" } });
  await waitFor(() => expect(paths.some((p) => p.includes("bucket=1d"))).toBe(true));
  // And it never requested the from-only range at the 1h default.
  const customCalls = paths.filter((p) => p.includes("since=2026-01-01") || p.includes("since=2025-12-31"));
  expect(customCalls.length).toBeGreaterThan(0);
  expect(customCalls.every((p) => !p.includes("bucket=1h"))).toBe(true);
});

it("switches grouping and requests the new group_by", async () => {
  const paths = stubFetch();
  renderPage();
  await waitFor(() => expect(screen.getByText("iter-123")).toBeInTheDocument());
  fireEvent.click(screen.getByText("Epics"));
  await waitFor(() => expect(screen.getByText("Per-agent Usage epic")).toBeInTheDocument());
  expect(paths.some((p) => p.includes("group_by=epic"))).toBe(true);
});

it("sends a since bound for the default preset window", async () => {
  const paths = stubFetch();
  renderPage();
  await waitFor(() => expect(paths.some((p) => p.includes("since="))).toBe(true));
});
