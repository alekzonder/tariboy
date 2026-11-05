import { describe, expect, it } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { render, screen } from "@/test/queryRender";
import { taskItem } from "@/fixtures/tasks";
import { letter } from "@/test/fixtures/mailFixtures";
import type { NeedsYouRow } from "@/lib/needsYou";
import { NeedsYouContext } from "@/pages/terminals/needsYouContext";
import { NeedsYouList } from "./NeedsYouList";

const TITLE = "Approve the release notes for the nightly release candidate of worker:v2";
const HOST = "a-build-host-with-a-deliberately-long-label";

const rows: NeedsYouRow[] = [
  {
    kind: "task",
    id: "TARIDESIGN-38fp",
    hostId: "",
    hostLabel: HOST,
    at: "2026-11-05T09:42:00Z",
    task: taskItem({
      key: "TARIDESIGN-38fp",
      title: TITLE,
      status: "wait_customer",
      assignee: "release-candidate-packager-for-the-long-running-nightly",
    }),
  },
  {
    kind: "letter",
    id: "m1",
    hostId: "",
    hostLabel: HOST,
    at: "2026-11-05T09:41:00Z",
    letter: letter({ id: "m1", subject: "Question about the nightly build" }),
  },
];

function renderList() {
  render(
    <MemoryRouter>
      <NeedsYouContext.Provider value={{ hosts: [], loaded: true, reload: () => {} }}>
        <NeedsYouList rows={rows} multiHost onOpenLetter={() => {}} />
      </NeedsYouContext.Provider>
    </MemoryRouter>,
  );
}

// jsdom does not lay out, so these tests pin the markup and classes that keep
// the title visible at 360 px; the story screenshots check the look.
describe("NeedsYouList row at 360 px", () => {
  it("keeps the task title in a grid row whose metadata sits under the title", () => {
    renderList();
    const row = screen.getByRole("button", { name: new RegExp(TITLE) });
    expect(row).toHaveClass("grid", "grid-cols-[6rem_minmax(0,1fr)]", "sm:flex");
    const meta = screen.getByText(new RegExp(`wait_customer .* ${HOST}`));
    expect(meta).toHaveClass("col-start-2", "min-w-0", "truncate", "sm:max-w-1/2", "sm:shrink-0");
    expect(meta).not.toHaveClass("shrink-0");
  });

  it("gives a letter row the same classes", () => {
    renderList();
    const row = screen.getByRole("button", { name: /Question about the nightly build/ });
    expect(row).toHaveClass("grid", "grid-cols-[6rem_minmax(0,1fr)]", "sm:flex");
    const meta = screen.getByText(new RegExp(`letter .* ${HOST}`));
    expect(meta).toHaveClass("col-start-2", "truncate", "sm:shrink-0");
    expect(meta).not.toHaveClass("shrink-0");
  });
});
