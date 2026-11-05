import { describe, expect, it } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { render, screen } from "@/test/queryRender";
import { agentLinks } from "@/fixtures/agents";
import { HomeView } from "./HomePage.view";

describe("HomeView", () => {
  it("announces the today error as an alert", () => {
    render(
      <MemoryRouter>
        <HomeView
          needsYouCount={0}
          needsYou={null}
          running={[]}
          recent={[]}
          totals={null}
          todayError="request timed out"
          activityHref="/activity"
          onNewAgent={() => {}}
        />
      </MemoryRouter>,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("request timed out");
  });

  // jsdom does not lay out: this pins the markup, the screenshots check the look.
  it("keeps the agent name before long metadata in Running now and Recent", () => {
    render(
      <MemoryRouter>
        <HomeView
          needsYouCount={0}
          needsYou={null}
          running={agentLinks.longText}
          recent={agentLinks.longText}
          totals={null}
          todayError={null}
          activityHref="/activity"
          onNewAgent={() => {}}
        />
      </MemoryRouter>,
    );
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
});
