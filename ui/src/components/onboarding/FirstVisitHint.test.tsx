import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import userEvent from "@testing-library/user-event";
import { render, screen } from "@testing-library/react";
import { FirstVisitHint } from "./FirstVisitHint";
import { HINT_TEXT, type HintTab } from "@/lib/onboarding/hints";

beforeEach(() => {
  localStorage.clear();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("FirstVisitHint", () => {
  it.each([
    [
      "console",
      "Type what you want done. To hand work over and let the agent run on its own, give it a task.",
    ],
    ["work", "Tasks you create here are given to this agent."],
    [
      "settings",
      "Autopilot: set a schedule here to let the agent keep working without you. Everything else is optional.",
    ],
  ] as const)("shows the %s line once per agent until it is dismissed", async (tab, text) => {
    expect(HINT_TEXT[tab as HintTab]).toBe(text);
    const first = render(<FirstVisitHint hostId="h1" agent="dev" tab={tab} />);
    expect(screen.getByText(text)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Dismiss hint" }));
    expect(screen.queryByText(text)).toBeNull();
    first.unmount();

    // The same agent keeps it hidden; another agent still gets it.
    const again = render(<FirstVisitHint hostId="h1" agent="dev" tab={tab} />);
    expect(screen.queryByText(text)).toBeNull();
    again.unmount();
    render(<FirstVisitHint hostId="h1" agent="other" tab={tab} />);
    expect(screen.getByText(text)).toBeInTheDocument();
  });

  it("shows the line again after a reload when storage is blocked", async () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("blocked", "SecurityError");
    });
    const first = render(<FirstVisitHint hostId="" agent="dev" tab="work" />);
    await userEvent.click(screen.getByRole("button", { name: "Dismiss hint" }));
    expect(screen.queryByText(HINT_TEXT.work)).toBeNull();
    first.unmount();

    render(<FirstVisitHint hostId="" agent="dev" tab="work" />);
    expect(screen.getByText(HINT_TEXT.work)).toBeInTheDocument();
  });
});
