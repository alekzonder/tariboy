import { describe, expect, it } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { render, screen } from "@/test/queryRender";
import { NeedsYouContext } from "@/pages/terminals/needsYouContext";
import { NeedsYouList } from "./NeedsYouList";
import type { NeedsYouViewRow } from "./needsYouModel";

const TITLE = "Approve the release notes for the nightly release candidate of worker:v2";
const HOST = "a-build-host-with-a-deliberately-long-label";

const rows: NeedsYouViewRow[] = [
  {
    id: "t1",
    kind: "task",
    hostId: "",
    ref: "TARIDESIGN-38fp",
    label: "TARIDESIGN-38fp",
    title: TITLE,
    meta: `waiting for you · release-candidate-packager · ${HOST} · 12 min ago`,
  },
  {
    id: "l1",
    kind: "letter",
    hostId: "",
    ref: "m1",
    label: "tariboy-release-publisher",
    title: "Which image builds the docs?",
    meta: `Answer on TARI-7n7t · ${HOST} · 2 h ago`,
  },
];

function renderList(showAll: { label: string; href: string } | null = null) {
  render(
    <MemoryRouter>
      <NeedsYouContext.Provider value={{ hosts: [], loaded: true, reload: () => {} }}>
        <NeedsYouList rows={rows} showAll={showAll} onOpenLetter={() => {}} />
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
    const meta = screen.getByText(new RegExp(`waiting for you .* ${HOST}`));
    expect(meta).toHaveClass("col-start-2", "min-w-0", "truncate", "sm:max-w-1/2", "sm:shrink-0");
    expect(meta).not.toHaveClass("shrink-0");
  });

  it("gives a letter row the same classes, a one-line sender and a Mail icon that keeps its size", () => {
    renderList();
    const row = screen.getByRole("button", { name: /Which image builds the docs/ });
    expect(row).toHaveClass("grid", "grid-cols-[6rem_minmax(0,1fr)]", "sm:flex");
    const sender = screen.getByText("tariboy-release-publisher");
    expect(sender).toHaveClass("truncate");
    expect(sender.previousElementSibling).toHaveClass("shrink-0");
    const meta = screen.getByText(/Answer on TARI-7n7t/);
    expect(meta).toHaveClass("col-start-2", "truncate", "sm:shrink-0");
  });

  it("shows no Show all link without a target, and the link with one", () => {
    renderList();
    expect(screen.queryByRole("link", { name: /Show all/ })).toBeNull();
  });

  it("links Show all (N) to its target", () => {
    renderList({ label: "Show all (100+)", href: "/inbox" });
    expect(screen.getByRole("link", { name: "Show all (100+)" })).toHaveAttribute("href", "/inbox");
  });
});
