import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import userEvent from "@testing-library/user-event";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { toast } from "sonner";
import { OnboardingProvider } from "./OnboardingProvider";
import { HelpMenu } from "./HelpMenu";
import { TOUR_ANCHORS, TOUR_STEPS, type TourAnchor } from "@/lib/onboarding/tour";
import { UI_VERSION } from "@/lib/onboarding/version";
import { readKey, storageKeys } from "@/lib/storage";
import { SidebarStateContext } from "@/pages/terminals/sidebarStateContext";

vi.mock("sonner", async (importOriginal) => ({
  ...(await importOriginal<typeof import("sonner")>()),
  toast: vi.fn(),
}));

const ALL = Object.values(TOUR_ANCHORS).filter(
  (anchor) => anchor !== TOUR_ANCHORS.helpMenu && anchor !== TOUR_ANCHORS.agentRow,
);
const START = "/inbox?letter=m-1";

function LocationProbe() {
  const { pathname, search } = useLocation();
  return <output data-testid="location">{pathname + search}</output>;
}

function renderTour({
  anchors = ALL,
  banner = false,
}: { anchors?: TourAnchor[]; banner?: boolean } = {}) {
  return render(
    <MemoryRouter initialEntries={[START]}>
      <OnboardingProvider>
        {banner && <div data-daemon-banner="error">tariboyd: down</div>}
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

async function takeTour() {
  await userEvent.click(screen.getByRole("button", { name: "Help" }));
  await userEvent.click(await screen.findByRole("menuitem", { name: "Take the tour" }));
  return card("Welcome to Tariboy");
}

/** Presses Next until the card of `title` shows. */
async function goTo(title: string) {
  for (let i = 0; i < TOUR_STEPS.length; i++) {
    if (screen.queryByRole("dialog", { name: title })) return;
    next();
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
  }
  await card(title);
}

beforeEach(() => {
  localStorage.clear();
  vi.mocked(toast).mockClear();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("onboarding tour", () => {
  it("starts no tour on the first run and stores the version as seen", async () => {
    renderTour();
    await waitFor(() => expect(readKey(storageKeys.onboarding)).toBe(UI_VERSION));
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("counts only the steps it shows: no agent steps without agents", async () => {
    renderTour();
    const welcome = await takeTour();
    expect(within(welcome).getByText("1 of 10")).toBeInTheDocument();
    expect(screen.queryByTestId("tour-highlight")).toBeNull();
  });

  it("counts the agent steps when the sidebar has an agent", async () => {
    renderTour({ anchors: [...ALL, TOUR_ANCHORS.agentRow] });
    const welcome = await takeTour();
    expect(within(welcome).getByText(`1 of ${TOUR_STEPS.length}`)).toBeInTheDocument();
  });

  it("goes back to the start route after Skip tour", async () => {
    renderTour();
    await takeTour();
    await goTo("Tasks");
    await waitFor(() => expect(screen.getByTestId("location").textContent).toBe("/tasks"));
    await userEvent.click(within(await card("Tasks")).getByRole("button", { name: "Skip tour" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByTestId("location")).toHaveTextContent(START);
    expect(readKey(storageKeys.onboarding)).toBe(UI_VERSION);
  });

  it("goes back to the start route after Done", async () => {
    renderTour();
    await takeTour();
    await goTo("Help");
    await userEvent.click(within(await card("Help")).getByRole("button", { name: "Done" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByTestId("location")).toHaveTextContent(START);
  });

  it("opens the home screen and its New agent button on the Agents step", async () => {
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue(
      DOMRect.fromRect({ x: 10, y: 20, width: 80, height: 24 }),
    );
    renderTour();
    await takeTour();
    next();
    await card("Agents");
    await waitFor(() => expect(screen.getByTestId("location").textContent).toBe("/"));
    await waitFor(() =>
      expect(screen.getByTestId("tour-highlight")).toHaveAttribute("data-anchor", "new-agent"),
    );
  });

  it("does not start while the daemon error banner shows, and says why", async () => {
    renderTour({ banner: true });
    await userEvent.click(screen.getByRole("button", { name: "Help" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Take the tour" }));
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(toast).toHaveBeenCalledWith("The tour opens after the daemon error is fixed.");
  });

  it("highlights the element of the step", async () => {
    // jsdom lays nothing out: give the anchors a box so they count as visible.
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue(
      DOMRect.fromRect({ x: 10, y: 20, width: 80, height: 24 }),
    );
    renderTour();
    await takeTour();
    await goTo("Hosts");
    await waitFor(() =>
      expect(screen.getByTestId("tour-highlight")).toHaveAttribute("data-anchor", "hosts-line"),
    );
  });

  it("opens the page of a step", async () => {
    renderTour();
    await takeTour();
    await goTo("Agent images");
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("/servers/local/images"),
    );
  });

  it("centers the card without highlight when the element is missing", async () => {
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue(
      DOMRect.fromRect({ x: 10, y: 20, width: 80, height: 24 }),
    );
    renderTour({ anchors: ALL.filter((anchor) => anchor !== TOUR_ANCHORS.addHost) });
    await takeTour();
    await goTo("Hosts");
    await waitFor(() => expect(screen.getByTestId("tour-highlight")).toBeInTheDocument());
    next();
    await card("Add a host");
    await new Promise((resolve) => setTimeout(resolve, 300));
    expect(screen.queryByTestId("tour-highlight")).toBeNull();
  });

  it("skips the agent steps when there is no agent", async () => {
    renderTour();
    await takeTour();
    await goTo("Agents");
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
        <MemoryRouter initialEntries={["/inbox"]}>
          <SidebarStateContext.Provider value={sidebar}>
            <OnboardingProvider>
              <HelpMenu />
            </OnboardingProvider>
          </SidebarStateContext.Provider>
        </MemoryRouter>,
      );
      await takeTour();
      await goTo("Hosts");
      await waitFor(() => expect(sidebar[call]).toHaveBeenCalledWith(arg));
    },
  );

  it("closes with Escape, records the version and goes back", async () => {
    renderTour();
    await takeTour();
    await goTo("Agent images");
    fireEvent.keyDown(window, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(readKey(storageKeys.onboarding)).toBe(UI_VERSION);
    expect(screen.getByTestId("location")).toHaveTextContent(START);
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
