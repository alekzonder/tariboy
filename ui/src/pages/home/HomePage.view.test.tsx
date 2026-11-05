import { describe, expect, it } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { render, screen } from "@/test/queryRender";
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
});
