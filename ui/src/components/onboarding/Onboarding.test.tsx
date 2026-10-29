import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import userEvent from "@testing-library/user-event";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { OnboardingProvider } from "./OnboardingProvider";
import { HelpMenu } from "./HelpMenu";
import { TOUR_ANCHORS, TOUR_STEPS, type TourAnchor } from "@/lib/onboarding/tour";
import { UI_VERSION } from "@/lib/onboarding/version";
import { readKey, storageKeys } from "@/lib/storage";
import { SidebarStateContext } from "@/pages/terminals/sidebarStateContext";

const ALL = Object.values(TOUR_ANCHORS).filter(
  (anchor) => anchor !== TOUR_ANCHORS.helpMenu && anchor !== TOUR_ANCHORS.agentRow,
);

function LocationProbe() {
  return <output data-testid="location">{useLocation().pathname}</output>;
}

function renderTour({ anchors = ALL }: { anchors?: TourAnchor[] } = {}) {
  return render(
    <MemoryRouter initialEntries={["/servers/local/mail"]}>
      <OnboardingProvider>
        {anchors.map((anchor) => (
          <button key={anchor} type="button" data-tour={anchor}>
            {anchor}
          </button>
        ))}
        <HelpMenu />
        <LocationProbe />
      </OnboardingProvider>
    </MemoryRouter>,
  );
}

const card = (name: string) => screen.findByRole("dialog", { name });
const next = () => fireEvent.keyDown(window, { key: "ArrowRight" });
const stepIndex = (id: string) => TOUR_STEPS.findIndex((step) => step.id === id);

async function goTo(id: string) {
  await card("Welcome to Tariboy");
  for (let i = 0; i < stepIndex(id); i++) {
    next();
    await card(TOUR_STEPS[i + 1].title).catch(() => undefined);
  }
}

beforeEach(() => {
  localStorage.clear();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("onboarding tour", () => {
  it("starts by itself on the first run", async () => {
    renderTour();
    const welcome = await card("Welcome to Tariboy");
    expect(within(welcome).getByText(`1 of ${TOUR_STEPS.length}`)).toBeInTheDocument();
    expect(screen.queryByTestId("tour-highlight")).toBeNull();
  });

  it("does not start again after Skip tour", async () => {
    const first = renderTour();
    await userEvent.click(
      within(await card("Welcome to Tariboy")).getByRole("button", { name: "Skip tour" }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(readKey(storageKeys.onboarding)).toBe(UI_VERSION);
    first.unmount();

    renderTour();
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("highlights the element of the step", async () => {
    // jsdom lays nothing out: give the anchors a box so they count as visible.
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue(
      DOMRect.fromRect({ x: 10, y: 20, width: 80, height: 24 }),
    );
    renderTour();
    await card("Welcome to Tariboy");
    await userEvent.click(screen.getByRole("button", { name: "Next" }));
    await card("Servers");
    await waitFor(() =>
      expect(screen.getByTestId("tour-highlight")).toHaveAttribute("data-anchor", "servers-tab"),
    );
  });

  it("opens the server section of a step", async () => {
    renderTour();
    await goTo("images");
    await card("Agent images");
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("/servers/local/images"),
    );
  });

  it("centers the card without highlight when the element is missing", async () => {
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue(
      DOMRect.fromRect({ x: 10, y: 20, width: 80, height: 24 }),
    );
    renderTour({ anchors: ALL.filter((anchor) => anchor !== TOUR_ANCHORS.addHost) });
    await goTo("servers");
    await waitFor(() => expect(screen.getByTestId("tour-highlight")).toBeInTheDocument());
    next();
    await card("Add a server");
    await new Promise((resolve) => setTimeout(resolve, 300));
    expect(screen.queryByTestId("tour-highlight")).toBeNull();
  });

  it("skips the agent steps when there is no agent", async () => {
    renderTour();
    await goTo("agents");
    await card("Agents");
    next();
    expect(await card("Tasks")).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "ArrowLeft" });
    expect(await card("Agents")).toBeInTheDocument();
  });

  it.each([
    { compact: false, hidden: true, drawerOpen: false, call: "setHidden", arg: false },
    { compact: true, hidden: false, drawerOpen: true, call: "setDrawerOpen", arg: false },
  ] as const)(
    "shows a hidden sidebar, but closes the modal phone drawer (compact: $compact)",
    async ({ compact, hidden, drawerOpen, call, arg }) => {
      const sidebar = {
        width: 268,
        hidden,
        compact,
        drawerOpen,
        setWidth: vi.fn(),
        setHidden: vi.fn(),
        setDrawerOpen: vi.fn(),
      };
      render(
        <MemoryRouter initialEntries={["/servers/local/mail"]}>
          <SidebarStateContext.Provider value={sidebar}>
            <OnboardingProvider>
              <HelpMenu />
            </OnboardingProvider>
          </SidebarStateContext.Provider>
        </MemoryRouter>,
      );
      await card("Welcome to Tariboy");
      next();
      await card("Servers");
      await waitFor(() => expect(sidebar[call]).toHaveBeenCalledWith(arg));
    },
  );

  it("closes with Escape and records the version", async () => {
    renderTour();
    await card("Welcome to Tariboy");
    fireEvent.keyDown(window, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(readKey(storageKeys.onboarding)).toBe(UI_VERSION);
  });

  it("starts again from the help menu", async () => {
    localStorage.setItem(storageKeys.onboarding.name, UI_VERSION);
    renderTour();
    await userEvent.click(screen.getByRole("button", { name: "Help" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Take the tour" }));
    expect(await card("Welcome to Tariboy")).toBeInTheDocument();
  });

  it("shows the changelog of the selected version", async () => {
    localStorage.setItem(storageKeys.onboarding.name, UI_VERSION);
    renderTour();
    await userEvent.click(screen.getByRole("button", { name: "Help" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Changelog" }));
    const dialog = await screen.findByRole("dialog", { name: "Changelog" });
    expect(within(dialog).getByRole("heading", { name: UI_VERSION })).toBeInTheDocument();
    await userEvent.click(within(dialog).getByRole("button", { name: /0\.96\.0/ }));
    expect(within(dialog).getByRole("heading", { name: "0.96.0" })).toBeInTheDocument();
  });
});
