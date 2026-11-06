import { describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { fireEvent, render, screen } from "@/test/queryRender";
import { agentLinks } from "@/fixtures/agents";
import { HomeView, type HomeViewProps } from "./HomePage.view";

const props: HomeViewProps = {
  needsYouCount: "0",
  needsYou: null,
  running: [],
  runningTotal: 0,
  runningExpanded: false,
  onToggleRunning: () => {},
  recent: [],
  totals: null,
  todayError: null,
  activityHref: "/activity",
  onNewAgent: () => {},
};

const renderView = (over: Partial<HomeViewProps>) =>
  render(
    <MemoryRouter>
      <HomeView {...props} {...over} />
    </MemoryRouter>,
  );

describe("HomeView", () => {
  it("announces the today error as an alert", () => {
    renderView({ todayError: "request timed out" });
    expect(screen.getByRole("alert")).toHaveTextContent("request timed out");
  });

  // jsdom does not lay out: this pins the markup, the screenshots check the look.
  it("keeps the agent name before long metadata in Running now and Recent", () => {
    renderView({
      running: agentLinks.longText,
      runningTotal: agentLinks.longText.length,
      recent: agentLinks.longText,
    });
    const { name, meta } = agentLinks.longText[0];
    const names = screen.getAllByText(name);
    expect(names).toHaveLength(2);
    for (const nameEl of names) {
      expect(nameEl).toHaveClass("truncate");
      const metaEl = nameEl.nextElementSibling;
      expect(metaEl).toHaveTextContent(meta);
      expect(metaEl).toHaveClass("truncate", "sm:max-w-1/2", "sm:shrink-0");
    }
  });

  it("offers Show all (N) when Running now has more than 8 agents, and Show less once open", () => {
    const toggle = vi.fn();
    const shown = agentLinks.many.slice(0, 8);
    const { rerender } = renderView({ running: shown, runningTotal: 12, onToggleRunning: toggle });
    expect(screen.getByRole("heading", { name: "Running now (12)" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Show all (12)" }));
    expect(toggle).toHaveBeenCalled();
    rerender(
      <MemoryRouter>
        <HomeView {...props} running={shown} runningTotal={12} runningExpanded />
      </MemoryRouter>,
    );
    expect(screen.getByRole("button", { name: "Show less" })).toBeInTheDocument();
  });

  it("has no Show all button for 8 running agents", () => {
    renderView({ running: agentLinks.many.slice(0, 8), runningTotal: 8 });
    expect(screen.queryByRole("button", { name: /Show/ })).toBeNull();
  });
});
